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

package resources

import (
	"encoding/json"
	"strings"
	"testing"
)

// Note: ConfigMap-based ResolveSkillPacks tests removed — resolution is now
// handled by the skillpacks.Resolver (GitHub-based) with its own test suite.

func TestExtractPackSkills(t *testing.T) {
	skills := []string{
		"@anthropic/mcp-server-fetch",
		"npm:@openclaw/matrix",
		"pack:image-gen",
		"pack:code-runner",
	}
	packs := ExtractPackSkills(skills)
	if len(packs) != 2 {
		t.Fatalf("expected 2 packs, got %d", len(packs))
	}
	if packs[0] != "image-gen" || packs[1] != "code-runner" {
		t.Errorf("unexpected packs: %v", packs)
	}
}

func TestExtractPackSkills_None(t *testing.T) {
	skills := []string{"@anthropic/fetch", "npm:pkg"}
	packs := ExtractPackSkills(skills)
	if len(packs) != 0 {
		t.Fatalf("expected 0 packs, got %d", len(packs))
	}
}

func TestFilterNonPackSkills(t *testing.T) {
	skills := []string{
		"@anthropic/fetch",
		"pack:image-gen",
		"npm:@openclaw/matrix",
		"pack:code-runner",
	}
	filtered := FilterNonPackSkills(skills)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 non-pack skills, got %d", len(filtered))
	}
	if filtered[0] != "@anthropic/fetch" || filtered[1] != "npm:@openclaw/matrix" {
		t.Errorf("unexpected filtered: %v", filtered)
	}
}

func TestSkillPackCMKey(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"SKILL.md", "SKILL.md"},
		{"skills/image-gen/SKILL.md", "skills--image-gen--SKILL.md"},
		{"skills/image-gen/scripts/generate.py", "skills--image-gen--scripts--generate.py"},
	}
	for _, tt := range tests {
		if got := SkillPackCMKey(tt.input); got != tt.expected {
			t.Errorf("SkillPackCMKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestHasSkillPackFiles(t *testing.T) {
	if HasSkillPackFiles(nil) {
		t.Error("expected false for nil")
	}
	if HasSkillPackFiles(&ResolvedSkillPacks{}) {
		t.Error("expected false for empty")
	}
	if !HasSkillPackFiles(&ResolvedSkillPacks{Files: map[string]string{"a": "b"}}) {
		t.Error("expected true for non-empty files")
	}
}

func TestBuildSkillsScript_FiltersPackEntries(t *testing.T) {
	instance := newTestInstance("skills-filter")
	instance.Spec.Skills = []string{
		"pack:image-gen",
		"npm:@openclaw/matrix",
		"@anthropic/mcp-server-fetch",
	}
	script := BuildSkillsScript(instance)
	if strings.Contains(script, "image-gen") {
		t.Error("script should not contain pack:image-gen")
	}
	if !strings.Contains(script, "npm install") {
		t.Error("script should contain npm install for @openclaw/matrix")
	}
	if !strings.Contains(script, "_install_skill") {
		t.Error("script should contain _install_skill for @anthropic/mcp-server-fetch")
	}
}

func TestBuildSkillsScript_OnlyPackEntries(t *testing.T) {
	instance := newTestInstance("skills-only-packs")
	instance.Spec.Skills = []string{"pack:image-gen", "pack:code-runner"}
	script := BuildSkillsScript(instance)
	if script != "" {
		t.Errorf("expected empty script for pack-only skills, got: %s", script)
	}
}

func TestBuildInitScript_WithSkillPacks(t *testing.T) {
	instance := newTestInstance("skill-pack-init")
	instance.Spec.Workspace = nil
	instance.Spec.Config.Raw = nil

	resolved := &ResolvedSkillPacks{
		Files: map[string]string{
			"skills--image-gen--SKILL.md":             "skill content",
			"skills--image-gen--scripts--generate.py": "script content",
		},
		PathMapping: map[string]string{
			"skills--image-gen--SKILL.md":             "skills/image-gen/SKILL.md",
			"skills--image-gen--scripts--generate.py": "skills/image-gen/scripts/generate.py",
		},
		Directories: []string{"skills/image-gen/scripts"},
	}

	script := BuildInitScript(instance, nil, nil, resolved)

	// Should create directories
	if !strings.Contains(script, "mkdir -p /data/workspace/'skills/image-gen/scripts'") {
		t.Error("expected mkdir for skill pack directory")
	}

	// Should copy files with path mapping
	if !strings.Contains(script, "cp /workspace-init/'skills--image-gen--SKILL.md' /data/workspace/'skills/image-gen/SKILL.md'") {
		t.Errorf("expected skill pack file copy in script:\n%s", script)
	}
	if !strings.Contains(script, "cp /workspace-init/'skills--image-gen--scripts--generate.py' /data/workspace/'skills/image-gen/scripts/generate.py'") {
		t.Errorf("expected skill pack script copy in script:\n%s", script)
	}
}

func TestBuildWorkspaceConfigMap_WithSkillPacks(t *testing.T) {
	instance := newTestInstance("ws-skill-packs")

	resolved := &ResolvedSkillPacks{
		Files: map[string]string{
			"skills--image-gen--SKILL.md": "skill content",
		},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, resolved)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap")
	}
	if cm.Data["skills--image-gen--SKILL.md"] != "skill content" {
		t.Errorf("expected skill pack file in ConfigMap data")
	}
}

func TestEnrichConfigWithSkillPacks(t *testing.T) {
	config := `{"skills": {"entries": {"web-search": {"enabled": true}}}}`
	skillEntries := map[string]interface{}{
		"image-gen":        map[string]interface{}{"enabled": true},
		"openai-image-gen": map[string]interface{}{"enabled": false},
	}

	enriched, err := enrichConfigWithSkillPacks([]byte(config), skillEntries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(enriched, &result); err != nil {
		t.Fatalf("failed to parse enriched config: %v", err)
	}

	skills := result["skills"].(map[string]interface{})
	entries := skills["entries"].(map[string]interface{})

	// User-defined entry should be preserved
	if ws, ok := entries["web-search"].(map[string]interface{}); !ok || ws["enabled"] != true {
		t.Error("web-search entry should be preserved")
	}
	// Skill pack entries should be added
	if ig, ok := entries["image-gen"].(map[string]interface{}); !ok || ig["enabled"] != true {
		t.Error("image-gen should be enabled")
	}
	if oig, ok := entries["openai-image-gen"].(map[string]interface{}); !ok || oig["enabled"] != false {
		t.Error("openai-image-gen should be disabled")
	}
}

func TestEnrichConfigWithSkillPacks_UserOverrideWins(t *testing.T) {
	// User already disabled image-gen — skill pack should NOT override
	config := `{"skills": {"entries": {"image-gen": {"enabled": false}}}}`
	skillEntries := map[string]interface{}{
		"image-gen": map[string]interface{}{"enabled": true},
	}

	enriched, err := enrichConfigWithSkillPacks([]byte(config), skillEntries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(enriched, &result); err != nil {
		t.Fatalf("failed to parse enriched config: %v", err)
	}
	entries := result["skills"].(map[string]interface{})["entries"].(map[string]interface{})
	ig := entries["image-gen"].(map[string]interface{})
	if ig["enabled"] != false {
		t.Error("user override should win — image-gen should remain disabled")
	}
}
