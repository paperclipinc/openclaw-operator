/*
Copyright 2026 OpenClaw.rocks

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
	"fmt"
	"strings"
)

// ValidateWorkspaceFilename checks a single workspace filename.
// Used for keys from an external ConfigMap referenced by spec.workspace.configMapRef,
// where Kubernetes itself enforces that keys cannot contain '/'.
//
// For inline spec.workspace.initialFiles use ValidateWorkspaceFilePath, which
// permits nested paths (the operator encodes them into ConfigMap-safe keys).
func ValidateWorkspaceFilename(name string) error {
	if name == "" {
		return fmt.Errorf("filename must not be empty")
	}
	if len(name) > 253 {
		return fmt.Errorf("filename must be at most 253 characters")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("filename must not contain '/'")
	}
	if strings.Contains(name, "\\") {
		return fmt.Errorf("filename must not contain '\\'")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("filename must not contain '..'")
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("filename must not start with '.'")
	}
	if name == "openclaw.json" {
		return fmt.Errorf("filename 'openclaw.json' is reserved for config")
	}
	return nil
}

// ValidateWorkspaceFilePath checks a workspace-relative file path. Unlike
// ValidateWorkspaceFilename it allows '/' so callers can seed nested files
// (e.g. "agents/AGENT.md"). The operator encodes '/' as '--' when storing the
// content in a ConfigMap and decodes it back when seeding the workspace
// (issue #482).
//
// Path safety rules (apply per segment as well as overall):
//   - non-empty, max 253 chars
//   - no backslashes
//   - no ".." segment (or substring inside any segment)
//   - no leading or trailing "/", no empty segments
//   - no segment starting with "." (hidden file/dir)
//   - "openclaw.json" reserved at the root only
func ValidateWorkspaceFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("path must not be empty")
	}
	if len(path) > 253 {
		return fmt.Errorf("path must be at most 253 characters")
	}
	if strings.Contains(path, "\\") {
		return fmt.Errorf("path must not contain '\\'")
	}
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must not be absolute")
	}
	if strings.HasSuffix(path, "/") {
		return fmt.Errorf("path must not end with '/'")
	}
	if path == "openclaw.json" {
		return fmt.Errorf("path 'openclaw.json' is reserved for config")
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			return fmt.Errorf("path must not contain empty segments")
		}
		if strings.Contains(seg, "..") {
			return fmt.Errorf("path segment %q must not contain '..'", seg)
		}
		if strings.HasPrefix(seg, ".") {
			return fmt.Errorf("path segment %q must not start with '.'", seg)
		}
	}
	return nil
}

// ValidateWorkspaceDirectory checks a single workspace directory path.
// Exported so both the webhook and the controller can validate directory names.
func ValidateWorkspaceDirectory(dir string) error {
	if dir == "" {
		return fmt.Errorf("directory must not be empty")
	}
	if len(dir) > 253 {
		return fmt.Errorf("directory must be at most 253 characters")
	}
	if strings.Contains(dir, "\\") {
		return fmt.Errorf("directory must not contain '\\'")
	}
	if strings.Contains(dir, "..") {
		return fmt.Errorf("directory must not contain '..'")
	}
	if strings.HasPrefix(dir, "/") {
		return fmt.Errorf("directory must not be an absolute path")
	}
	return nil
}
