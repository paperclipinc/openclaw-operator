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
	"strings"
)

const (
	// SkillPackPrefix is the prefix for skill pack entries in the skills list.
	SkillPackPrefix = "pack:"
)

// ResolvedSkillPacks contains resolved workspace files, directories, and config for skill packs.
type ResolvedSkillPacks struct {
	// Files maps ConfigMap-safe keys to file content.
	Files map[string]string

	// PathMapping maps ConfigMap-safe keys to workspace-relative paths.
	PathMapping map[string]string

	// Directories to create in the workspace.
	Directories []string

	// SkillEntries to inject into config.raw.skills.entries.
	SkillEntries map[string]interface{}
}

// ExtractPackSkills returns pack names from the skills list (entries with "pack:" prefix).
func ExtractPackSkills(skills []string) []string {
	var packs []string
	for _, s := range skills {
		if name, ok := strings.CutPrefix(s, SkillPackPrefix); ok {
			packs = append(packs, name)
		}
	}
	return packs
}

// FilterNonPackSkills returns skills that are NOT pack: prefixed.
func FilterNonPackSkills(skills []string) []string {
	var result []string
	for _, s := range skills {
		if !strings.HasPrefix(s, SkillPackPrefix) {
			result = append(result, s)
		}
	}
	return result
}

// SkillPackCMKey converts a workspace-relative path to a ConfigMap-safe key.
// Replaces "/" with "--" to comply with ConfigMap key naming rules. Also used
// for inline spec.workspace.initialFiles paths that contain '/' (#482); the
// init script decodes the key back to the original path when seeding.
func SkillPackCMKey(wsPath string) string {
	return strings.ReplaceAll(wsPath, "/", "--")
}

// HasSkillPackFiles returns true if the resolved skill packs contain any workspace files.
func HasSkillPackFiles(sp *ResolvedSkillPacks) bool {
	return sp != nil && len(sp.Files) > 0
}
