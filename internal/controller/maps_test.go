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
	"testing"
)

func TestMergeStringMap_NilCurrent(t *testing.T) {
	result := mergeStringMap(nil, map[string]string{"foo": "bar"})
	if result["foo"] != "bar" {
		t.Errorf("expected foo=bar, got %q", result["foo"])
	}
}

func TestMergeStringMap_PreservesExternalAnnotations(t *testing.T) {
	current := map[string]string{
		"field.cattle.io/publicEndpoints": "external-value",
	}
	result := mergeStringMap(current, map[string]string{"operator.io/key": "v1"})
	if result["field.cattle.io/publicEndpoints"] != "external-value" {
		t.Error("external annotation was stripped")
	}
	if result["operator.io/key"] != "v1" {
		t.Error("desired annotation was not set")
	}
}

func TestMergeStringMap_UpdatesExistingOperatorAnnotation(t *testing.T) {
	current := map[string]string{"operator.io/key": "v1"}
	result := mergeStringMap(current, map[string]string{"operator.io/key": "v2"})
	if result["operator.io/key"] != "v2" {
		t.Errorf("expected v2, got %q", result["operator.io/key"])
	}
}

func TestMergeStringMap_StaleOperatorAnnotationsRetained(t *testing.T) {
	// Stale operator annotations are intentionally not pruned; see mergeStringMap doc.
	current := map[string]string{"operator.io/old-key": "v1"}
	result := mergeStringMap(current, map[string]string{"operator.io/new-key": "v2"})
	if result["operator.io/old-key"] != "v1" {
		t.Error("stale annotation should be retained, not pruned")
	}
	if result["operator.io/new-key"] != "v2" {
		t.Error("new annotation was not set")
	}
}

func TestMergeStringMap_Labels(t *testing.T) {
	current := map[string]string{
		"app.kubernetes.io/managed-by": "external-tool",
	}
	result := mergeStringMap(current, map[string]string{"operator.io/label": "v1"})
	if result["app.kubernetes.io/managed-by"] != "external-tool" {
		t.Error("external label was stripped")
	}
	if result["operator.io/label"] != "v1" {
		t.Error("desired label was not set")
	}
}
