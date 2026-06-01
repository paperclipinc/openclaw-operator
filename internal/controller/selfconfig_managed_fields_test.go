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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// buildManagedFieldsEntry creates a ManagedFieldsEntry with the given manager name
// and raw JSON fields, using the Apply operation.
func buildManagedFieldsEntry(manager, fieldsJSON string) metav1.ManagedFieldsEntry {
	return metav1.ManagedFieldsEntry{
		Manager:   manager,
		Operation: metav1.ManagedFieldsOperationApply,
		FieldsV1:  &metav1.FieldsV1{Raw: []byte(fieldsJSON)},
	}
}

func TestExtractOwnedSkills(t *testing.T) {
	managedFields := []metav1.ManagedFieldsEntry{
		buildManagedFieldsEntry(SelfConfigFieldManager, `{
			"f:spec": {
				"f:skills": {
					"v:mcp-server-fetch": {},
					"v:mcp-server-puppeteer": {}
				}
			}
		}`),
		buildManagedFieldsEntry("kubectl-client-side-apply", `{
			"f:spec": {
				"f:skills": {
					"v:bootstrap-skill": {}
				}
			}
		}`),
	}

	owned := extractOwnedSkills(managedFields)
	if owned == nil {
		t.Fatal("expected non-nil owned skills")
	}
	if !owned["mcp-server-fetch"] {
		t.Error("expected mcp-server-fetch to be owned")
	}
	if !owned["mcp-server-puppeteer"] {
		t.Error("expected mcp-server-puppeteer to be owned")
	}
	if owned["bootstrap-skill"] {
		t.Error("bootstrap-skill should not be owned by selfconfig")
	}
}

func TestExtractOwnedSkills_NoSelfConfigManager(t *testing.T) {
	managedFields := []metav1.ManagedFieldsEntry{
		buildManagedFieldsEntry("kubectl", `{
			"f:spec": {
				"f:skills": {
					"v:some-skill": {}
				}
			}
		}`),
	}

	owned := extractOwnedSkills(managedFields)
	if owned != nil {
		t.Errorf("expected nil, got %v", owned)
	}
}

func TestExtractOwnedSkills_NoSkillsField(t *testing.T) {
	managedFields := []metav1.ManagedFieldsEntry{
		buildManagedFieldsEntry(SelfConfigFieldManager, `{
			"f:spec": {
				"f:config": {}
			}
		}`),
	}

	owned := extractOwnedSkills(managedFields)
	if owned != nil {
		t.Errorf("expected nil, got %v", owned)
	}
}

func TestExtractOwnedEnvVars(t *testing.T) {
	managedFields := []metav1.ManagedFieldsEntry{
		buildManagedFieldsEntry(SelfConfigFieldManager, `{
			"f:spec": {
				"f:env": {
					"k:{\"name\":\"MY_VAR\"}": {},
					"k:{\"name\":\"OTHER_VAR\"}": {}
				}
			}
		}`),
	}

	owned := extractOwnedEnvVars(managedFields)
	if owned == nil {
		t.Fatal("expected non-nil owned env vars")
	}
	if !owned["MY_VAR"] {
		t.Error("expected MY_VAR to be owned")
	}
	if !owned["OTHER_VAR"] {
		t.Error("expected OTHER_VAR to be owned")
	}
}

func TestExtractOwnedEnvVars_NoManager(t *testing.T) {
	owned := extractOwnedEnvVars(nil)
	if owned != nil {
		t.Errorf("expected nil, got %v", owned)
	}
}

func TestExtractOwnedWorkspaceFiles(t *testing.T) {
	managedFields := []metav1.ManagedFieldsEntry{
		buildManagedFieldsEntry(SelfConfigFieldManager, `{
			"f:spec": {
				"f:workspace": {
					"f:initialFiles": {
						"f:notes.md": {},
						"f:plan.txt": {}
					}
				}
			}
		}`),
	}

	owned := extractOwnedWorkspaceFiles(managedFields)
	if owned == nil {
		t.Fatal("expected non-nil owned workspace files")
	}
	if !owned["notes.md"] {
		t.Error("expected notes.md to be owned")
	}
	if !owned["plan.txt"] {
		t.Error("expected plan.txt to be owned")
	}
}

func TestExtractOwnedWorkspaceFiles_NoManager(t *testing.T) {
	owned := extractOwnedWorkspaceFiles(nil)
	if owned != nil {
		t.Errorf("expected nil, got %v", owned)
	}
}

func TestFindSkillFieldManager(t *testing.T) {
	managedFields := []metav1.ManagedFieldsEntry{
		buildManagedFieldsEntry(SelfConfigFieldManager, `{
			"f:spec": {
				"f:skills": {
					"v:selfconfig-skill": {}
				}
			}
		}`),
		buildManagedFieldsEntry("flux-system", `{
			"f:spec": {
				"f:skills": {
					"v:gitops-skill": {}
				}
			}
		}`),
	}

	manager := findSkillFieldManager(managedFields, "gitops-skill")
	if manager != "flux-system" {
		t.Errorf("expected flux-system, got %q", manager)
	}

	manager = findSkillFieldManager(managedFields, "selfconfig-skill")
	if manager != SelfConfigFieldManager {
		t.Errorf("expected %s, got %q", SelfConfigFieldManager, manager)
	}

	manager = findSkillFieldManager(managedFields, "unknown-skill")
	if manager != "" {
		t.Errorf("expected empty, got %q", manager)
	}
}

func TestFindEnvVarFieldManager(t *testing.T) {
	managedFields := []metav1.ManagedFieldsEntry{
		buildManagedFieldsEntry("argocd", `{
			"f:spec": {
				"f:env": {
					"k:{\"name\":\"ARGO_VAR\"}": {}
				}
			}
		}`),
		buildManagedFieldsEntry(SelfConfigFieldManager, `{
			"f:spec": {
				"f:env": {
					"k:{\"name\":\"SELF_VAR\"}": {}
				}
			}
		}`),
	}

	manager := findEnvVarFieldManager(managedFields, "ARGO_VAR")
	if manager != "argocd" {
		t.Errorf("expected argocd, got %q", manager)
	}

	manager = findEnvVarFieldManager(managedFields, "SELF_VAR")
	if manager != SelfConfigFieldManager {
		t.Errorf("expected %s, got %q", SelfConfigFieldManager, manager)
	}

	manager = findEnvVarFieldManager(managedFields, "UNKNOWN")
	if manager != "" {
		t.Errorf("expected empty, got %q", manager)
	}
}

func TestFindWorkspaceFileFieldManager(t *testing.T) {
	managedFields := []metav1.ManagedFieldsEntry{
		buildManagedFieldsEntry("flux-system", `{
			"f:spec": {
				"f:workspace": {
					"f:initialFiles": {
						"f:gitops-file.md": {}
					}
				}
			}
		}`),
		buildManagedFieldsEntry(SelfConfigFieldManager, `{
			"f:spec": {
				"f:workspace": {
					"f:initialFiles": {
						"f:agent-file.md": {}
					}
				}
			}
		}`),
	}

	manager := findWorkspaceFileFieldManager(managedFields, "gitops-file.md")
	if manager != "flux-system" {
		t.Errorf("expected flux-system, got %q", manager)
	}

	manager = findWorkspaceFileFieldManager(managedFields, "agent-file.md")
	if manager != SelfConfigFieldManager {
		t.Errorf("expected %s, got %q", SelfConfigFieldManager, manager)
	}

	manager = findWorkspaceFileFieldManager(managedFields, "unknown.md")
	if manager != "" {
		t.Errorf("expected empty, got %q", manager)
	}
}

func TestFindFieldManagerByKey_SkipsUpdateEntries(t *testing.T) {
	managedFields := []metav1.ManagedFieldsEntry{
		{
			Manager:   "kubectl-edit",
			Operation: metav1.ManagedFieldsOperationUpdate,
			FieldsV1:  &metav1.FieldsV1{Raw: []byte(`{"f:spec":{"f:skills":{"v:update-skill":{}}}}`)},
		},
		buildManagedFieldsEntry("flux-system", `{"f:spec":{"f:skills":{"v:apply-skill":{}}}}`),
	}

	manager := findSkillFieldManager(managedFields, "update-skill")
	if manager != "" {
		t.Errorf("expected empty (Update entry should be skipped), got %q", manager)
	}

	manager = findSkillFieldManager(managedFields, "apply-skill")
	if manager != "flux-system" {
		t.Errorf("expected flux-system, got %q", manager)
	}
}

func TestExtractNameFromMapKey(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{`k:{"name":"MY_VAR"}`, "MY_VAR"},
		{`k:{"name":""}`, ""},
		{`k:invalid`, ""},
	}

	for _, tt := range tests {
		got := extractNameFromMapKey(tt.key)
		if got != tt.want {
			t.Errorf("extractNameFromMapKey(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
