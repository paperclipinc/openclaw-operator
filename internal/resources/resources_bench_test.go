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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// newBenchInstance creates a minimal instance for benchmarking.
func newBenchInstance() *openclawv1alpha1.OpenClawInstance {
	return &openclawv1alpha1.OpenClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bench",
			Namespace: "bench-ns",
		},
		Spec: openclawv1alpha1.OpenClawInstanceSpec{},
	}
}

// newFullBenchInstance creates a fully-loaded instance for benchmarking.
func newFullBenchInstance() *openclawv1alpha1.OpenClawInstance {
	return &openclawv1alpha1.OpenClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bench-full",
			Namespace: "bench-ns",
		},
		Spec: openclawv1alpha1.OpenClawInstanceSpec{
			Image: openclawv1alpha1.ImageSpec{
				Repository: "ghcr.io/openclaw/openclaw",
				Tag:        "v1.0.0",
			},
			Config: openclawv1alpha1.ConfigSpec{
				Raw: &openclawv1alpha1.RawConfig{
					RawExtension: runtime.RawExtension{Raw: []byte(`{"key":"value","nested":{"a":1,"b":"c"}}`)},
				},
			},
			Chromium: openclawv1alpha1.ChromiumSpec{
				Enabled: true,
			},
			Env: []corev1.EnvVar{
				{Name: "ENV1", Value: "val1"},
				{Name: "ENV2", Value: "val2"},
			},
			EnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: "api-keys"},
					},
				},
			},
			Resources: openclawv1alpha1.ResourcesSpec{
				Requests: openclawv1alpha1.ResourceList{
					CPU:    "500m",
					Memory: "512Mi",
				},
				Limits: openclawv1alpha1.ResourceList{
					CPU:    "2",
					Memory: "2Gi",
				},
			},
			Availability: openclawv1alpha1.AvailabilitySpec{
				NodeSelector: map[string]string{"node-type": "gpu"},
				Tolerations: []corev1.Toleration{
					{Key: "gpu", Operator: corev1.TolerationOpEqual, Value: "true", Effect: corev1.TaintEffectNoSchedule},
				},
				Affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
							{
								Weight: 1,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1a"}},
									},
								},
							},
						},
					},
				},
				PodDisruptionBudget: &openclawv1alpha1.PodDisruptionBudgetSpec{
					Enabled:        Ptr(true),
					MaxUnavailable: Ptr(int32(1)),
				},
				AutoScaling: &openclawv1alpha1.AutoScalingSpec{
					Enabled:              Ptr(true),
					MinReplicas:          Ptr(int32(2)),
					MaxReplicas:          Ptr(int32(10)),
					TargetCPUUtilization: Ptr(int32(80)),
				},
			},
			Security: openclawv1alpha1.SecuritySpec{
				NetworkPolicy: openclawv1alpha1.NetworkPolicySpec{
					Enabled:                  Ptr(true),
					AllowedIngressNamespaces: []string{"monitoring", "ingress-nginx"},
					AllowedIngressCIDRs:      []string{"10.0.0.0/8"},
					AllowedEgressCIDRs:       []string{"10.0.0.0/8"},
				},
			},
			Networking: openclawv1alpha1.NetworkingSpec{
				Ingress: openclawv1alpha1.IngressSpec{
					Enabled: true,
					Hosts: []openclawv1alpha1.IngressHost{
						{Host: "bench.example.com"},
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// StatefulSet benchmarks
// ---------------------------------------------------------------------------

func BenchmarkBuildStatefulSet_Minimal(b *testing.B) {
	instance := newBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildStatefulSet(instance, "", nil, nil, nil)
	}
}

func BenchmarkBuildStatefulSet_FullyLoaded(b *testing.B) {
	instance := newFullBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildStatefulSet(instance, "", nil, nil, nil)
	}
}

// ---------------------------------------------------------------------------
// ConfigMap benchmarks
// ---------------------------------------------------------------------------

func BenchmarkBuildConfigMap_Minimal(b *testing.B) {
	instance := newBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildConfigMap(instance, "", nil)
	}
}

func BenchmarkBuildConfigMap_WithRawConfig(b *testing.B) {
	instance := newFullBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildConfigMap(instance, "test-token-hex", nil)
	}
}

// ---------------------------------------------------------------------------
// NetworkPolicy benchmarks
// ---------------------------------------------------------------------------

func BenchmarkBuildNetworkPolicy_Minimal(b *testing.B) {
	instance := newBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildNetworkPolicy(instance)
	}
}

func BenchmarkBuildNetworkPolicy_FullyLoaded(b *testing.B) {
	instance := newFullBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildNetworkPolicy(instance)
	}
}

// ---------------------------------------------------------------------------
// Ingress benchmarks
// ---------------------------------------------------------------------------

func BenchmarkBuildIngress(b *testing.B) {
	instance := newFullBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildIngress(instance)
	}
}

// ---------------------------------------------------------------------------
// Service benchmarks
// ---------------------------------------------------------------------------

func BenchmarkBuildService_Minimal(b *testing.B) {
	instance := newBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildService(instance)
	}
}

func BenchmarkBuildService_FullyLoaded(b *testing.B) {
	instance := newFullBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildService(instance)
	}
}

// ---------------------------------------------------------------------------
// PDB benchmarks
// ---------------------------------------------------------------------------

func BenchmarkBuildPDB(b *testing.B) {
	instance := newFullBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildPDB(instance)
	}
}

// ---------------------------------------------------------------------------
// HPA benchmarks
// ---------------------------------------------------------------------------

func BenchmarkBuildHPA(b *testing.B) {
	instance := newFullBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildHPA(instance)
	}
}

// ---------------------------------------------------------------------------
// PVC benchmarks
// ---------------------------------------------------------------------------

func BenchmarkBuildPVC(b *testing.B) {
	instance := newFullBenchInstance()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildPVC(instance)
	}
}
