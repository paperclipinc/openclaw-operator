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
	"testing"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

func TestBuildHTTPRoute_Nil(t *testing.T) {
	instance := newTestInstance("my-instance")
	if route := BuildHTTPRoute(instance); route != nil {
		t.Fatalf("expected nil HTTPRoute when spec.networking.httpRoute is unset, got %v", route)
	}
}

func TestBuildHTTPRoute_Defaults(t *testing.T) {
	instance := newTestInstance("my-instance")
	instance.Spec.Networking.HTTPRoute = &openclawv1alpha1.HTTPRouteSpec{
		Enabled: true,
		ParentRefs: []openclawv1alpha1.HTTPRouteParentRef{
			{Name: "external"},
		},
		Hostnames: []string{"openclaw.example.com"},
	}

	route := BuildHTTPRoute(instance)
	if route == nil {
		t.Fatal("expected non-nil HTTPRoute")
	}

	if got := route.GetAPIVersion(); got != "gateway.networking.k8s.io/v1" {
		t.Errorf("apiVersion: expected gateway.networking.k8s.io/v1, got %q", got)
	}
	if got := route.GetKind(); got != "HTTPRoute" {
		t.Errorf("kind: expected HTTPRoute, got %q", got)
	}
	if got := route.GetName(); got != "my-instance" {
		t.Errorf("name: expected my-instance, got %q", got)
	}
	if got := route.GetNamespace(); got != "test-ns" {
		t.Errorf("namespace: expected test-ns, got %q", got)
	}
	if route.GetLabels()["app.kubernetes.io/instance"] != "my-instance" {
		t.Errorf("missing standard instance label: %v", route.GetLabels())
	}

	spec := route.Object["spec"].(map[string]interface{})

	parentRefs := spec["parentRefs"].([]interface{})
	if len(parentRefs) != 1 {
		t.Fatalf("expected 1 parentRef, got %d", len(parentRefs))
	}
	pr := parentRefs[0].(map[string]interface{})
	if pr["name"] != "external" {
		t.Errorf("parentRef name: expected external, got %v", pr["name"])
	}
	if _, ok := pr["namespace"]; ok {
		t.Errorf("parentRef namespace should be omitted when unset, got %v", pr["namespace"])
	}

	hostnames := spec["hostnames"].([]interface{})
	if len(hostnames) != 1 || hostnames[0] != "openclaw.example.com" {
		t.Errorf("hostnames: expected [openclaw.example.com], got %v", hostnames)
	}

	rules := spec["rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	rule := rules[0].(map[string]interface{})

	matches := rule["matches"].([]interface{})
	match := matches[0].(map[string]interface{})
	path := match["path"].(map[string]interface{})
	if path["type"] != "PathPrefix" {
		t.Errorf("path type: expected PathPrefix, got %v", path["type"])
	}
	if path["value"] != "/" {
		t.Errorf("path value: expected /, got %v", path["value"])
	}

	backendRefs := rule["backendRefs"].([]interface{})
	backend := backendRefs[0].(map[string]interface{})
	if backend["name"] != "my-instance" {
		t.Errorf("backend name: expected my-instance (Service name), got %v", backend["name"])
	}
	if backend["port"] != int64(GatewayPort) {
		t.Errorf("backend port: expected default gateway port %d, got %v", GatewayPort, backend["port"])
	}
}

func TestBuildHTTPRoute_CustomPortAndParentRefFields(t *testing.T) {
	instance := newTestInstance("my-instance")
	ns := "gateway-ns"
	section := "https"
	instance.Spec.Networking.HTTPRoute = &openclawv1alpha1.HTTPRouteSpec{
		Enabled: true,
		Port:    Ptr(int32(CanvasPort)),
		ParentRefs: []openclawv1alpha1.HTTPRouteParentRef{
			{Name: "gw", Namespace: &ns, SectionName: &section},
		},
		Annotations: map[string]string{"example.com/foo": "bar"},
	}

	route := BuildHTTPRoute(instance)
	if route == nil {
		t.Fatal("expected non-nil HTTPRoute")
	}

	if route.GetAnnotations()["example.com/foo"] != "bar" {
		t.Errorf("expected annotation to be propagated, got %v", route.GetAnnotations())
	}

	spec := route.Object["spec"].(map[string]interface{})

	pr := spec["parentRefs"].([]interface{})[0].(map[string]interface{})
	if pr["namespace"] != "gateway-ns" {
		t.Errorf("parentRef namespace: expected gateway-ns, got %v", pr["namespace"])
	}
	if pr["sectionName"] != "https" {
		t.Errorf("parentRef sectionName: expected https, got %v", pr["sectionName"])
	}

	rule := spec["rules"].([]interface{})[0].(map[string]interface{})
	backend := rule["backendRefs"].([]interface{})[0].(map[string]interface{})
	if backend["port"] != int64(CanvasPort) {
		t.Errorf("backend port: expected custom port %d, got %v", CanvasPort, backend["port"])
	}
}

func TestBuildHTTPRoute_NoHostnamesOmitsField(t *testing.T) {
	instance := newTestInstance("my-instance")
	instance.Spec.Networking.HTTPRoute = &openclawv1alpha1.HTTPRouteSpec{
		Enabled:    true,
		ParentRefs: []openclawv1alpha1.HTTPRouteParentRef{{Name: "gw"}},
	}

	route := BuildHTTPRoute(instance)
	spec := route.Object["spec"].(map[string]interface{})
	if _, ok := spec["hostnames"]; ok {
		t.Errorf("hostnames should be omitted when none configured, got %v", spec["hostnames"])
	}
}
