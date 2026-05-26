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

package e2e

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

var _ = Describe("OpenClawSelfConfig Controller", func() {
	const (
		timeout  = time.Second * 120
		interval = time.Second * 2
	)

	Context("When creating an instance with selfConfigure enabled", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-sc-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespace},
			}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespace},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should create RBAC resources with self-configure permissions", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "sc-rbac-test"

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: namespace,
					Annotations: map[string]string{
						"openclaw.rocks/skip-backup": "true",
					},
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					Image: openclawv1alpha1.ImageSpec{
						Repository: "ghcr.io/openclaw/openclaw",
						Tag:        "latest",
					},
					SelfConfigure: openclawv1alpha1.SelfConfigureSpec{
						Enabled: true,
						AllowedActions: []openclawv1alpha1.SelfConfigAction{
							openclawv1alpha1.SelfConfigActionSkills,
							openclawv1alpha1.SelfConfigActionConfig,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for StatefulSet to exist - proves full reconcile completed
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
				if err == nil {
					return "found"
				}
				// Include instance phase in error for diagnostics
				inst := &openclawv1alpha1.OpenClawInstance{}
				phase := "unknown"
				if getErr := k8sClient.Get(ctx, types.NamespacedName{
					Name: instanceName, Namespace: namespace,
				}, inst); getErr == nil {
					phase = inst.Status.Phase
				}
				return "not found (instance phase: " + phase + ")"
			}, timeout, interval).Should(Equal("found"),
				"StatefulSet should be created by reconcile")

			// Verify SA has AutomountServiceAccountToken = true
			sa := &corev1.ServiceAccount{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.ServiceAccountName(instance),
				Namespace: namespace,
			}, sa)).To(Succeed())
			Expect(sa.AutomountServiceAccountToken).NotTo(BeNil())
			Expect(*sa.AutomountServiceAccountToken).To(BeTrue(),
				"SA should have automount enabled for self-configure")

			// Verify Role has openclaw.rocks RBAC rules
			role := &rbacv1.Role{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.RoleName(instance),
				Namespace: namespace,
			}, role)).To(Succeed())

			var foundInstances, foundSelfConfigs bool
			for _, rule := range role.Rules {
				for _, res := range rule.Resources {
					if res == "openclawinstances" {
						foundInstances = true
					}
					if res == "openclawselfconfigs" {
						foundSelfConfigs = true
					}
				}
			}
			Expect(foundInstances).To(BeTrue(), "Role should have openclawinstances rule")
			Expect(foundSelfConfigs).To(BeTrue(), "Role should have openclawselfconfigs rule")

			// Verify workspace ConfigMap has self-configure files
			wsCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.WorkspaceConfigMapName(instance),
				Namespace: namespace,
			}, wsCM)).To(Succeed())
			Expect(wsCM.Data).To(HaveKey("SELFCONFIG.md"))
			Expect(wsCM.Data).To(HaveKey("selfconfig.sh"))

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should apply an OpenClawSelfConfig and update parent instance", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "sc-apply-test"

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: namespace,
					Annotations: map[string]string{
						"openclaw.rocks/skip-backup": "true",
					},
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					Image: openclawv1alpha1.ImageSpec{
						Repository: "ghcr.io/openclaw/openclaw",
						Tag:        "latest",
					},
					Skills: []string{"existing-skill"},
					SelfConfigure: openclawv1alpha1.SelfConfigureSpec{
						Enabled: true,
						AllowedActions: []openclawv1alpha1.SelfConfigAction{
							openclawv1alpha1.SelfConfigActionSkills,
							openclawv1alpha1.SelfConfigActionEnvVars,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for initial reconciliation (StatefulSet proves full reconcile completed)
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, &appsv1.StatefulSet{})
				if err == nil {
					return "found"
				}
				inst := &openclawv1alpha1.OpenClawInstance{}
				phase := "unknown"
				if getErr := k8sClient.Get(ctx, types.NamespacedName{
					Name: instanceName, Namespace: namespace,
				}, inst); getErr == nil {
					phase = inst.Status.Phase
				}
				return "not found (instance phase: " + phase + ")"
			}, timeout, interval).Should(Equal("found"),
				"StatefulSet should be created by reconcile")

			// Create a self-config request to add a skill
			sc := &openclawv1alpha1.OpenClawSelfConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "add-skill-e2e",
					Namespace: namespace,
				},
				Spec: openclawv1alpha1.OpenClawSelfConfigSpec{
					InstanceRef: instanceName,
					AddSkills:   []string{"@anthropic/mcp-server-fetch"},
				},
			}
			Expect(k8sClient.Create(ctx, sc)).Should(Succeed())

			// Wait for Applied phase
			Eventually(func() openclawv1alpha1.SelfConfigPhase {
				fetched := &openclawv1alpha1.OpenClawSelfConfig{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "add-skill-e2e", Namespace: namespace,
				}, fetched); err != nil {
					return ""
				}
				return fetched.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.SelfConfigPhaseApplied))

			// Verify parent instance was updated
			updatedInstance := &openclawv1alpha1.OpenClawInstance{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: instanceName, Namespace: namespace,
			}, updatedInstance)).To(Succeed())
			Expect(updatedInstance.Spec.Skills).To(ContainElement("@anthropic/mcp-server-fetch"))
			Expect(updatedInstance.Spec.Skills).To(ContainElement("existing-skill"))

			// Verify that the selfconfig field manager appears in managed fields
			managedFields := updatedInstance.GetManagedFields()
			var foundSelfConfigManager bool
			for _, entry := range managedFields {
				if entry.Manager == "openclaw-selfconfig" {
					foundSelfConfigManager = true
					break
				}
			}
			Expect(foundSelfConfigManager).To(BeTrue(),
				"openclaw-selfconfig field manager should appear in managed fields after SSA apply")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should preserve field ownership across multiple SelfConfig applies", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "sc-multi-apply-test"

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: namespace,
					Annotations: map[string]string{
						"openclaw.rocks/skip-backup": "true",
					},
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					Image: openclawv1alpha1.ImageSpec{
						Repository: "ghcr.io/openclaw/openclaw",
						Tag:        "latest",
					},
					Skills: []string{"bootstrap-skill"},
					SelfConfigure: openclawv1alpha1.SelfConfigureSpec{
						Enabled: true,
						AllowedActions: []openclawv1alpha1.SelfConfigAction{
							openclawv1alpha1.SelfConfigActionSkills,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for initial reconciliation
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, &appsv1.StatefulSet{})
				if err == nil {
					return "found"
				}
				inst := &openclawv1alpha1.OpenClawInstance{}
				phase := "unknown"
				if getErr := k8sClient.Get(ctx, types.NamespacedName{
					Name: instanceName, Namespace: namespace,
				}, inst); getErr == nil {
					phase = inst.Status.Phase
				}
				return "not found (instance phase: " + phase + ")"
			}, timeout, interval).Should(Equal("found"),
				"StatefulSet should be created by reconcile")

			// First SelfConfig: add skill 1
			sc1 := &openclawv1alpha1.OpenClawSelfConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "add-skill-1",
					Namespace: namespace,
				},
				Spec: openclawv1alpha1.OpenClawSelfConfigSpec{
					InstanceRef: instanceName,
					AddSkills:   []string{"added-skill-1"},
				},
			}
			Expect(k8sClient.Create(ctx, sc1)).Should(Succeed())

			Eventually(func() openclawv1alpha1.SelfConfigPhase {
				fetched := &openclawv1alpha1.OpenClawSelfConfig{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "add-skill-1", Namespace: namespace,
				}, fetched); err != nil {
					return ""
				}
				return fetched.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.SelfConfigPhaseApplied))

			// Second SelfConfig: add skill 2
			sc2 := &openclawv1alpha1.OpenClawSelfConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "add-skill-2",
					Namespace: namespace,
				},
				Spec: openclawv1alpha1.OpenClawSelfConfigSpec{
					InstanceRef: instanceName,
					AddSkills:   []string{"added-skill-2"},
				},
			}
			Expect(k8sClient.Create(ctx, sc2)).Should(Succeed())

			Eventually(func() openclawv1alpha1.SelfConfigPhase {
				fetched := &openclawv1alpha1.OpenClawSelfConfig{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "add-skill-2", Namespace: namespace,
				}, fetched); err != nil {
					return ""
				}
				return fetched.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.SelfConfigPhaseApplied))

			// Verify all three skills coexist
			updatedInstance := &openclawv1alpha1.OpenClawInstance{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: instanceName, Namespace: namespace,
			}, updatedInstance)).To(Succeed())
			Expect(updatedInstance.Spec.Skills).To(ContainElement("bootstrap-skill"))
			Expect(updatedInstance.Spec.Skills).To(ContainElement("added-skill-1"))
			Expect(updatedInstance.Spec.Skills).To(ContainElement("added-skill-2"))

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should deny disallowed config action", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "sc-deny-test"

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: namespace,
					Annotations: map[string]string{
						"openclaw.rocks/skip-backup": "true",
					},
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					Image: openclawv1alpha1.ImageSpec{
						Repository: "ghcr.io/openclaw/openclaw",
						Tag:        "latest",
					},
					SelfConfigure: openclawv1alpha1.SelfConfigureSpec{
						Enabled: true,
						AllowedActions: []openclawv1alpha1.SelfConfigAction{
							openclawv1alpha1.SelfConfigActionSkills,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for initial reconciliation (StatefulSet proves instance is indexed in cache)
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, &appsv1.StatefulSet{})
				if err == nil {
					return "found"
				}
				inst := &openclawv1alpha1.OpenClawInstance{}
				phase := "unknown"
				if getErr := k8sClient.Get(ctx, types.NamespacedName{
					Name: instanceName, Namespace: namespace,
				}, inst); getErr == nil {
					phase = inst.Status.Phase
				}
				return "not found (instance phase: " + phase + ")"
			}, timeout, interval).Should(Equal("found"),
				"StatefulSet should be created by reconcile")

			// Create a self-config request for config (not in allowedActions)
			sc := &openclawv1alpha1.OpenClawSelfConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "denied-config-e2e",
					Namespace: namespace,
				},
				Spec: openclawv1alpha1.OpenClawSelfConfigSpec{
					InstanceRef: instanceName,
					ConfigPatch: &openclawv1alpha1.RawConfig{
						RawExtension: runtime.RawExtension{Raw: []byte(`{"key":"value"}`)},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sc)).Should(Succeed())

			// Wait for Denied phase
			Eventually(func() openclawv1alpha1.SelfConfigPhase {
				fetched := &openclawv1alpha1.OpenClawSelfConfig{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "denied-config-e2e", Namespace: namespace,
				}, fetched); err != nil {
					return ""
				}
				return fetched.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.SelfConfigPhaseDenied))

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
