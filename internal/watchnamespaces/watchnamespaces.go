// Package watchnamespaces parses OPENCLAW_WATCH_NAMESPACES for namespace-scoped operator caches.
package watchnamespaces

import (
	"os"
	"strings"
)

const envVar = "OPENCLAW_WATCH_NAMESPACES"

// FromEnv returns a deduplicated list of namespaces from OPENCLAW_WATCH_NAMESPACES
// (comma-separated). Empty or unset env means nil (watch all namespaces).
func FromEnv() []string {
	return Parse(os.Getenv(envVar))
}

// Parse splits a comma-separated watch namespace list, trims entries, drops empties, dedupes.
func Parse(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		ns := strings.TrimSpace(p)
		if ns == "" {
			continue
		}
		if _, ok := seen[ns]; ok {
			continue
		}
		seen[ns] = struct{}{}
		out = append(out, ns)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
