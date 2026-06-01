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
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

const defaultBaseURL = "https://api.github.com"

// errNotFound is returned by fetchFile / fetchTree when GitHub responds 404.
// Wrapping allows callers to detect a missing resource (e.g. missing skillpack.json)
// without relying on error message substrings.
var errNotFound = errors.New("not found")

// Resolver fetches skill pack manifests and files from GitHub repositories.
type Resolver struct {
	cacheTTL    time.Duration
	httpClient  *http.Client
	githubToken string
	baseURL     string // GitHub API base URL (overridable for tests)

	mu    sync.RWMutex
	cache map[string]*cacheEntry
}

type cacheEntry struct {
	resolved  *resources.ResolvedSkillPacks
	fetchedAt time.Time
}

// packRef is a parsed pack:owner/repo/path[@ref] reference.
type packRef struct {
	Owner string
	Repo  string
	Path  string
	Ref   string // empty means default branch
}

// manifest is the skillpack.json structure.
type manifest struct {
	Files       map[string]string      `json:"files"`
	Directories []string               `json:"directories"`
	Config      map[string]interface{} `json:"config"`
}

// contentsResponse is the GitHub Contents API response for a single file.
type contentsResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// treeEntry is one entry in a GitHub Git Trees API response.
type treeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"` // "blob" or "tree"
}

// treeResponse is the GitHub Git Trees API response.
type treeResponse struct {
	Tree      []treeEntry `json:"tree"`
	Truncated bool        `json:"truncated"`
}

// NewResolver creates a new GitHub-based skill pack resolver.
func NewResolver(cacheTTL time.Duration, githubToken string) *Resolver {
	return &Resolver{
		cacheTTL:    cacheTTL,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		githubToken: githubToken,
		baseURL:     defaultBaseURL,
		cache:       make(map[string]*cacheEntry),
	}
}

// Resolve fetches and resolves the given pack names from GitHub.
// Returns nil if packNames is empty.
func (r *Resolver) Resolve(ctx context.Context, packNames []string) (*resources.ResolvedSkillPacks, error) {
	if len(packNames) == 0 {
		return nil, nil
	}

	// Sort for deterministic output
	sorted := make([]string, len(packNames))
	copy(sorted, packNames)
	sort.Strings(sorted)

	merged := &resources.ResolvedSkillPacks{
		Files:        make(map[string]string),
		PathMapping:  make(map[string]string),
		SkillEntries: make(map[string]interface{}),
	}

	for _, name := range sorted {
		resolved, err := r.resolvePack(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("skill pack %q: %w", name, err)
		}

		// Merge into combined result
		for k, v := range resolved.Files {
			merged.Files[k] = v
		}
		for k, v := range resolved.PathMapping {
			merged.PathMapping[k] = v
		}
		merged.Directories = append(merged.Directories, resolved.Directories...)
		for k, v := range resolved.SkillEntries {
			merged.SkillEntries[k] = v
		}
	}

	// Deduplicate and sort directories
	dirSet := make(map[string]bool)
	for _, d := range merged.Directories {
		dirSet[d] = true
	}
	merged.Directories = nil
	for d := range dirSet {
		merged.Directories = append(merged.Directories, d)
	}
	sort.Strings(merged.Directories)

	return merged, nil
}

// resolvePack resolves a single pack reference, using cache if valid.
// If fetching fails and a stale cache entry exists, returns the stale data
// instead of an error so that transient GitHub outages do not block reconciliation.
func (r *Resolver) resolvePack(ctx context.Context, name string) (*resources.ResolvedSkillPacks, error) {
	r.mu.RLock()
	if entry, ok := r.cache[name]; ok && time.Since(entry.fetchedAt) < r.cacheTTL {
		resolved := entry.resolved
		r.mu.RUnlock()
		return resolved, nil
	}
	r.mu.RUnlock()

	resolved, err := r.fetchPack(ctx, name)
	if err != nil {
		// Stale cache fallback - return expired data rather than failing
		r.mu.RLock()
		if entry, ok := r.cache[name]; ok {
			stale := entry.resolved
			r.mu.RUnlock()
			return stale, nil
		}
		r.mu.RUnlock()
		return nil, err
	}

	r.mu.Lock()
	r.cache[name] = &cacheEntry{resolved: resolved, fetchedAt: time.Now()}
	r.mu.Unlock()

	return resolved, nil
}

// fetchPack fetches a skill pack manifest and its files from GitHub.
// When skillpack.json is absent, falls back to raw-repo mode: the pack path
// is installed verbatim as a single skill rooted at skills/<basename>/, as long
// as a SKILL.md exists at the path.
func (r *Resolver) fetchPack(ctx context.Context, name string) (*resources.ResolvedSkillPacks, error) {
	ref, err := parsePackRef(name)
	if err != nil {
		return nil, err
	}

	// Fetch skillpack.json
	manifestPath := ref.Path + "/skillpack.json"
	manifestBytes, err := r.fetchFile(ctx, ref.Owner, ref.Repo, manifestPath, ref.Ref)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return r.fetchRawPack(ctx, ref)
		}
		return nil, fmt.Errorf("fetching skillpack.json: %w", err)
	}

	var m manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return nil, fmt.Errorf("parsing skillpack.json: %w", err)
	}

	resolved := &resources.ResolvedSkillPacks{
		Files:        make(map[string]string),
		PathMapping:  make(map[string]string),
		Directories:  m.Directories,
		SkillEntries: make(map[string]interface{}),
	}

	// Fetch each file listed in the manifest
	for wsPath, repoRelPath := range m.Files {
		filePath := ref.Path + "/" + repoRelPath
		content, err := r.fetchFile(ctx, ref.Owner, ref.Repo, filePath, ref.Ref)
		if err != nil {
			return nil, fmt.Errorf("fetching file %q: %w", repoRelPath, err)
		}
		cmKey := resources.SkillPackCMKey(wsPath)
		resolved.Files[cmKey] = string(content)
		resolved.PathMapping[cmKey] = wsPath
	}

	// Copy config entries
	for k, v := range m.Config {
		resolved.SkillEntries[k] = v
	}

	return resolved, nil
}

// fetchFile retrieves a single file from GitHub using the Contents API.
func (r *Resolver) fetchFile(ctx context.Context, owner, repo, filePath, ref string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", r.baseURL, owner, repo, filePath)
	if ref != "" {
		url += "?ref=" + ref
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if r.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.githubToken)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s/%s/%s", errNotFound, owner, repo, filePath)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s/%s/%s", resp.StatusCode, owner, repo, filePath)
	}

	var cr contentsResponse
	if decErr := json.NewDecoder(resp.Body).Decode(&cr); decErr != nil {
		return nil, fmt.Errorf("decoding response: %w", decErr)
	}

	if cr.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected encoding %q (expected base64)", cr.Encoding)
	}

	// GitHub base64 content may contain newlines
	cleaned := strings.ReplaceAll(cr.Content, "\n", "")
	data, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("decoding base64 content: %w", err)
	}

	return data, nil
}

// fetchTree retrieves the recursive Git tree for a ref. Passes the ref name
// (branch, tag, or commit SHA) to GitHub's Git Trees API, which resolves it
// to a tree SHA server-side. When ref is empty, "HEAD" is used so the default
// branch is picked.
func (r *Resolver) fetchTree(ctx context.Context, owner, repo, ref string) (*treeResponse, error) {
	treeRef := ref
	if treeRef == "" {
		treeRef = "HEAD"
	}
	url := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", r.baseURL, owner, repo, treeRef)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if r.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.githubToken)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: tree %s/%s@%s", errNotFound, owner, repo, treeRef)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for tree %s/%s@%s", resp.StatusCode, owner, repo, treeRef)
	}

	var tr treeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decoding tree response: %w", err)
	}
	return &tr, nil
}

// fetchRawPack installs the pack path as a single skill without a skillpack.json
// manifest. It lists the repository tree, verifies a SKILL.md is present at the
// pack path, and seeds every blob under the path into the workspace at
// skills/<basename>/<relative-path>.
func (r *Resolver) fetchRawPack(ctx context.Context, ref *packRef) (*resources.ResolvedSkillPacks, error) {
	tree, err := r.fetchTree(ctx, ref.Owner, ref.Repo, ref.Ref)
	if err != nil {
		return nil, fmt.Errorf("listing repository tree: %w", err)
	}
	if tree.Truncated {
		return nil, fmt.Errorf("repository tree for %s/%s is too large to enumerate (GitHub truncated the response) -- add a skillpack.json manifest at %s to install this pack", ref.Owner, ref.Repo, ref.Path)
	}

	base := path.Base(ref.Path)
	if base == "." || base == "/" || base == "" {
		return nil, fmt.Errorf("pack path %q has no basename to derive a skill directory from", ref.Path)
	}

	prefix := strings.TrimSuffix(ref.Path, "/") + "/"
	wsRoot := "skills/" + base

	var (
		blobPaths    []string
		dirs         = []string{wsRoot}
		skillMDFound bool
	)
	for _, te := range tree.Tree {
		if !strings.HasPrefix(te.Path, prefix) {
			continue
		}
		rel := strings.TrimPrefix(te.Path, prefix)
		if rel == "" {
			continue
		}
		switch te.Type {
		case "blob":
			blobPaths = append(blobPaths, te.Path)
			if rel == "SKILL.md" {
				skillMDFound = true
			}
		case "tree":
			dirs = append(dirs, wsRoot+"/"+rel)
		}
	}

	if !skillMDFound {
		return nil, fmt.Errorf("no skillpack.json and no SKILL.md found at %s/%s/%s -- raw-repo install requires a SKILL.md at the pack path", ref.Owner, ref.Repo, ref.Path)
	}

	sort.Strings(blobPaths)
	sort.Strings(dirs)

	resolved := &resources.ResolvedSkillPacks{
		Files:        make(map[string]string),
		PathMapping:  make(map[string]string),
		Directories:  dirs,
		SkillEntries: make(map[string]interface{}),
	}

	for _, blob := range blobPaths {
		rel := strings.TrimPrefix(blob, prefix)
		content, err := r.fetchFile(ctx, ref.Owner, ref.Repo, blob, ref.Ref)
		if err != nil {
			return nil, fmt.Errorf("fetching file %q: %w", blob, err)
		}
		wsPath := wsRoot + "/" + rel
		cmKey := resources.SkillPackCMKey(wsPath)
		resolved.Files[cmKey] = string(content)
		resolved.PathMapping[cmKey] = wsPath
	}

	return resolved, nil
}

// parsePackRef parses "owner/repo/path[@ref]" into its components.
func parsePackRef(name string) (*packRef, error) {
	// Split off optional @ref
	ref := ""
	atIdx := strings.LastIndex(name, "@")
	base := name
	if atIdx > 0 {
		ref = name[atIdx+1:]
		base = name[:atIdx]
	}

	// Split into owner/repo/path (minimum 3 segments)
	parts := strings.SplitN(base, "/", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid pack reference %q: expected owner/repo/path[@ref]", name)
	}

	return &packRef{
		Owner: parts[0],
		Repo:  parts[1],
		Path:  parts[2],
		Ref:   ref,
	}, nil
}
