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

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
)

// maxPages is the maximum number of paginated requests to prevent infinite loops.
const maxPages = 100

// Resolver queries OCI registries for tags and resolves the latest semver version.
type Resolver struct {
	cacheTTL   time.Duration
	httpClient *http.Client

	mu    sync.RWMutex
	cache map[string]*cacheEntry
}

type cacheEntry struct {
	tags      []string
	fetchedAt time.Time
}

// tagsListResponse is the OCI Distribution Spec response for /v2/{name}/tags/list
type tagsListResponse struct {
	Tags []string `json:"tags"`
}

// tokenResponse is the token exchange response
type tokenResponse struct {
	Token string `json:"token"`
}

// NewResolver creates a new Resolver with the given cache TTL.
func NewResolver(cacheTTL time.Duration) *Resolver {
	return &Resolver{
		cacheTTL: cacheTTL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: make(map[string]*cacheEntry),
	}
}

// LatestSemver returns the highest stable semver tag from the given repository.
// If constraint is non-nil, only tags matching the constraint are considered.
func (r *Resolver) LatestSemver(ctx context.Context, repository string, constraint *semver.Constraints) (string, error) {
	tags, err := r.getTags(ctx, repository)
	if err != nil {
		return "", fmt.Errorf("fetching tags for %s: %w", repository, err)
	}

	var best *semver.Version
	var bestRaw string

	for _, tag := range tags {
		v, err := semver.NewVersion(tag)
		if err != nil {
			continue
		}
		// Skip pre-release versions
		if v.Prerelease() != "" {
			continue
		}
		// Apply constraint if provided
		if constraint != nil && !constraint.Check(v) {
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best = v
			bestRaw = tag
		}
	}

	if best == nil {
		return "", fmt.Errorf("no stable semver tags found in %s", repository)
	}

	return bestRaw, nil
}

// getTags returns the tag list for the repository, using cache if valid.
func (r *Resolver) getTags(ctx context.Context, repository string) ([]string, error) {
	r.mu.RLock()
	if entry, ok := r.cache[repository]; ok && time.Since(entry.fetchedAt) < r.cacheTTL {
		tags := entry.tags
		r.mu.RUnlock()
		return tags, nil
	}
	r.mu.RUnlock()

	tags, err := r.fetchTags(ctx, repository)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[repository] = &cacheEntry{tags: tags, fetchedAt: time.Now()}
	r.mu.Unlock()

	return tags, nil
}

// fetchTags queries the OCI Distribution API for the tag list, following
// pagination via the Link header as defined by the OCI Distribution Spec.
func (r *Resolver) fetchTags(ctx context.Context, repository string) ([]string, error) {
	host, name, err := parseRepository(repository)
	if err != nil {
		return nil, err
	}

	token, err := r.getToken(ctx, host, name)
	if err != nil {
		return nil, fmt.Errorf("authenticating with %s: %w", host, err)
	}

	baseURL := fmt.Sprintf("https://%s/v2/%s/tags/list", host, name)
	nextURL := baseURL + "?n=10000"
	var allTags []string

	for page := 0; page < maxPages; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, http.NoBody)
		if err != nil {
			return nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := r.httpClient.Do(req) // #nosec G704 -- URL is built from operator-controlled spec.image.repository
		if err != nil {
			return nil, fmt.Errorf("fetching tags from %s: %w", nextURL, err)
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, nextURL)
		}

		var tagsList tagsListResponse
		decErr := json.NewDecoder(resp.Body).Decode(&tagsList)
		linkHeader := resp.Header.Get("Link")
		_ = resp.Body.Close()
		if decErr != nil {
			return nil, fmt.Errorf("decoding tags response: %w", decErr)
		}

		allTags = append(allTags, tagsList.Tags...)

		// Check for a next page via the Link header (OCI pagination)
		nextURL, err = parseNextLink(linkHeader, baseURL)
		if err != nil {
			return nil, fmt.Errorf("parsing Link header: %w", err)
		}
		if nextURL == "" {
			break
		}
	}

	return allTags, nil
}

// parseNextLink extracts the next page URL from an OCI Distribution Link header.
// The header format is: `</v2/.../tags/list?last=...&n=...>; rel="next"`.
// Returns empty string if there is no next page.
func parseNextLink(header, baseURL string) (string, error) {
	if header == "" {
		return "", nil
	}

	// OCI spec: Link: </v2/name/tags/list?last=tag&n=100>; rel="next"
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end < 0 || end <= start {
			continue
		}
		linkPath := part[start+1 : end]

		// The link may be absolute or relative. Resolve against the base URL.
		base, err := url.Parse(baseURL)
		if err != nil {
			return "", fmt.Errorf("parsing base URL %q: %w", baseURL, err)
		}
		ref, err := url.Parse(linkPath)
		if err != nil {
			return "", fmt.Errorf("parsing link %q: %w", linkPath, err)
		}
		return base.ResolveReference(ref).String(), nil
	}

	return "", nil
}

// getToken performs the anonymous token flow for OCI registries.
// It first makes an unauthenticated request to discover the auth challenge,
// then exchanges for a token.
func (r *Resolver) getToken(ctx context.Context, host, name string) (string, error) {
	// Probe /v2/ to get the WWW-Authenticate challenge
	probeURL := fmt.Sprintf("https://%s/v2/", host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, http.NoBody)
	if err != nil {
		return "", err
	}

	resp, err := r.httpClient.Do(req) // #nosec G704 -- URL is built from operator-controlled spec.image.repository
	if err != nil {
		return "", fmt.Errorf("probing %s: %w", probeURL, err)
	}
	defer resp.Body.Close()

	// If the registry doesn't require auth, return empty token
	if resp.StatusCode == http.StatusOK {
		return "", nil
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, probeURL)
	}

	// Parse WWW-Authenticate header to find the token endpoint
	authHeader := resp.Header.Get("WWW-Authenticate")
	realm, service := parseAuthChallenge(authHeader)
	if realm == "" {
		return "", fmt.Errorf("no realm in WWW-Authenticate header from %s", probeURL)
	}

	// Request anonymous token
	tokenURL := fmt.Sprintf("%s?scope=repository:%s:pull&service=%s", realm, name, service)
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, http.NoBody)
	if err != nil {
		return "", err
	}

	tokenResp, err := r.httpClient.Do(tokenReq) // #nosec G704 -- token URL derived from registry WWW-Authenticate challenge
	if err != nil {
		return "", fmt.Errorf("fetching token from %s: %w", tokenURL, err)
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from token endpoint %s", tokenResp.StatusCode, tokenURL)
	}

	var tok tokenResponse
	if err := json.NewDecoder(tokenResp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	return tok.Token, nil
}

// parseRepository splits "ghcr.io/openclaw/openclaw" into host="ghcr.io", name="openclaw/openclaw".
func parseRepository(repository string) (host, name string, err error) {
	parts := strings.SplitN(repository, "/", 2)
	if len(parts) != 2 || !strings.Contains(parts[0], ".") {
		return "", "", fmt.Errorf("invalid repository format %q: expected host/name", repository)
	}
	return parts[0], parts[1], nil
}

// parseAuthChallenge extracts realm and service from a WWW-Authenticate header.
// Example: `Bearer realm="https://ghcr.io/token",service="ghcr.io"`
func parseAuthChallenge(header string) (realm, service string) {
	header = strings.TrimPrefix(header, "Bearer ")
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "realm=") {
			realm = strings.Trim(strings.TrimPrefix(part, "realm="), "\"")
		} else if strings.HasPrefix(part, "service=") {
			service = strings.Trim(strings.TrimPrefix(part, "service="), "\"")
		}
	}
	return realm, service
}
