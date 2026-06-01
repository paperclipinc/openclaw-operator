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

package skillpacks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

// ghFile returns a GitHub Contents API JSON response for a file.
func ghFile(content string) string {
	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	resp := contentsResponse{Content: b64, Encoding: "base64"}
	data, _ := json.Marshal(resp)
	return string(data)
}

// newTestResolver creates a resolver pointing at the httptest server.
func newTestResolver(server *httptest.Server, token string) *Resolver {
	r := NewResolver(5*time.Minute, token)
	r.baseURL = server.URL
	return r
}

func TestParsePackRef(t *testing.T) {
	tests := []struct {
		input   string
		owner   string
		repo    string
		path    string
		ref     string
		wantErr bool
	}{
		{"paperclipinc/skills/image-gen", "paperclipinc", "skills", "image-gen", "", false},
		{"paperclipinc/skills/image-gen@v1.0.0", "paperclipinc", "skills", "image-gen", "v1.0.0", false},
		{"myorg/private-skills/custom-tool@main", "myorg", "private-skills", "custom-tool", "main", false},
		{"paperclipinc/skills/nested/deep/path@abc123", "paperclipinc", "skills", "nested/deep/path", "abc123", false},
		{"owner/repo", "", "", "", "", true},
		{"invalid", "", "", "", "", true},
	}

	for _, tt := range tests {
		ref, err := parsePackRef(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parsePackRef(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePackRef(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if ref.Owner != tt.owner || ref.Repo != tt.repo || ref.Path != tt.path || ref.Ref != tt.ref {
			t.Errorf("parsePackRef(%q) = {%s, %s, %s, %s}, want {%s, %s, %s, %s}",
				tt.input, ref.Owner, ref.Repo, ref.Path, ref.Ref,
				tt.owner, tt.repo, tt.path, tt.ref)
		}
	}
}

func TestResolve_Success(t *testing.T) {
	manifestJSON := `{
		"files": {
			"skills/image-gen/SKILL.md": "SKILL.md",
			"skills/image-gen/scripts/generate.py": "scripts/generate.py"
		},
		"directories": ["skills/image-gen/scripts"],
		"config": {
			"image-gen": {"enabled": true},
			"openai-image-gen": {"enabled": false}
		}
	}`

	skillMD := "---\nname: image-gen\n---\n"
	generatePy := "#!/usr/bin/env python3\nprint('hello')\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "image-gen/skillpack.json"):
			_, _ = w.Write([]byte(ghFile(manifestJSON)))
		case strings.HasSuffix(path, "image-gen/SKILL.md"):
			_, _ = w.Write([]byte(ghFile(skillMD)))
		case strings.HasSuffix(path, "image-gen/scripts/generate.py"):
			_, _ = w.Write([]byte(ghFile(generatePy)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")

	resolved, err := resolver.Resolve(context.Background(), []string{"test-owner/test-repo/image-gen"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected resolved skill packs, got nil")
	}

	// Check files
	if len(resolved.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(resolved.Files))
	}
	skillMDKey := resources.SkillPackCMKey("skills/image-gen/SKILL.md")
	if resolved.Files[skillMDKey] != skillMD {
		t.Errorf("unexpected SKILL.md content: %q", resolved.Files[skillMDKey])
	}
	genPyKey := resources.SkillPackCMKey("skills/image-gen/scripts/generate.py")
	if resolved.Files[genPyKey] != generatePy {
		t.Errorf("unexpected generate.py content: %q", resolved.Files[genPyKey])
	}

	// Check path mapping
	if resolved.PathMapping[skillMDKey] != "skills/image-gen/SKILL.md" {
		t.Errorf("unexpected path mapping: %q", resolved.PathMapping[skillMDKey])
	}
	if resolved.PathMapping[genPyKey] != "skills/image-gen/scripts/generate.py" {
		t.Errorf("unexpected path mapping: %q", resolved.PathMapping[genPyKey])
	}

	// Check directories
	if len(resolved.Directories) != 1 || resolved.Directories[0] != "skills/image-gen/scripts" {
		t.Errorf("unexpected directories: %v", resolved.Directories)
	}

	// Check config
	if len(resolved.SkillEntries) != 2 {
		t.Errorf("expected 2 skill entries, got %d", len(resolved.SkillEntries))
	}
	imgGen, ok := resolved.SkillEntries["image-gen"].(map[string]interface{})
	if !ok || imgGen["enabled"] != true {
		t.Errorf("expected image-gen enabled: %v", resolved.SkillEntries["image-gen"])
	}
}

func TestResolve_Empty(t *testing.T) {
	resolver := NewResolver(5*time.Minute, "")
	resolved, err := resolver.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != nil {
		t.Errorf("expected nil for empty pack names, got %v", resolved)
	}
}

func TestResolve_MissingManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")

	_, err := resolver.Resolve(context.Background(), []string{"owner/repo/missing-pack"})
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolve_MissingFile(t *testing.T) {
	manifestJSON := `{
		"files": {"skills/test/SKILL.md": "SKILL.md"},
		"directories": [],
		"config": {}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "skillpack.json") {
			_, _ = w.Write([]byte(ghFile(manifestJSON)))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")

	_, err := resolver.Resolve(context.Background(), []string{"owner/repo/test-pack"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolve_Caching(t *testing.T) {
	callCount := 0
	manifestJSON := `{"files": {}, "directories": [], "config": {}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		_, _ = w.Write([]byte(ghFile(manifestJSON)))
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")

	// First call
	_, err := resolver.Resolve(context.Background(), []string{"owner/repo/test"})
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	firstCallCount := callCount

	// Second call should use cache
	_, err = resolver.Resolve(context.Background(), []string{"owner/repo/test"})
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if callCount != firstCallCount {
		t.Errorf("expected cached result, but got %d additional API calls", callCount-firstCallCount)
	}
}

func TestResolve_AuthHeader(t *testing.T) {
	var gotAuth string
	manifestJSON := `{"files": {}, "directories": [], "config": {}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(ghFile(manifestJSON)))
	}))
	defer server.Close()

	resolver := newTestResolver(server, "ghp_test_token_123")

	_, err := resolver.Resolve(context.Background(), []string{"owner/repo/test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotAuth != "Bearer ghp_test_token_123" {
		t.Errorf("expected auth header 'Bearer ghp_test_token_123', got %q", gotAuth)
	}
}

func TestResolve_NoAuthWhenTokenEmpty(t *testing.T) {
	var gotAuth string
	manifestJSON := `{"files": {}, "directories": [], "config": {}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(ghFile(manifestJSON)))
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")

	_, err := resolver.Resolve(context.Background(), []string{"owner/repo/test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotAuth != "" {
		t.Errorf("expected no auth header, got %q", gotAuth)
	}
}

func TestResolve_WithRef(t *testing.T) {
	var gotRef string
	manifestJSON := `{"files": {}, "directories": [], "config": {}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRef = r.URL.Query().Get("ref")
		_, _ = w.Write([]byte(ghFile(manifestJSON)))
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")

	_, err := resolver.Resolve(context.Background(), []string{"owner/repo/test@v1.2.3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotRef != "v1.2.3" {
		t.Errorf("expected ref 'v1.2.3', got %q", gotRef)
	}
}

func TestResolve_InvalidRef(t *testing.T) {
	resolver := NewResolver(5*time.Minute, "")

	_, err := resolver.Resolve(context.Background(), []string{"invalid"})
	if err == nil {
		t.Fatal("expected error for invalid pack reference")
	}
	if !strings.Contains(err.Error(), "invalid pack reference") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolve_StaleCacheFallback(t *testing.T) {
	manifestJSON := `{"files": {}, "directories": ["skills/test"], "config": {"test-skill": {"enabled": true}}}`
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount > 1 {
			// Simulate GitHub outage on subsequent requests
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(ghFile(manifestJSON)))
	}))
	defer server.Close()

	// Use a very short TTL so the cache expires quickly
	resolver := &Resolver{
		cacheTTL:   1 * time.Millisecond,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    server.URL,
		cache:      make(map[string]*cacheEntry),
	}

	// First call succeeds and populates cache
	resolved, err := resolver.Resolve(context.Background(), []string{"owner/repo/test"})
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if len(resolved.Directories) != 1 || resolved.Directories[0] != "skills/test" {
		t.Errorf("unexpected directories: %v", resolved.Directories)
	}

	// Wait for cache to expire
	time.Sleep(5 * time.Millisecond)

	// Second call fails but returns stale cache instead of error
	resolved, err = resolver.Resolve(context.Background(), []string{"owner/repo/test"})
	if err != nil {
		t.Fatalf("expected stale cache fallback, got error: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected stale cached result, got nil")
	}
	if len(resolved.Directories) != 1 || resolved.Directories[0] != "skills/test" {
		t.Errorf("unexpected stale directories: %v", resolved.Directories)
	}
}

// ghTree returns a GitHub Git Trees API JSON response.
func ghTree(truncated bool, entries ...treeEntry) string {
	resp := treeResponse{Tree: entries, Truncated: truncated}
	data, _ := json.Marshal(resp)
	return string(data)
}

func TestResolve_RawRepo_SingleSkillWithNestedFiles(t *testing.T) {
	// Simulates fluxcd/agent-skills layout:
	//   skills/gitops-repo-audit/SKILL.md
	//   skills/gitops-repo-audit/assets/schemas/kustomization.json
	skillMD := "# Audit skill\n"
	schema := `{"type":"object"}`

	treeJSON := ghTree(false,
		treeEntry{Path: "skills", Type: "tree"},
		treeEntry{Path: "skills/gitops-repo-audit", Type: "tree"},
		treeEntry{Path: "skills/gitops-repo-audit/SKILL.md", Type: "blob"},
		treeEntry{Path: "skills/gitops-repo-audit/assets", Type: "tree"},
		treeEntry{Path: "skills/gitops-repo-audit/assets/schemas", Type: "tree"},
		treeEntry{Path: "skills/gitops-repo-audit/assets/schemas/kustomization.json", Type: "blob"},
		// Sibling skill that must NOT be picked up
		treeEntry{Path: "skills/other-skill/SKILL.md", Type: "blob"},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/skills/gitops-repo-audit/skillpack.json"):
			// No manifest — trigger raw-repo fallback
			http.NotFound(w, r)
		case strings.HasSuffix(path, "/git/trees/HEAD"):
			_, _ = w.Write([]byte(treeJSON))
		case strings.HasSuffix(path, "/contents/skills/gitops-repo-audit/SKILL.md"):
			_, _ = w.Write([]byte(ghFile(skillMD)))
		case strings.HasSuffix(path, "/contents/skills/gitops-repo-audit/assets/schemas/kustomization.json"):
			_, _ = w.Write([]byte(ghFile(schema)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")
	resolved, err := resolver.Resolve(context.Background(),
		[]string{"fluxcd/agent-skills/skills/gitops-repo-audit"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Two blobs should be seeded under skills/gitops-repo-audit/
	if len(resolved.Files) != 2 {
		t.Fatalf("expected 2 files, got %d: %+v", len(resolved.Files), resolved.PathMapping)
	}

	skillKey := resources.SkillPackCMKey("skills/gitops-repo-audit/SKILL.md")
	if resolved.Files[skillKey] != skillMD {
		t.Errorf("SKILL.md content mismatch: %q", resolved.Files[skillKey])
	}
	if resolved.PathMapping[skillKey] != "skills/gitops-repo-audit/SKILL.md" {
		t.Errorf("SKILL.md workspace path mismatch: %q", resolved.PathMapping[skillKey])
	}

	schemaKey := resources.SkillPackCMKey("skills/gitops-repo-audit/assets/schemas/kustomization.json")
	if resolved.Files[schemaKey] != schema {
		t.Errorf("schema content mismatch: %q", resolved.Files[schemaKey])
	}
	if resolved.PathMapping[schemaKey] != "skills/gitops-repo-audit/assets/schemas/kustomization.json" {
		t.Errorf("schema workspace path mismatch: %q", resolved.PathMapping[schemaKey])
	}

	// Directories should include the workspace root and every nested dir so
	// the init container's mkdir -p creates parents before cp.
	want := map[string]bool{
		"skills/gitops-repo-audit":                true,
		"skills/gitops-repo-audit/assets":         true,
		"skills/gitops-repo-audit/assets/schemas": true,
	}
	got := make(map[string]bool, len(resolved.Directories))
	for _, d := range resolved.Directories {
		got[d] = true
	}
	for d := range want {
		if !got[d] {
			t.Errorf("expected directory %q in resolved.Directories (got %v)", d, resolved.Directories)
		}
	}

	// Sibling skill path must not leak in.
	for cmKey, wsPath := range resolved.PathMapping {
		if strings.Contains(wsPath, "other-skill") || strings.Contains(cmKey, "other-skill") {
			t.Errorf("sibling skill leaked into result: %q -> %q", cmKey, wsPath)
		}
	}

	// Raw mode does not inject config entries.
	if len(resolved.SkillEntries) != 0 {
		t.Errorf("expected no skill entries in raw mode, got %v", resolved.SkillEntries)
	}
}

func TestResolve_RawRepo_NoSkillMD(t *testing.T) {
	// Tree has files but no SKILL.md at the pack path -- raw install must refuse.
	treeJSON := ghTree(false,
		treeEntry{Path: "skills", Type: "tree"},
		treeEntry{Path: "skills/no-manifest", Type: "tree"},
		treeEntry{Path: "skills/no-manifest/README.md", Type: "blob"},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "skillpack.json") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "/git/trees/") {
			_, _ = w.Write([]byte(treeJSON))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")
	_, err := resolver.Resolve(context.Background(), []string{"owner/repo/skills/no-manifest"})
	if err == nil {
		t.Fatal("expected error when no skillpack.json and no SKILL.md")
	}
	if !strings.Contains(err.Error(), "SKILL.md") {
		t.Errorf("error should mention SKILL.md: %v", err)
	}
}

func TestResolve_RawRepo_TruncatedTree(t *testing.T) {
	// GitHub returns truncated=true when the tree is too large. Refuse and
	// tell the user to use a skillpack.json manifest instead.
	treeJSON := ghTree(true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "skillpack.json") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "/git/trees/") {
			_, _ = w.Write([]byte(treeJSON))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")
	_, err := resolver.Resolve(context.Background(), []string{"owner/huge-repo/skills/foo"})
	if err == nil {
		t.Fatal("expected error for truncated tree")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("error should mention truncation: %v", err)
	}
}

func TestResolve_RawRepo_RefPassedThrough(t *testing.T) {
	// The user-supplied ref must flow to both the tree lookup and each
	// blob fetch, so the pinned version is consistent across the install.
	var gotTreeRef string
	var gotContentRefs []string
	skillMD := "# skill\n"
	treeJSON := ghTree(false,
		treeEntry{Path: "my-skill", Type: "tree"},
		treeEntry{Path: "my-skill/SKILL.md", Type: "blob"},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/skillpack.json"):
			http.NotFound(w, r)
		case strings.Contains(path, "/git/trees/"):
			// URL form: .../git/trees/<ref>
			parts := strings.Split(path, "/git/trees/")
			gotTreeRef = parts[1]
			_, _ = w.Write([]byte(treeJSON))
		case strings.Contains(path, "/contents/"):
			gotContentRefs = append(gotContentRefs, r.URL.Query().Get("ref"))
			_, _ = w.Write([]byte(ghFile(skillMD)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")
	_, err := resolver.Resolve(context.Background(), []string{"owner/repo/my-skill@v2.3.4"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotTreeRef != "v2.3.4" {
		t.Errorf("expected tree ref 'v2.3.4', got %q", gotTreeRef)
	}
	for _, ref := range gotContentRefs {
		if ref != "v2.3.4" {
			t.Errorf("expected content ref 'v2.3.4', got %q", ref)
		}
	}
}

func TestResolve_NoCacheFallbackOnFirstFailure(t *testing.T) {
	// When there is no cached data at all, errors should propagate
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	resolver := newTestResolver(server, "")

	_, err := resolver.Resolve(context.Background(), []string{"owner/repo/test"})
	if err == nil {
		t.Fatal("expected error when no cache exists and fetch fails")
	}
}
