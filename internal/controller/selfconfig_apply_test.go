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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

func newTestInstance() *openclawv1alpha1.OpenClawInstance {
	return &openclawv1alpha1.OpenClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inst1",
			Namespace: "test-ns",
		},
		Spec: openclawv1alpha1.OpenClawInstanceSpec{},
	}
}

func newTestSelfConfig() *openclawv1alpha1.OpenClawSelfConfig {
	return &openclawv1alpha1.OpenClawSelfConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sc1",
			Namespace: "test-ns",
		},
		Spec: openclawv1alpha1.OpenClawSelfConfigSpec{
			InstanceRef: "inst1",
		},
	}
}

func TestDetermineActions_Skills(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.AddSkills = []string{"@anthropic/mcp-server-fetch"}

	actions := determineActions(sc)
	if len(actions) != 1 || actions[0] != openclawv1alpha1.SelfConfigActionSkills {
		t.Errorf("expected [skills], got %v", actions)
	}
}

func TestDetermineActions_Multiple(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.AddSkills = []string{"skill1"}
	sc.Spec.RemoveEnvVars = []string{"FOO"}

	actions := determineActions(sc)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
}

func TestDetermineActions_Empty(t *testing.T) {
	sc := newTestSelfConfig()

	actions := determineActions(sc)
	if len(actions) != 0 {
		t.Errorf("expected no actions, got %v", actions)
	}
}

func TestCheckAllowedActions_AllAllowed(t *testing.T) {
	requested := []openclawv1alpha1.SelfConfigAction{
		openclawv1alpha1.SelfConfigActionSkills,
		openclawv1alpha1.SelfConfigActionConfig,
	}
	allowed := []openclawv1alpha1.SelfConfigAction{
		openclawv1alpha1.SelfConfigActionSkills,
		openclawv1alpha1.SelfConfigActionConfig,
		openclawv1alpha1.SelfConfigActionEnvVars,
	}

	denied := checkAllowedActions(requested, allowed)
	if len(denied) != 0 {
		t.Errorf("expected no denied, got %v", denied)
	}
}

func TestCheckAllowedActions_SomeDenied(t *testing.T) {
	requested := []openclawv1alpha1.SelfConfigAction{
		openclawv1alpha1.SelfConfigActionSkills,
		openclawv1alpha1.SelfConfigActionEnvVars,
	}
	allowed := []openclawv1alpha1.SelfConfigAction{
		openclawv1alpha1.SelfConfigActionSkills,
	}

	denied := checkAllowedActions(requested, allowed)
	if len(denied) != 1 || denied[0] != openclawv1alpha1.SelfConfigActionEnvVars {
		t.Errorf("expected [envVars] denied, got %v", denied)
	}
}

func TestCheckAllowedActions_EmptyAllowed(t *testing.T) {
	requested := []openclawv1alpha1.SelfConfigAction{
		openclawv1alpha1.SelfConfigActionSkills,
	}
	denied := checkAllowedActions(requested, nil)
	if len(denied) != 1 {
		t.Errorf("expected 1 denied, got %v", denied)
	}
}

func TestBuildSkillsApply_Add(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.AddSkills = []string{"new-skill"}

	result := buildSkillsApply([]string{"existing-skill"}, sc)

	if len(result) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(result))
	}
	if result[0] != "existing-skill" || result[1] != "new-skill" {
		t.Errorf("expected [existing-skill, new-skill], got %v", result)
	}
}

func TestBuildSkillsApply_AddDeduplicate(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.AddSkills = []string{"existing-skill", "new-skill"}

	result := buildSkillsApply([]string{"existing-skill"}, sc)

	if len(result) != 2 {
		t.Fatalf("expected 2 skills (deduplicated), got %d", len(result))
	}
}

func TestBuildSkillsApply_Remove(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.RemoveSkills = []string{"skill-b"}

	result := buildSkillsApply([]string{"skill-a", "skill-b", "skill-c"}, sc)

	if len(result) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(result))
	}
	for _, s := range result {
		if s == "skill-b" {
			t.Error("skill-b should have been removed")
		}
	}
}

func TestBuildSkillsApply_AddAndRemove(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.RemoveSkills = []string{"old-skill"}
	sc.Spec.AddSkills = []string{"new-skill"}

	result := buildSkillsApply([]string{"keep-skill", "old-skill"}, sc)

	if len(result) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(result))
	}
	if result[0] != "keep-skill" || result[1] != "new-skill" {
		t.Errorf("expected [keep-skill, new-skill], got %v", result)
	}
}

func TestBuildSkillsApply_EmptyCurrent(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.AddSkills = []string{"skill-a"}

	result := buildSkillsApply(nil, sc)

	if len(result) != 1 || result[0] != "skill-a" {
		t.Errorf("expected [skill-a], got %v", result)
	}
}

func TestBuildConfigApply_Merge(t *testing.T) {
	current := &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"mcpServers":{"existing":{"command":"node"}},"key":"value"}`)},
	}

	sc := newTestSelfConfig()
	sc.Spec.ConfigPatch = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"mcpServers":{"new":{"command":"python"}},"newKey":"newValue"}`)},
	}

	raw, err := buildConfigApply(current, sc)
	if err != nil {
		t.Fatalf("buildConfigApply failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if result["key"] != "value" {
		t.Error("existing key 'key' should be preserved")
	}
	if result["newKey"] != "newValue" {
		t.Error("new key 'newKey' should be added")
	}
	servers, ok := result["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("mcpServers should be a map")
	}
	if _, ok := servers["existing"]; !ok {
		t.Error("existing MCP server should be preserved")
	}
	if _, ok := servers["new"]; !ok {
		t.Error("new MCP server should be added")
	}
}

func TestBuildConfigApply_ProtectedKey(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.ConfigPatch = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"gateway":{"auth":{"token":"hacked"}}}`)},
	}

	_, err := buildConfigApply(nil, sc)
	if err == nil {
		t.Error("expected error for protected config key 'gateway'")
	}
}

func TestBuildConfigApply_EmptyBase(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.ConfigPatch = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"key":"value"}`)},
	}

	raw, err := buildConfigApply(nil, sc)
	if err != nil {
		t.Fatalf("buildConfigApply failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if result["key"] != "value" {
		t.Error("key should be set")
	}
}

func TestBuildConfigApply_NilPatch(t *testing.T) {
	sc := newTestSelfConfig()
	// No config patch

	raw, err := buildConfigApply(nil, sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if raw != nil {
		t.Error("expected nil raw for nil patch")
	}
}

func TestBuildWorkspaceFilesApply_Add(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.AddWorkspaceFiles = map[string]string{
		"notes.md": "# Notes",
	}

	result := buildWorkspaceFilesApply(nil, sc)

	if result["notes.md"] != "# Notes" {
		t.Error("notes.md should be added")
	}
}

func TestBuildWorkspaceFilesApply_Remove(t *testing.T) {
	current := map[string]string{
		"keep.md":   "keep",
		"remove.md": "remove",
	}

	sc := newTestSelfConfig()
	sc.Spec.RemoveWorkspaceFiles = []string{"remove.md"}

	result := buildWorkspaceFilesApply(current, sc)

	if _, ok := result["remove.md"]; ok {
		t.Error("remove.md should have been removed")
	}
	if result["keep.md"] != "keep" {
		t.Error("keep.md should be preserved")
	}
}

func TestBuildWorkspaceFilesApply_AddAndRemove(t *testing.T) {
	current := map[string]string{
		"old.md": "old content",
	}

	sc := newTestSelfConfig()
	sc.Spec.RemoveWorkspaceFiles = []string{"old.md"}
	sc.Spec.AddWorkspaceFiles = map[string]string{
		"new.md": "new content",
	}

	result := buildWorkspaceFilesApply(current, sc)

	if _, ok := result["old.md"]; ok {
		t.Error("old.md should have been removed")
	}
	if result["new.md"] != "new content" {
		t.Error("new.md should be added")
	}
}

func TestBuildEnvApply_Add(t *testing.T) {
	current := []corev1.EnvVar{
		{Name: "EXISTING", Value: "value1"},
	}

	sc := newTestSelfConfig()
	sc.Spec.AddEnvVars = []openclawv1alpha1.SelfConfigEnvVar{
		{Name: "NEW_VAR", Value: "new_value"},
	}

	result, err := buildEnvApply(current, sc)
	if err != nil {
		t.Fatalf("buildEnvApply failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(result))
	}
}

func TestBuildEnvApply_Replace(t *testing.T) {
	current := []corev1.EnvVar{
		{Name: "MY_VAR", Value: "old"},
	}

	sc := newTestSelfConfig()
	sc.Spec.AddEnvVars = []openclawv1alpha1.SelfConfigEnvVar{
		{Name: "MY_VAR", Value: "new"},
	}

	result, err := buildEnvApply(current, sc)
	if err != nil {
		t.Fatalf("buildEnvApply failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(result))
	}
	if result[0].Value != "new" {
		t.Errorf("expected value 'new', got %q", result[0].Value)
	}
}

func TestBuildEnvApply_Remove(t *testing.T) {
	current := []corev1.EnvVar{
		{Name: "KEEP", Value: "yes"},
		{Name: "REMOVE", Value: "bye"},
	}

	sc := newTestSelfConfig()
	sc.Spec.RemoveEnvVars = []string{"REMOVE"}

	result, err := buildEnvApply(current, sc)
	if err != nil {
		t.Fatalf("buildEnvApply failed: %v", err)
	}

	if len(result) != 1 || result[0].Name != "KEEP" {
		t.Error("should only have KEEP env var")
	}
}

func TestBuildEnvApply_ProtectedAdd(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.AddEnvVars = []openclawv1alpha1.SelfConfigEnvVar{
		{Name: "HOME", Value: "/hacked"},
	}

	_, err := buildEnvApply(nil, sc)
	if err == nil {
		t.Error("expected error for protected env var HOME")
	}
}

func TestBuildEnvApply_ProtectedRemove(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.RemoveEnvVars = []string{"OPENCLAW_GATEWAY_TOKEN"}

	_, err := buildEnvApply(nil, sc)
	if err == nil {
		t.Error("expected error for removing protected env var OPENCLAW_GATEWAY_TOKEN")
	}
}

func TestBuildEnvApply_AddAndRemove(t *testing.T) {
	current := []corev1.EnvVar{
		{Name: "KEEP", Value: "yes"},
		{Name: "OLD_VAR", Value: "old"},
	}

	sc := newTestSelfConfig()
	sc.Spec.RemoveEnvVars = []string{"OLD_VAR"}
	sc.Spec.AddEnvVars = []openclawv1alpha1.SelfConfigEnvVar{
		{Name: "NEW_VAR", Value: "new"},
	}

	result, err := buildEnvApply(current, sc)
	if err != nil {
		t.Fatalf("buildEnvApply failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(result))
	}
	if result[0].Name != "KEEP" || result[1].Name != "NEW_VAR" {
		t.Errorf("expected [KEEP, NEW_VAR], got [%s, %s]", result[0].Name, result[1].Name)
	}
}

func TestDeepMerge(t *testing.T) {
	dst := map[string]interface{}{
		"a": "1",
		"b": map[string]interface{}{
			"c": "2",
			"d": "3",
		},
	}
	src := map[string]interface{}{
		"b": map[string]interface{}{
			"d": "4",
			"e": "5",
		},
		"f": "6",
	}

	result := deepMerge(dst, src)

	if result["a"] != "1" {
		t.Error("a should be preserved")
	}
	b, ok := result["b"].(map[string]interface{})
	if !ok {
		t.Fatal("b should be a map")
	}
	if b["c"] != "2" {
		t.Error("b.c should be preserved")
	}
	if b["d"] != "4" {
		t.Error("b.d should be overwritten by src")
	}
	if b["e"] != "5" {
		t.Error("b.e should be added from src")
	}
	if result["f"] != "6" {
		t.Error("f should be added from src")
	}
}

func TestBuildSkillsApply_RemoveAll_NonNilSlice(t *testing.T) {
	sc := newTestSelfConfig()
	sc.Spec.RemoveSkills = []string{"skill-a", "skill-b"}

	result := buildSkillsApply([]string{"skill-a", "skill-b"}, sc)

	if result == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

func TestBuildEnvApply_RemoveAll_NonNilSlice(t *testing.T) {
	current := []corev1.EnvVar{
		{Name: "VAR_A", Value: "a"},
		{Name: "VAR_B", Value: "b"},
	}

	sc := newTestSelfConfig()
	sc.Spec.RemoveEnvVars = []string{"VAR_A", "VAR_B"}

	result, err := buildEnvApply(current, sc)
	if err != nil {
		t.Fatalf("buildEnvApply failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

func TestBuildApplySpec_AllActions(t *testing.T) {
	instance := newTestInstance()
	instance.Spec.Skills = []string{"existing-skill"}
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"key":"value"}`)},
	}
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{"file.md": "content"},
	}
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "EXISTING", Value: "val"},
	}

	sc := newTestSelfConfig()
	sc.Spec.AddSkills = []string{"new-skill"}
	sc.Spec.ConfigPatch = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"newKey":"newValue"}`)},
	}
	sc.Spec.AddWorkspaceFiles = map[string]string{"new.md": "new"}
	sc.Spec.AddEnvVars = []openclawv1alpha1.SelfConfigEnvVar{
		{Name: "NEW_VAR", Value: "new_val"},
	}

	actions := []openclawv1alpha1.SelfConfigAction{
		openclawv1alpha1.SelfConfigActionSkills,
		openclawv1alpha1.SelfConfigActionConfig,
		openclawv1alpha1.SelfConfigActionWorkspaceFiles,
		openclawv1alpha1.SelfConfigActionEnvVars,
	}

	spec, err := buildApplySpec(instance, sc, actions)
	if err != nil {
		t.Fatalf("buildApplySpec failed: %v", err)
	}

	// Verify skills
	if len(spec.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(spec.Skills))
	}

	// Verify config
	if spec.Config.Raw == nil {
		t.Fatal("expected config to be set")
	}
	var config map[string]interface{}
	if err := json.Unmarshal(spec.Config.Raw.Raw, &config); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	if config["key"] != "value" {
		t.Error("existing config key should be preserved")
	}
	if config["newKey"] != "newValue" {
		t.Error("new config key should be added")
	}

	// Verify workspace
	if spec.Workspace == nil {
		t.Fatal("expected workspace to be set")
	}
	if len(spec.Workspace.InitialFiles) != 2 {
		t.Errorf("expected 2 workspace files, got %d", len(spec.Workspace.InitialFiles))
	}

	// Verify env
	if len(spec.Env) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(spec.Env))
	}
}

func TestBuildApplySpec_ProtectedConfigKey(t *testing.T) {
	instance := newTestInstance()

	sc := newTestSelfConfig()
	sc.Spec.ConfigPatch = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"gateway":{"token":"bad"}}`)},
	}

	actions := []openclawv1alpha1.SelfConfigAction{
		openclawv1alpha1.SelfConfigActionConfig,
	}

	_, err := buildApplySpec(instance, sc, actions)
	if err == nil {
		t.Error("expected error for protected config key")
	}
}

func TestBuildApplySpec_ProtectedEnvVar(t *testing.T) {
	instance := newTestInstance()

	sc := newTestSelfConfig()
	sc.Spec.AddEnvVars = []openclawv1alpha1.SelfConfigEnvVar{
		{Name: "PATH", Value: "/hacked"},
	}

	actions := []openclawv1alpha1.SelfConfigAction{
		openclawv1alpha1.SelfConfigActionEnvVars,
	}

	_, err := buildApplySpec(instance, sc, actions)
	if err == nil {
		t.Error("expected error for protected env var")
	}
}
