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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testPacks(files map[string]string) *ResolvedSkillPacks {
	sp := &ResolvedSkillPacks{
		Files:       make(map[string]string),
		PathMapping: make(map[string]string),
	}
	for wsPath, content := range files {
		cmKey := SkillPackCMKey(wsPath)
		sp.Files[cmKey] = content
		sp.PathMapping[cmKey] = wsPath
	}
	return sp
}

// TestBuildInitScript_SkillPacks_ReplaceDefault verifies that the default
// policy (empty or "Replace") copies pack files unconditionally and maintains
// the manifest, so a pinned pack revision bump refreshes seeded contents (#564).
func TestBuildInitScript_SkillPacks_ReplaceDefault(t *testing.T) {
	instance := newTestInstance("pack-replace")
	packs := testPacks(map[string]string{
		"skills/example/SKILL.md": "# v2",
	})

	script := BuildInitScript(instance, nil, nil, packs)

	if strings.Contains(script, "[ -f /data/workspace/'skills/example/SKILL.md' ] ||") {
		t.Error("Replace policy must not seed pack files conditionally")
	}
	if !strings.Contains(script, "cp /workspace-init/'skills--example--SKILL.md' /data/workspace/'skills/example/SKILL.md'") {
		t.Errorf("expected unconditional cp for the pack file, got:\n%s", script)
	}
	if !strings.Contains(script, "printf '%s\\n' 'skills/example/SKILL.md' > /data/.skillpack-manifest.new") {
		t.Errorf("expected manifest write, got:\n%s", script)
	}
	if !strings.Contains(script, "mv /data/.skillpack-manifest.new /data/.skillpack-manifest") {
		t.Errorf("expected manifest promotion, got:\n%s", script)
	}
	if !strings.Contains(script, "mkdir -p /data/workspace/'skills/example'") {
		t.Errorf("expected parent dir creation for the pack file, got:\n%s", script)
	}
}

// TestBuildInitScript_SkillPacks_CreateOnly verifies the legacy opt-out keeps
// seed-if-absent semantics and does not touch the manifest.
func TestBuildInitScript_SkillPacks_CreateOnly(t *testing.T) {
	instance := newTestInstance("pack-createonly")
	instance.Spec.SkillPackUpdatePolicy = SkillPackUpdatePolicyCreateOnly
	packs := testPacks(map[string]string{
		"skills/example/SKILL.md": "# v1",
	})

	script := BuildInitScript(instance, nil, nil, packs)

	if !strings.Contains(script, "[ -f /data/workspace/'skills/example/SKILL.md' ] || cp /workspace-init/'skills--example--SKILL.md' /data/workspace/'skills/example/SKILL.md'") {
		t.Errorf("CreateOnly policy should seed pack files only when absent, got:\n%s", script)
	}
	if strings.Contains(script, ".skillpack-manifest") {
		t.Errorf("CreateOnly policy must not touch the skill pack manifest, got:\n%s", script)
	}
}

// TestBuildInitScript_SkillPacksRemoved_Cleanup verifies that with no packs
// declared, the script still cleans up files recorded in a manifest left by a
// previously declared pack, then removes the manifest.
func TestBuildInitScript_SkillPacksRemoved_Cleanup(t *testing.T) {
	instance := newTestInstance("pack-removed")

	script := BuildInitScript(instance, nil, nil, nil)

	if !strings.Contains(script, "if [ -f /data/.skillpack-manifest ]; then") {
		t.Errorf("expected manifest-guarded cleanup, got:\n%s", script)
	}
	if !strings.Contains(script, "rm -f /data/.skillpack-manifest.new /data/.skillpack-manifest") {
		t.Errorf("expected manifest removal after cleanup, got:\n%s", script)
	}
}

// runSkillPackSync renders the Replace-mode sync for the given pack files,
// rebases the hardcoded /data and /workspace-init paths into the test's temp
// dirs, and executes it with sh. This exercises the actual shell semantics:
// unconditional copy, stale-file removal, and empty-dir pruning.
func runSkillPackSync(t *testing.T, dataDir, initDir string, files map[string]string) {
	t.Helper()

	packs := testPacks(files)
	// Materialize the "ConfigMap" contents the init container would see.
	for cmKey, content := range packs.Files {
		if err := os.WriteFile(filepath.Join(initDir, cmKey), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var sync []string
	if len(files) == 0 {
		sync = buildSkillPackSyncLines(nil)
	} else {
		sync = buildSkillPackSyncLines(packs)
	}
	script := strings.Join(sync, "\n")
	script = strings.ReplaceAll(script, "/data", dataDir)
	script = strings.ReplaceAll(script, "/workspace-init", initDir)

	cmd := exec.Command("sh", "-c", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sync script failed: %v\noutput: %s\nscript:\n%s", err, out, script)
	}
}

// TestSkillPackSync_ShellBehavior seeds a pack at revision 1, then re-runs the
// sync for revision 2 (changed file, removed file, added file) and verifies
// the workspace converges: contents refreshed, stale file and its now-empty
// directory removed, unrelated user files untouched (#564).
func TestSkillPackSync_ShellBehavior(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	dataDir := t.TempDir()
	initDir := t.TempDir()
	wsDir := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// A user-owned file the sync must never touch.
	userFile := filepath.Join(wsDir, "SOUL.md")
	if err := os.WriteFile(userFile, []byte("user content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Revision 1: two files in one skill dir.
	runSkillPackSync(t, dataDir, initDir, map[string]string{
		"skills/gog-gmail-mcp/SKILL.md":  "# rev1",
		"skills/gog-gmail-mcp/server.py": "print('gmail only')",
	})

	got, err := os.ReadFile(filepath.Join(wsDir, "skills", "gog-gmail-mcp", "SKILL.md"))
	if err != nil || string(got) != "# rev1" {
		t.Fatalf("rev1 seed failed: %v, content %q", err, got)
	}

	// Revision 2: skill renamed, one file changed, old paths gone.
	runSkillPackSync(t, dataDir, initDir, map[string]string{
		"skills/gog-google-mcp/SKILL.md":  "# rev2",
		"skills/gog-google-mcp/server.py": "print('gmail+calendar')",
	})

	got, err = os.ReadFile(filepath.Join(wsDir, "skills", "gog-google-mcp", "server.py"))
	if err != nil || string(got) != "print('gmail+calendar')" {
		t.Fatalf("rev2 content not converged: %v, content %q", err, got)
	}
	if _, err := os.Stat(filepath.Join(wsDir, "skills", "gog-gmail-mcp", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("stale rev1 file should be removed")
	}
	if _, err := os.Stat(filepath.Join(wsDir, "skills", "gog-gmail-mcp")); !os.IsNotExist(err) {
		t.Error("empty stale skill directory should be pruned")
	}
	if got, _ := os.ReadFile(userFile); string(got) != "user content" {
		t.Error("user file must not be touched by pack sync")
	}

	// Same revision re-run must be idempotent and keep files identical.
	runSkillPackSync(t, dataDir, initDir, map[string]string{
		"skills/gog-google-mcp/SKILL.md":  "# rev2",
		"skills/gog-google-mcp/server.py": "print('gmail+calendar')",
	})
	if _, err := os.Stat(filepath.Join(wsDir, "skills", "gog-google-mcp", "SKILL.md")); err != nil {
		t.Errorf("idempotent re-run should keep seeded files: %v", err)
	}

	// All packs removed: seeded files cleaned up, manifest dropped, user file kept.
	runSkillPackSync(t, dataDir, initDir, nil)
	if _, err := os.Stat(filepath.Join(wsDir, "skills", "gog-google-mcp")); !os.IsNotExist(err) {
		t.Error("removing all packs should clean up seeded files")
	}
	if _, err := os.Stat(filepath.Join(dataDir, ".skillpack-manifest")); !os.IsNotExist(err) {
		t.Error("manifest should be removed when no packs are declared")
	}
	if got, _ := os.ReadFile(userFile); string(got) != "user content" {
		t.Error("user file must survive full pack removal")
	}

	// Overwrite semantics: local edits to a pack-managed file are reverted.
	seeded := filepath.Join(wsDir, "skills", "pinned", "SKILL.md")
	runSkillPackSync(t, dataDir, initDir, map[string]string{"skills/pinned/SKILL.md": "# pinned"})
	if err := os.WriteFile(seeded, []byte("local drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	runSkillPackSync(t, dataDir, initDir, map[string]string{"skills/pinned/SKILL.md": "# pinned"})
	if got, _ := os.ReadFile(seeded); string(got) != "# pinned" {
		t.Errorf("Replace policy should converge drifted file, got %q", got)
	}
}
