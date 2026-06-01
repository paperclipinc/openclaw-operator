/*
Copyright 2026 Paperclip Inc.

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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// managedFieldsSet is a recursive map structure representing SSA managed fields.
// Keys are field paths like "f:spec", "f:skills", "v:skill-name", "k:{\"name\":\"VAR\"}".
type managedFieldsSet map[string]interface{}

// findSelfConfigFields returns the managed fields set for the SelfConfig field manager,
// or nil if the field manager is not found.
func findSelfConfigFields(managedFields []metav1.ManagedFieldsEntry) managedFieldsSet {
	for _, entry := range managedFields {
		if entry.Manager == SelfConfigFieldManager && entry.Operation == metav1.ManagedFieldsOperationApply {
			if entry.FieldsV1 == nil {
				return nil
			}
			var fields managedFieldsSet
			if err := json.Unmarshal(entry.FieldsV1.Raw, &fields); err != nil {
				return nil
			}
			return fields
		}
	}
	return nil
}

// navigateFields traverses a managedFieldsSet through the given path segments.
// Each segment should be a field key like "f:spec" or "f:skills".
func navigateFields(fields managedFieldsSet, path ...string) managedFieldsSet {
	current := fields
	for _, segment := range path {
		if current == nil {
			return nil
		}
		next, ok := current[segment]
		if !ok {
			return nil
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return nil
		}
		current = managedFieldsSet(nextMap)
	}
	return current
}

// findFieldManagerByKey searches all Apply-type managed field entries for one
// that owns the given key at the specified field path. Returns the field manager
// name or empty string if not found. Only Apply entries are checked to avoid
// matching Update entries from the same manager.
func findFieldManagerByKey(managedFields []metav1.ManagedFieldsEntry, key string, path ...string) string {
	for _, entry := range managedFields {
		if entry.Operation != metav1.ManagedFieldsOperationApply {
			continue
		}
		if entry.FieldsV1 == nil {
			continue
		}
		var fields managedFieldsSet
		if err := json.Unmarshal(entry.FieldsV1.Raw, &fields); err != nil {
			continue
		}
		container := navigateFields(fields, path...)
		if container == nil {
			continue
		}
		if _, ok := container[key]; ok {
			return entry.Manager
		}
	}
	return ""
}

// extractOwnedSkills returns the set of skill names owned by the SelfConfig field manager.
func extractOwnedSkills(managedFields []metav1.ManagedFieldsEntry) map[string]bool {
	fields := findSelfConfigFields(managedFields)
	if fields == nil {
		return nil
	}

	skillsContainer := navigateFields(fields, "f:spec", "f:skills")
	if skillsContainer == nil {
		return nil
	}

	result := make(map[string]bool)
	for key := range skillsContainer {
		// Set items use the format v:"skill-name"
		if strings.HasPrefix(key, "v:") {
			result[key[2:]] = true
		}
	}
	return result
}

// extractOwnedEnvVars returns the set of env var names owned by the SelfConfig field manager.
func extractOwnedEnvVars(managedFields []metav1.ManagedFieldsEntry) map[string]bool {
	fields := findSelfConfigFields(managedFields)
	if fields == nil {
		return nil
	}

	envContainer := navigateFields(fields, "f:spec", "f:env")
	if envContainer == nil {
		return nil
	}

	result := make(map[string]bool)
	for key := range envContainer {
		// Map items use the format k:{"name":"VAR_NAME"}
		if strings.HasPrefix(key, "k:") {
			name := extractNameFromMapKey(key)
			if name != "" {
				result[name] = true
			}
		}
	}
	return result
}

// extractOwnedWorkspaceFiles returns the set of workspace file names owned by the SelfConfig field manager.
func extractOwnedWorkspaceFiles(managedFields []metav1.ManagedFieldsEntry) map[string]bool {
	fields := findSelfConfigFields(managedFields)
	if fields == nil {
		return nil
	}

	filesContainer := navigateFields(fields, "f:spec", "f:workspace", "f:initialFiles")
	if filesContainer == nil {
		return nil
	}

	result := make(map[string]bool)
	for key := range filesContainer {
		// Map fields use the format f:filename
		if strings.HasPrefix(key, "f:") {
			result[key[2:]] = true
		}
	}
	return result
}

// findSkillFieldManager returns the field manager that owns a given skill.
func findSkillFieldManager(managedFields []metav1.ManagedFieldsEntry, skill string) string {
	return findFieldManagerByKey(managedFields, "v:"+skill, "f:spec", "f:skills")
}

// findEnvVarFieldManager returns the field manager that owns a given env var.
// The key is constructed via string concatenation without JSON-escaping because
// Kubernetes env var names are restricted to [A-Za-z_][A-Za-z0-9_]* and cannot
// contain characters that would need escaping.
func findEnvVarFieldManager(managedFields []metav1.ManagedFieldsEntry, name string) string {
	key := `k:{"name":"` + name + `"}`
	return findFieldManagerByKey(managedFields, key, "f:spec", "f:env")
}

// findWorkspaceFileFieldManager returns the field manager that owns a given workspace file.
func findWorkspaceFileFieldManager(managedFields []metav1.ManagedFieldsEntry, filename string) string {
	return findFieldManagerByKey(managedFields, "f:"+filename, "f:spec", "f:workspace", "f:initialFiles")
}

// extractNameFromMapKey extracts the "name" value from an SSA map key like k:{"name":"VAR_NAME"}.
func extractNameFromMapKey(key string) string {
	// key format: k:{"name":"VAR_NAME"}
	jsonPart := key[2:] // strip "k:"
	var parsed map[string]string
	if err := json.Unmarshal([]byte(jsonPart), &parsed); err != nil {
		return ""
	}
	return parsed["name"]
}
