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
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
)

// writeJSON encodes v as JSON to the response writer.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// newTestServer creates an httptest.Server that simulates an OCI registry with
// token auth and a tags/list endpoint.
func newTestServer(tags []string) *httptest.Server {
	return newPaginatedTestServer([][]string{tags})
}

// newPaginatedTestServer creates an httptest.Server that simulates an OCI registry
// with paginated tag responses. Each element of pages is one page of tags.
func newPaginatedTestServer(pages [][]string) *httptest.Server {
	mux := http.NewServeMux()

	// Token endpoint (anonymous)
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, tokenResponse{Token: "test-token"})
	})

	// /v2/ probe and tags/list handler
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tags/list") {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Determine which page to serve based on the "last" query parameter
			last := r.URL.Query().Get("last")
			pageIdx := 0
			if last != "" {
				// Find the page after the one containing "last"
				for i, page := range pages {
					if len(page) > 0 && page[len(page)-1] == last {
						pageIdx = i + 1
						break
					}
				}
			}

			if pageIdx >= len(pages) {
				writeJSON(w, tagsListResponse{Tags: []string{}})
				return
			}

			// Set Link header if there are more pages
			if pageIdx < len(pages)-1 {
				lastTag := pages[pageIdx][len(pages[pageIdx])-1]
				w.Header().Set("Link", fmt.Sprintf(`<%s?last=%s&n=%d>; rel="next"`,
					r.URL.Path, lastTag, len(pages[pageIdx])))
			}

			writeJSON(w, tagsListResponse{Tags: pages[pageIdx]})
			return
		}

		// /v2/ probe
		w.Header().Set("WWW-Authenticate", `Bearer realm="REALM_PLACEHOLDER/token",service="test-registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	})

	server := httptest.NewTLSServer(mux)

	// Patch the realm URL to point to the test server
	origHandler := mux
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Override the WWW-Authenticate header for /v2/ probe
		if r.URL.Path == "/v2/" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="test-registry"`, server.URL))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		origHandler.ServeHTTP(w, r)
	})
	server.Config.Handler = mux2

	return server
}

func TestLatestSemver(t *testing.T) {
	tags := []string{"v1.0.0", "v1.1.0", "v2.0.0", "v1.2.0-rc1", "latest", "main", "v0.9.0"}
	server := newTestServer(tags)
	defer server.Close()

	resolver := NewResolver(5 * time.Minute)
	resolver.httpClient = server.Client()

	// Extract host from server URL (strip https://)
	host := strings.TrimPrefix(server.URL, "https://")
	repo := host + "/openclaw/openclaw"

	version, err := resolver.LatestSemver(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "v2.0.0" {
		t.Errorf("expected v2.0.0, got %s", version)
	}
}

func TestLatestSemverWithConstraint(t *testing.T) {
	tags := []string{"v1.0.0", "v1.1.0", "v2.0.0", "v1.2.0", "v0.9.0"}
	server := newTestServer(tags)
	defer server.Close()

	resolver := NewResolver(5 * time.Minute)
	resolver.httpClient = server.Client()

	host := strings.TrimPrefix(server.URL, "https://")
	repo := host + "/openclaw/openclaw"

	constraint, _ := semver.NewConstraint(">=1.0.0, <2.0.0")
	version, err := resolver.LatestSemver(context.Background(), repo, constraint)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "v1.2.0" {
		t.Errorf("expected v1.2.0, got %s", version)
	}
}

func TestLatestSemverSkipsPrerelease(t *testing.T) {
	tags := []string{"v1.0.0", "v2.0.0-beta.1", "v2.0.0-rc1"}
	server := newTestServer(tags)
	defer server.Close()

	resolver := NewResolver(5 * time.Minute)
	resolver.httpClient = server.Client()

	host := strings.TrimPrefix(server.URL, "https://")
	repo := host + "/openclaw/openclaw"

	version, err := resolver.LatestSemver(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "v1.0.0" {
		t.Errorf("expected v1.0.0, got %s", version)
	}
}

func TestLatestSemverWithoutVPrefix(t *testing.T) {
	tags := []string{"1.0.0", "1.1.0", "2.0.0"}
	server := newTestServer(tags)
	defer server.Close()

	resolver := NewResolver(5 * time.Minute)
	resolver.httpClient = server.Client()

	host := strings.TrimPrefix(server.URL, "https://")
	repo := host + "/openclaw/openclaw"

	version, err := resolver.LatestSemver(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should preserve the original tag format (no v prefix)
	if version != "2.0.0" {
		t.Errorf("expected 2.0.0, got %s", version)
	}
}

func TestLatestSemverNoTags(t *testing.T) {
	server := newTestServer([]string{"latest", "main"})
	defer server.Close()

	resolver := NewResolver(5 * time.Minute)
	resolver.httpClient = server.Client()

	host := strings.TrimPrefix(server.URL, "https://")
	repo := host + "/openclaw/openclaw"

	_, err := resolver.LatestSemver(context.Background(), repo, nil)
	if err == nil {
		t.Fatal("expected error for no semver tags")
	}
	if !strings.Contains(err.Error(), "no stable semver tags") {
		t.Errorf("expected 'no stable semver tags' error, got: %v", err)
	}
}

func TestCacheHit(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.URL.Path, "/tags/list") {
			callCount++
			writeJSON(w, tagsListResponse{Tags: []string{"v1.0.0"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	resolver := NewResolver(5 * time.Minute)
	resolver.httpClient = server.Client()

	host := strings.TrimPrefix(server.URL, "https://")
	repo := host + "/test/repo"

	// First call
	_, err := resolver.LatestSemver(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Second call — should hit cache
	_, err = resolver.LatestSemver(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 fetch call (cache hit), got %d", callCount)
	}
}

func TestCacheExpiry(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.URL.Path, "/tags/list") {
			callCount++
			writeJSON(w, tagsListResponse{Tags: []string{"v1.0.0"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	// Very short TTL for testing
	resolver := NewResolver(1 * time.Millisecond)
	resolver.httpClient = server.Client()

	host := strings.TrimPrefix(server.URL, "https://")
	repo := host + "/test/repo"

	_, _ = resolver.LatestSemver(context.Background(), repo, nil)
	time.Sleep(5 * time.Millisecond) // Wait for cache to expire
	_, _ = resolver.LatestSemver(context.Background(), repo, nil)

	if callCount != 2 {
		t.Errorf("expected 2 fetch calls (cache expired), got %d", callCount)
	}
}

func TestParseRepository(t *testing.T) {
	tests := []struct {
		input       string
		wantHost    string
		wantName    string
		shouldError bool
	}{
		{"ghcr.io/openclaw/openclaw", "ghcr.io", "openclaw/openclaw", false},
		{"docker.io/library/nginx", "docker.io", "library/nginx", false},
		{"registry.example.com/org/repo/sub", "registry.example.com", "org/repo/sub", false},
		{"invalid", "", "", true},
		{"noDot/repo", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			host, name, err := parseRepository(tt.input)
			if tt.shouldError {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tt.wantHost {
				t.Errorf("host: got %q, want %q", host, tt.wantHost)
			}
			if name != tt.wantName {
				t.Errorf("name: got %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestParseAuthChallenge(t *testing.T) {
	header := `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:openclaw/openclaw:pull"`
	realm, service := parseAuthChallenge(header)
	if realm != "https://ghcr.io/token" {
		t.Errorf("realm: got %q, want %q", realm, "https://ghcr.io/token")
	}
	if service != "ghcr.io" {
		t.Errorf("service: got %q, want %q", service, "ghcr.io")
	}
}

func TestRegistryUnreachable(t *testing.T) {
	resolver := NewResolver(5 * time.Minute)

	_, err := resolver.LatestSemver(context.Background(), "unreachable.invalid/org/repo", nil)
	if err == nil {
		t.Fatal("expected error for unreachable registry")
	}
}

func TestPaginatedTags(t *testing.T) {
	// Simulate GHCR returning tags across multiple pages - the latest version
	// is on page 2 and must not be missed.
	page1 := []string{"v1.0.0", "v1.1.0", "v1.2.0"}
	page2 := []string{"v2.0.0", "v2.1.0", "v3.0.0"}
	server := newPaginatedTestServer([][]string{page1, page2})
	defer server.Close()

	resolver := NewResolver(5 * time.Minute)
	resolver.httpClient = server.Client()

	host := strings.TrimPrefix(server.URL, "https://")
	repo := host + "/openclaw/openclaw"

	version, err := resolver.LatestSemver(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "v3.0.0" {
		t.Errorf("expected v3.0.0 (from page 2), got %s", version)
	}
}

func TestPaginatedTagsThreePages(t *testing.T) {
	page1 := []string{"v1.0.0", "v1.1.0"}
	page2 := []string{"v2.0.0", "v2.1.0"}
	page3 := []string{"v3.0.0", "v3.1.0"}
	server := newPaginatedTestServer([][]string{page1, page2, page3})
	defer server.Close()

	resolver := NewResolver(5 * time.Minute)
	resolver.httpClient = server.Client()

	host := strings.TrimPrefix(server.URL, "https://")
	repo := host + "/openclaw/openclaw"

	version, err := resolver.LatestSemver(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "v3.1.0" {
		t.Errorf("expected v3.1.0 (from page 3), got %s", version)
	}
}

func TestPaginatedSinglePage(t *testing.T) {
	// A single page should work identically to the non-paginated case.
	server := newPaginatedTestServer([][]string{{"v1.0.0", "v2.0.0"}})
	defer server.Close()

	resolver := NewResolver(5 * time.Minute)
	resolver.httpClient = server.Client()

	host := strings.TrimPrefix(server.URL, "https://")
	repo := host + "/openclaw/openclaw"

	version, err := resolver.LatestSemver(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "v2.0.0" {
		t.Errorf("expected v2.0.0, got %s", version)
	}
}

func TestParseNextLink(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		baseURL string
		want    string
	}{
		{
			name:    "empty header",
			header:  "",
			baseURL: "https://ghcr.io/v2/openclaw/openclaw/tags/list",
			want:    "",
		},
		{
			name:    "relative link",
			header:  `</v2/openclaw/openclaw/tags/list?last=v1.0.0&n=100>; rel="next"`,
			baseURL: "https://ghcr.io/v2/openclaw/openclaw/tags/list",
			want:    "https://ghcr.io/v2/openclaw/openclaw/tags/list?last=v1.0.0&n=100",
		},
		{
			name:    "absolute link",
			header:  `<https://ghcr.io/v2/openclaw/openclaw/tags/list?last=v1.0.0>; rel="next"`,
			baseURL: "https://ghcr.io/v2/openclaw/openclaw/tags/list",
			want:    "https://ghcr.io/v2/openclaw/openclaw/tags/list?last=v1.0.0",
		},
		{
			name:    "no rel=next",
			header:  `</v2/openclaw/openclaw/tags/list?last=v1.0.0>; rel="prev"`,
			baseURL: "https://ghcr.io/v2/openclaw/openclaw/tags/list",
			want:    "",
		},
		{
			name:    "GHCR style with n=0",
			header:  `</v2/openclaw/openclaw/tags/list?last=2026.3.1&n=0>; rel="next"`,
			baseURL: "https://ghcr.io/v2/openclaw/openclaw/tags/list",
			want:    "https://ghcr.io/v2/openclaw/openclaw/tags/list?last=2026.3.1&n=0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNextLink(tt.header, tt.baseURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
