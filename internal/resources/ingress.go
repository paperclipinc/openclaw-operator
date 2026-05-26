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
	"strconv"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// IngressProvider represents the detected ingress controller type
type IngressProvider string

const (
	IngressProviderNginx   IngressProvider = "nginx"
	IngressProviderTraefik IngressProvider = "traefik"
	IngressProviderUnknown IngressProvider = "unknown"
)

// BuildIngress creates an Ingress for the OpenClawInstance
func BuildIngress(instance *openclawv1alpha1.OpenClawInstance) *networkingv1.Ingress {
	labels := Labels(instance)
	annotations := buildIngressAnnotations(instance)

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        IngressName(instance),
			Namespace:   instance.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: instance.Spec.Networking.Ingress.ClassName,
			Rules:            buildIngressRulesFromSpec(instance),
			TLS:              buildIngressTLS(instance),
		},
	}

	return ingress
}

// DetectIngressProvider determines the ingress controller type from the className.
// Returns IngressProviderNginx if className contains "nginx" (case-insensitive),
// IngressProviderTraefik if it contains "traefik", or IngressProviderUnknown otherwise.
func DetectIngressProvider(className *string) IngressProvider {
	if className == nil {
		return IngressProviderUnknown
	}
	lower := strings.ToLower(*className)
	if strings.Contains(lower, "nginx") {
		return IngressProviderNginx
	}
	if strings.Contains(lower, "traefik") {
		return IngressProviderTraefik
	}
	return IngressProviderUnknown
}

// buildIngressAnnotations creates annotations for the Ingress with security settings.
// Annotations are provider-aware: only nginx-specific annotations are emitted for nginx,
// only traefik-specific annotations for traefik. Unknown/nil providers get no provider-specific
// annotations — users can still add their own via spec.networking.ingress.annotations.
func buildIngressAnnotations(instance *openclawv1alpha1.OpenClawInstance) map[string]string {
	annotations := map[string]string{}

	// Copy user-provided annotations
	for k, v := range instance.Spec.Networking.Ingress.Annotations {
		annotations[k] = v
	}

	provider := DetectIngressProvider(instance.Spec.Networking.Ingress.ClassName)
	emitNginx := provider == IngressProviderNginx
	emitTraefik := provider == IngressProviderTraefik

	// Apply security settings
	security := instance.Spec.Networking.Ingress.Security

	// Force HTTPS redirect
	forceHTTPS := security.ForceHTTPS == nil || *security.ForceHTTPS
	if forceHTTPS {
		if emitNginx {
			annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
			annotations["nginx.ingress.kubernetes.io/force-ssl-redirect"] = "true"
		}
		if emitTraefik {
			annotations["traefik.ingress.kubernetes.io/router.entrypoints"] = "websecure"
		}
	}

	// Enable HSTS (nginx only — traefik HSTS requires a Middleware CRD)
	enableHSTS := security.EnableHSTS == nil || *security.EnableHSTS
	if enableHSTS && emitNginx {
		annotations["nginx.ingress.kubernetes.io/configuration-snippet"] = `more_set_headers "Strict-Transport-Security: max-age=31536000; includeSubDomains";`
	}

	// Rate limiting (nginx only — traefik rate limiting requires a Middleware CRD)
	if security.RateLimiting != nil {
		enabled := security.RateLimiting.Enabled == nil || *security.RateLimiting.Enabled
		if enabled && emitNginx {
			rps := int32(10)
			if security.RateLimiting.RequestsPerSecond != nil {
				rps = *security.RateLimiting.RequestsPerSecond
			}
			annotations["nginx.ingress.kubernetes.io/limit-rps"] = strconv.Itoa(int(rps))
		}
	}

	// WebSocket support (nginx only — traefik auto-detects WebSocket upgrades)
	if emitNginx {
		annotations["nginx.ingress.kubernetes.io/proxy-read-timeout"] = "3600"
		annotations["nginx.ingress.kubernetes.io/proxy-send-timeout"] = "3600"
		annotations["nginx.ingress.kubernetes.io/proxy-http-version"] = "1.1"
		annotations["nginx.ingress.kubernetes.io/upstream-hash-by"] = "$binary_remote_addr"
	}

	// Basic Auth
	if security.BasicAuth != nil {
		basicAuthEnabled := security.BasicAuth.Enabled == nil || *security.BasicAuth.Enabled
		if basicAuthEnabled {
			secretName := BasicAuthSecretName(instance)
			if security.BasicAuth.ExistingSecret != "" {
				secretName = security.BasicAuth.ExistingSecret
			}
			realm := "OpenClaw"
			if security.BasicAuth.Realm != "" {
				realm = security.BasicAuth.Realm
			}
			if emitNginx {
				annotations["nginx.ingress.kubernetes.io/auth-type"] = "basic"
				annotations["nginx.ingress.kubernetes.io/auth-secret"] = secretName
				annotations["nginx.ingress.kubernetes.io/auth-realm"] = realm
			}
			if emitTraefik {
				// For Traefik, a BasicAuth Middleware must be created alongside the Ingress.
				// The annotation references it as <namespace>-<name>@kubernetescrd.
				middlewareName := instance.Name + "-basic-auth"
				annotations["traefik.ingress.kubernetes.io/router.middlewares"] =
					instance.Namespace + "-" + middlewareName + "@kubernetescrd"
			}
		}
	}

	return annotations
}

// buildIngressRulesFromSpec creates Ingress rules from the spec
func buildIngressRulesFromSpec(instance *openclawv1alpha1.OpenClawInstance) []networkingv1.IngressRule {
	rules := []networkingv1.IngressRule{}

	pathType := networkingv1.PathTypePrefix

	for _, host := range instance.Spec.Networking.Ingress.Hosts {
		rule := networkingv1.IngressRule{
			Host: host.Host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{},
				},
			},
		}

		// Add paths or default to /
		paths := host.Paths
		if len(paths) == 0 {
			paths = []openclawv1alpha1.IngressPath{{Path: "/", PathType: "Prefix"}}
		}

		for _, p := range paths {
			path := p.Path
			if path == "" {
				path = "/"
			}

			pt := pathType
			if p.PathType == "Exact" {
				pt = networkingv1.PathTypeExact
			} else if p.PathType == "ImplementationSpecific" {
				pt = networkingv1.PathTypeImplementationSpecific
			}

			backendPort := int32(GatewayPort)
			if p.Port != nil {
				backendPort = *p.Port
			}

			rule.HTTP.Paths = append(rule.HTTP.Paths, networkingv1.HTTPIngressPath{
				Path:     path,
				PathType: &pt,
				Backend: networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: ServiceName(instance),
						Port: networkingv1.ServiceBackendPort{
							Number: backendPort,
						},
					},
				},
			})
		}

		rules = append(rules, rule)
	}

	return rules
}

// buildIngressTLS creates TLS configuration from the spec
func buildIngressTLS(instance *openclawv1alpha1.OpenClawInstance) []networkingv1.IngressTLS {
	tls := []networkingv1.IngressTLS{}

	for _, t := range instance.Spec.Networking.Ingress.TLS {
		tls = append(tls, networkingv1.IngressTLS{
			Hosts:      t.Hosts,
			SecretName: t.SecretName,
		})
	}

	return tls
}
