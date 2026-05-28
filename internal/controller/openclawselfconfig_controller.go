/*
Copyright 2026 OpenClaw.rocks

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

const (
	// SelfConfigTTL is how long completed requests are kept before auto-deletion.
	SelfConfigTTL = 1 * time.Hour
)

// OpenClawSelfConfigReconciler reconciles OpenClawSelfConfig objects
type OpenClawSelfConfigReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=openclaw.rocks,resources=openclawselfconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=openclaw.rocks,resources=openclawselfconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=openclaw.rocks,resources=openclawselfconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=openclaw.rocks,resources=openclawinstances,verbs=get;patch

// Reconcile processes an OpenClawSelfConfig request.
func (r *OpenClawSelfConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the SelfConfig resource
	sc := &openclawv1alpha1.OpenClawSelfConfig{}
	if err := r.Get(ctx, req.NamespacedName, sc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Terminal phases - check TTL for cleanup
	if sc.Status.Phase == openclawv1alpha1.SelfConfigPhaseApplied ||
		sc.Status.Phase == openclawv1alpha1.SelfConfigPhaseFailed ||
		sc.Status.Phase == openclawv1alpha1.SelfConfigPhaseDenied {
		if sc.Status.CompletionTime != nil {
			age := time.Since(sc.Status.CompletionTime.Time)
			if age >= SelfConfigTTL {
				logger.Info("deleting expired self-config request", "name", sc.Name, "age", age)
				if err := r.Delete(ctx, sc); err != nil && !apierrors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
			// Requeue for cleanup
			return ctrl.Result{RequeueAfter: SelfConfigTTL - age}, nil
		}
		return ctrl.Result{}, nil
	}

	// Fetch parent instance
	instance := &openclawv1alpha1.OpenClawInstance{}
	if err := r.Get(ctx, types.NamespacedName{Name: sc.Spec.InstanceRef, Namespace: sc.Namespace}, instance); err != nil {
		if apierrors.IsNotFound(err) {
			return r.setTerminalStatus(ctx, sc, openclawv1alpha1.SelfConfigPhaseFailed,
				fmt.Sprintf("instance %q not found", sc.Spec.InstanceRef))
		}
		return ctrl.Result{}, err
	}

	// Validate self-configure is enabled
	if !instance.Spec.SelfConfigure.Enabled {
		return r.setTerminalStatus(ctx, sc, openclawv1alpha1.SelfConfigPhaseDenied,
			"self-configure is not enabled on the target instance")
	}

	// Determine which actions the request uses
	requestedActions := determineActions(sc)
	if len(requestedActions) == 0 {
		return r.setTerminalStatus(ctx, sc, openclawv1alpha1.SelfConfigPhaseFailed,
			"request contains no actions")
	}

	// Check against allowed actions
	denied := checkAllowedActions(requestedActions, instance.Spec.SelfConfigure.AllowedActions)
	if len(denied) > 0 {
		msg := fmt.Sprintf("denied actions: %v", denied)
		r.Recorder.Event(instance, "Warning", "SelfConfigDenied", msg)
		return r.setTerminalStatus(ctx, sc, openclawv1alpha1.SelfConfigPhaseDenied, msg)
	}

	// Build the partial spec for SSA apply
	applySpec, err := buildApplySpec(instance, sc, requestedActions)
	if err != nil {
		logger.Error(err, "failed to build apply spec")
		return r.setTerminalStatus(ctx, sc, openclawv1alpha1.SelfConfigPhaseFailed,
			fmt.Sprintf("failed to build apply spec: %v", err))
	}

	// Check managed fields for removal attempts on unowned items.
	// These checks are purely informational - SSA naturally prevents removing
	// items not owned by our field manager. The apply spec still includes the
	// removals, but SSA will simply not remove items owned by other managers.
	// We emit warnings so users understand why certain removals had no effect.
	managedFields := instance.GetManagedFields()
	var warnings []string

	if len(sc.Spec.RemoveSkills) > 0 {
		ownedSkills := extractOwnedSkills(managedFields)
		for _, s := range sc.Spec.RemoveSkills {
			if msg := checkRemovalOwnership(s, "skill", ownedSkills, func(name string) string {
				return findSkillFieldManager(managedFields, name)
			}); msg != "" {
				warnings = append(warnings, msg)
				r.Recorder.Event(instance, "Warning", "SelfConfigSkippedRemoval", msg)
			}
		}
	}

	if len(sc.Spec.RemoveEnvVars) > 0 {
		ownedEnv := extractOwnedEnvVars(managedFields)
		for _, name := range sc.Spec.RemoveEnvVars {
			if msg := checkRemovalOwnership(name, "env var", ownedEnv, func(name string) string {
				return findEnvVarFieldManager(managedFields, name)
			}); msg != "" {
				warnings = append(warnings, msg)
				r.Recorder.Event(instance, "Warning", "SelfConfigSkippedRemoval", msg)
			}
		}
	}

	if len(sc.Spec.RemoveWorkspaceFiles) > 0 {
		ownedFiles := extractOwnedWorkspaceFiles(managedFields)
		for _, name := range sc.Spec.RemoveWorkspaceFiles {
			if msg := checkRemovalOwnership(name, "workspace file", ownedFiles, func(name string) string {
				return findWorkspaceFileFieldManager(managedFields, name)
			}); msg != "" {
				warnings = append(warnings, msg)
				r.Recorder.Event(instance, "Warning", "SelfConfigSkippedRemoval", msg)
			}
		}
	}

	// Apply changes using Server-Side Apply with dedicated field manager.
	// Use unstructured to preserve empty slices/maps in the JSON payload,
	// which typed objects with omitempty would drop. This ensures the field
	// manager correctly releases ownership when all items are removed.
	typedObj := &openclawv1alpha1.OpenClawInstance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openclawv1alpha1.GroupVersion.String(),
			Kind:       "OpenClawInstance",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: *applySpec,
	}

	rawMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(typedObj)
	if err != nil {
		logger.Error(err, "failed to convert apply spec to unstructured")
		return r.setTerminalStatus(ctx, sc, openclawv1alpha1.SelfConfigPhaseFailed,
			fmt.Sprintf("failed to convert apply spec: %v", err))
	}

	applyObj := &unstructured.Unstructured{Object: rawMap}

	applyErr := r.Patch(ctx, applyObj,
		client.Apply,
		client.FieldOwner(SelfConfigFieldManager),
		client.ForceOwnership,
	)

	if applyErr != nil {
		logger.Error(applyErr, "failed to apply self-config changes via SSA")
		return r.setTerminalStatus(ctx, sc, openclawv1alpha1.SelfConfigPhaseFailed,
			fmt.Sprintf("failed to apply changes: %v", applyErr))
	}

	// Set owner reference to parent instance (for GC on instance deletion)
	if err := controllerutil.SetOwnerReference(instance, sc, r.Scheme); err != nil {
		logger.Error(err, "failed to set owner reference")
		// Non-fatal - continue to mark as applied
	} else {
		if err := r.Update(ctx, sc); err != nil { // reconcile-guard:allow
			logger.Error(err, "failed to update owner reference")
		}
	}

	// Emit events
	r.Recorder.Event(sc, "Normal", "Applied", "self-config request applied successfully")
	r.Recorder.Event(instance, "Normal", "SelfConfigApplied",
		fmt.Sprintf("self-config request %q applied", sc.Name))

	// Build status message including any warnings about skipped removals
	statusMsg := "changes applied successfully"
	if len(warnings) > 0 {
		statusMsg = fmt.Sprintf("changes applied with warnings: %s", strings.Join(warnings, "; "))
	}

	return r.setTerminalStatus(ctx, sc, openclawv1alpha1.SelfConfigPhaseApplied, statusMsg)
}

// setTerminalStatus updates the SelfConfig status to a terminal phase.
func (r *OpenClawSelfConfigReconciler) setTerminalStatus(
	ctx context.Context,
	sc *openclawv1alpha1.OpenClawSelfConfig,
	phase openclawv1alpha1.SelfConfigPhase,
	message string,
) (ctrl.Result, error) {
	now := metav1.Now()
	sc.Status.Phase = phase
	sc.Status.Message = message
	sc.Status.CompletionTime = &now

	if err := r.Status().Update(ctx, sc); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue for TTL cleanup
	return ctrl.Result{RequeueAfter: SelfConfigTTL}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OpenClawSelfConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openclawv1alpha1.OpenClawSelfConfig{}).
		Complete(r)
}
