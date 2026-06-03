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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// HTTPRouteGVK returns the GroupVersionKind for a Gateway API HTTPRoute.
// The operator builds the HTTPRoute as an unstructured object so the
// Gateway API Go module is not a build dependency (mirroring how the
// ServiceMonitor and PrometheusRule resources are built).
func HTTPRouteGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "HTTPRoute",
	}
}

// HTTPRouteName returns the name of the HTTPRoute
func HTTPRouteName(instance *openclawv1alpha1.OpenClawInstance) string {
	return instance.Name
}

// BuildHTTPRoute creates an unstructured Gateway API HTTPRoute
// (gateway.networking.k8s.io/v1) for the OpenClawInstance. It routes the
// configured hostnames to the OpenClaw gateway Service and port. Returns nil
// when HTTPRoute is not configured.
func BuildHTTPRoute(instance *openclawv1alpha1.OpenClawInstance) *unstructured.Unstructured {
	spec := instance.Spec.Networking.HTTPRoute
	if spec == nil {
		return nil
	}

	// Backend port defaults to the gateway port when not set explicitly.
	backendPort := int32(GatewayPort)
	if spec.Port != nil {
		backendPort = *spec.Port
	}

	parentRefs := make([]interface{}, 0, len(spec.ParentRefs))
	for _, ref := range spec.ParentRefs {
		pr := map[string]interface{}{
			"name": ref.Name,
		}
		if ref.Namespace != nil {
			pr["namespace"] = *ref.Namespace
		}
		if ref.SectionName != nil {
			pr["sectionName"] = *ref.SectionName
		}
		parentRefs = append(parentRefs, pr)
	}

	hostnames := make([]interface{}, 0, len(spec.Hostnames))
	for _, h := range spec.Hostnames {
		hostnames = append(hostnames, h)
	}

	// Set Kubernetes defaults explicitly: a single PathPrefix "/" match
	// routing to the gateway Service backend.
	rule := map[string]interface{}{
		"matches": []interface{}{
			map[string]interface{}{
				"path": map[string]interface{}{
					"type":  "PathPrefix",
					"value": "/",
				},
			},
		},
		"backendRefs": []interface{}{
			map[string]interface{}{
				"name": ServiceName(instance),
				"port": int64(backendPort),
			},
		},
	}

	httpSpec := map[string]interface{}{
		"parentRefs": parentRefs,
		"rules":      []interface{}{rule},
	}
	if len(hostnames) > 0 {
		httpSpec["hostnames"] = hostnames
	}

	route := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": HTTPRouteGVK().GroupVersion().String(),
			"kind":       HTTPRouteGVK().Kind,
			"metadata": map[string]interface{}{
				"name":      HTTPRouteName(instance),
				"namespace": instance.Namespace,
				"labels":    toStringInterfaceMap(Labels(instance)),
			},
			"spec": httpSpec,
		},
	}

	if len(spec.Annotations) > 0 {
		route.SetAnnotations(spec.Annotations)
	}

	return route
}
