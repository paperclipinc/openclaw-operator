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
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// SelfConfigFieldManager is the SSA field manager name for SelfConfig changes.
const SelfConfigFieldManager = "openclaw-selfconfig"

// protectedConfigKeys are config paths that cannot be modified via self-config
// to prevent breaking gateway authentication.
var protectedConfigKeys = map[string]bool{
	"gateway": true, // block entire gateway subtree for safety
}

// protectedEnvVars are environment variable names that cannot be overridden
// via self-config because they are operator-managed.
var protectedEnvVars = map[string]bool{
	"HOME":                      true,
	"OPENCLAW_DISABLE_BONJOUR":  true,
	"OPENCLAW_GATEWAY_TOKEN":    true,
	"OPENCLAW_INSTANCE_NAME":    true,
	"OPENCLAW_NAMESPACE":        true,
	"PATH":                      true,
	"CHROMIUM_URL":              true,
	"OLLAMA_HOST":               true,
	"TS_AUTHKEY":                true,
	"TS_HOSTNAME":               true,
	"TS_SOCKET":                 true,
	"NODE_EXTRA_CA_CERTS":       true,
	"NPM_CONFIG_CACHE":          true,
	"NPM_CONFIG_IGNORE_SCRIPTS": true,
}

// determineActions inspects which action categories a SelfConfig request uses.
func determineActions(sc *openclawv1alpha1.OpenClawSelfConfig) []openclawv1alpha1.SelfConfigAction {
	var actions []openclawv1alpha1.SelfConfigAction
	if len(sc.Spec.AddSkills) > 0 || len(sc.Spec.RemoveSkills) > 0 {
		actions = append(actions, openclawv1alpha1.SelfConfigActionSkills)
	}
	if sc.Spec.ConfigPatch != nil {
		actions = append(actions, openclawv1alpha1.SelfConfigActionConfig)
	}
	if len(sc.Spec.AddWorkspaceFiles) > 0 || len(sc.Spec.RemoveWorkspaceFiles) > 0 {
		actions = append(actions, openclawv1alpha1.SelfConfigActionWorkspaceFiles)
	}
	if len(sc.Spec.AddEnvVars) > 0 || len(sc.Spec.RemoveEnvVars) > 0 {
		actions = append(actions, openclawv1alpha1.SelfConfigActionEnvVars)
	}
	return actions
}

// checkAllowedActions validates that all requested actions are in the allowed list.
// Returns a list of denied action names, or nil if all are allowed.
func checkAllowedActions(requested, allowed []openclawv1alpha1.SelfConfigAction) []openclawv1alpha1.SelfConfigAction {
	allowedSet := make(map[openclawv1alpha1.SelfConfigAction]bool, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = true
	}

	var denied []openclawv1alpha1.SelfConfigAction
	for _, a := range requested {
		if !allowedSet[a] {
			denied = append(denied, a)
		}
	}
	return denied
}

// buildSkillsApply computes the skills list for an SSA apply.
// Merges new skills into the current list and filters out removals.
func buildSkillsApply(current []string, sc *openclawv1alpha1.OpenClawSelfConfig) []string {
	removeSet := make(map[string]bool, len(sc.Spec.RemoveSkills))
	for _, s := range sc.Spec.RemoveSkills {
		removeSet[s] = true
	}

	seen := make(map[string]bool, len(current)+len(sc.Spec.AddSkills))
	result := make([]string, 0, len(current)+len(sc.Spec.AddSkills))
	for _, s := range current {
		if removeSet[s] {
			continue
		}
		if !seen[s] {
			result = append(result, s)
			seen[s] = true
		}
	}

	for _, s := range sc.Spec.AddSkills {
		if !seen[s] && !removeSet[s] {
			result = append(result, s)
			seen[s] = true
		}
	}

	return result
}

// buildConfigApply deep-merges the config patch into the current config.
// Returns the merged raw JSON for the apply configuration.
func buildConfigApply(current *openclawv1alpha1.RawConfig, sc *openclawv1alpha1.OpenClawSelfConfig) ([]byte, error) {
	if sc.Spec.ConfigPatch == nil || len(sc.Spec.ConfigPatch.Raw) == 0 {
		return nil, nil
	}

	var patch map[string]interface{}
	if err := json.Unmarshal(sc.Spec.ConfigPatch.Raw, &patch); err != nil {
		return nil, fmt.Errorf("invalid config patch JSON: %w", err)
	}

	for key := range patch {
		if protectedConfigKeys[key] {
			return nil, fmt.Errorf("config key %q is protected and cannot be modified via self-config", key)
		}
	}

	var base map[string]interface{}
	if current != nil && len(current.Raw) > 0 {
		if err := json.Unmarshal(current.Raw, &base); err != nil {
			return nil, fmt.Errorf("failed to parse existing config: %w", err)
		}
	} else {
		base = make(map[string]interface{})
	}

	merged := deepMerge(base, patch)
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged config: %w", err)
	}

	return raw, nil
}

// deepMerge recursively merges src into dst. Arrays are replaced, not merged.
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(dst))
	for k, v := range dst {
		result[k] = v
	}
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := result[k].(map[string]interface{}); ok {
				result[k] = deepMerge(dstMap, srcMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}

// buildWorkspaceFilesApply computes the workspace files map for an SSA apply.
// Merges new files into the current map and removes targeted files.
func buildWorkspaceFilesApply(current map[string]string, sc *openclawv1alpha1.OpenClawSelfConfig) map[string]string {
	result := make(map[string]string, len(current)+len(sc.Spec.AddWorkspaceFiles))

	removeSet := make(map[string]bool, len(sc.Spec.RemoveWorkspaceFiles))
	for _, name := range sc.Spec.RemoveWorkspaceFiles {
		removeSet[name] = true
	}
	for name, content := range current {
		if !removeSet[name] {
			result[name] = content
		}
	}

	for name, content := range sc.Spec.AddWorkspaceFiles {
		result[name] = content
	}

	return result
}

// buildEnvApply computes the env var list for an SSA apply.
// Validates against protected env vars, then merges adds and filters removals.
func buildEnvApply(current []corev1.EnvVar, sc *openclawv1alpha1.OpenClawSelfConfig) ([]corev1.EnvVar, error) {
	for _, ev := range sc.Spec.AddEnvVars {
		if protectedEnvVars[ev.Name] {
			return nil, fmt.Errorf("environment variable %q is protected and cannot be modified via self-config", ev.Name)
		}
	}

	for _, name := range sc.Spec.RemoveEnvVars {
		if protectedEnvVars[name] {
			return nil, fmt.Errorf("environment variable %q is protected and cannot be removed via self-config", name)
		}
	}

	removeSet := make(map[string]bool, len(sc.Spec.RemoveEnvVars))
	for _, name := range sc.Spec.RemoveEnvVars {
		removeSet[name] = true
	}
	addByName := make(map[string]string, len(sc.Spec.AddEnvVars))
	for _, ev := range sc.Spec.AddEnvVars {
		addByName[ev.Name] = ev.Value
	}

	seen := make(map[string]bool, len(current)+len(sc.Spec.AddEnvVars))
	result := make([]corev1.EnvVar, 0, len(current)+len(sc.Spec.AddEnvVars))
	for _, ev := range current {
		if removeSet[ev.Name] {
			continue
		}
		if val, ok := addByName[ev.Name]; ok {
			result = append(result, corev1.EnvVar{Name: ev.Name, Value: val})
			seen[ev.Name] = true
			continue
		}
		result = append(result, ev)
		seen[ev.Name] = true
	}

	for _, ev := range sc.Spec.AddEnvVars {
		if !seen[ev.Name] {
			result = append(result, corev1.EnvVar{Name: ev.Name, Value: ev.Value})
			seen[ev.Name] = true
		}
	}

	return result, nil
}

// checkRemovalOwnership checks whether a named item is owned by the SelfConfig
// field manager. Returns a warning message if the item is not owned (and thus
// SSA will not remove it), or empty string if the removal is valid.
func checkRemovalOwnership(name, kind string, owned map[string]bool, findManager func(string) string) string {
	if owned != nil && owned[name] {
		return ""
	}
	if manager := findManager(name); manager != "" {
		return fmt.Sprintf("cannot remove %s %q: managed by field manager %q", kind, name, manager)
	}
	return fmt.Sprintf("cannot remove %s %q: not managed by %s", kind, name, SelfConfigFieldManager)
}

// buildApplySpec constructs the partial OpenClawInstanceSpec for an SSA apply
// based on the current instance state and the requested SelfConfig changes.
func buildApplySpec(
	instance *openclawv1alpha1.OpenClawInstance,
	sc *openclawv1alpha1.OpenClawSelfConfig,
	actions []openclawv1alpha1.SelfConfigAction,
) (*openclawv1alpha1.OpenClawInstanceSpec, error) {
	spec := &openclawv1alpha1.OpenClawInstanceSpec{}

	for _, action := range actions {
		switch action {
		case openclawv1alpha1.SelfConfigActionSkills:
			spec.Skills = buildSkillsApply(instance.Spec.Skills, sc)

		case openclawv1alpha1.SelfConfigActionConfig:
			raw, err := buildConfigApply(instance.Spec.Config.Raw, sc)
			if err != nil {
				return nil, err
			}
			if raw != nil {
				spec.Config.Raw = &openclawv1alpha1.RawConfig{
					RawExtension: runtime.RawExtension{Raw: raw},
				}
			}

		case openclawv1alpha1.SelfConfigActionWorkspaceFiles:
			var currentFiles map[string]string
			if instance.Spec.Workspace != nil {
				currentFiles = instance.Spec.Workspace.InitialFiles
			}
			files := buildWorkspaceFilesApply(currentFiles, sc)
			spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
				InitialFiles: files,
			}

		case openclawv1alpha1.SelfConfigActionEnvVars:
			env, err := buildEnvApply(instance.Spec.Env, sc)
			if err != nil {
				return nil, err
			}
			spec.Env = env
		}
	}

	return spec, nil
}
