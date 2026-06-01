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

package e2e

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

// This suite verifies #457 - cluster-wide defaults applied at reconcile time.
// The singleton OpenClawClusterDefaults fills in unset instance fields so
// platform operators do not need to duplicate registry/env boilerplate in
// every OpenClawInstance manifest (common for air-gapped / China deployments).
var _ = Describe("OpenClawClusterDefaults singleton (#457)", func() {
	Context("When the cluster-defaults singleton is set", func() {
		var namespace string
		var defaults *openclawv1alpha1.OpenClawClusterDefaults

		BeforeEach(func() {
			namespace = "test-cluster-defaults-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			defaults = &openclawv1alpha1.OpenClawClusterDefaults{
				ObjectMeta: metav1.ObjectMeta{Name: openclawv1alpha1.ClusterDefaultsSingletonName},
				Spec: openclawv1alpha1.OpenClawClusterDefaultsSpec{
					Registry: "mirror.example.com",
					Env: []corev1.EnvVar{
						{Name: "NPM_CONFIG_REGISTRY", Value: "https://registry.npmmirror.com"},
						{Name: "PIP_INDEX_URL", Value: "https://mirrors.aliyun.com/pypi/simple/"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, defaults)).Should(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, defaults)
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should apply defaults to instances with unset fields", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "inherits-defaults",
					Namespace: namespace,
					Annotations: map[string]string{
						"openclaw.rocks/skip-backup": "true",
					},
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					Image: openclawv1alpha1.ImageSpec{
						Repository: "ghcr.io/openclaw/openclaw",
						Tag:        "latest",
					},
					Storage: openclawv1alpha1.StorageSpec{
						Persistence: openclawv1alpha1.PersistenceSpec{Size: "1Gi"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, sts)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			main := sts.Spec.Template.Spec.Containers[0]

			// Cluster-default Registry should have replaced the main container's
			// image registry prefix.
			Expect(main.Image).To(HavePrefix("mirror.example.com/"),
				"main container image %q should inherit cluster-default registry prefix", main.Image)

			// Cluster-default env vars should appear on the main container.
			envByName := map[string]string{}
			for _, e := range main.Env {
				envByName[e.Name] = e.Value
			}
			Expect(envByName).To(HaveKeyWithValue("NPM_CONFIG_REGISTRY", "https://registry.npmmirror.com"),
				"cluster-default NPM_CONFIG_REGISTRY should be present on the main container")
			Expect(envByName).To(HaveKeyWithValue("PIP_INDEX_URL", "https://mirrors.aliyun.com/pypi/simple/"),
				"cluster-default PIP_INDEX_URL should be present on the main container")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should let instance fields override cluster defaults", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instance-wins",
					Namespace: namespace,
					Annotations: map[string]string{
						"openclaw.rocks/skip-backup": "true",
					},
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					Registry: "instance-registry.example.com",
					Image: openclawv1alpha1.ImageSpec{
						Repository: "ghcr.io/openclaw/openclaw",
						Tag:        "latest",
					},
					Env: []corev1.EnvVar{
						{Name: "PIP_INDEX_URL", Value: "https://instance-mirror.example.com/pypi"},
					},
					Storage: openclawv1alpha1.StorageSpec{
						Persistence: openclawv1alpha1.PersistenceSpec{Size: "1Gi"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, sts)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			main := sts.Spec.Template.Spec.Containers[0]

			Expect(main.Image).To(HavePrefix("instance-registry.example.com/"),
				"instance-level Registry should win over cluster default; got %q", main.Image)

			envByName := map[string]string{}
			for _, e := range main.Env {
				envByName[e.Name] = e.Value
			}
			Expect(envByName).To(HaveKeyWithValue("PIP_INDEX_URL", "https://instance-mirror.example.com/pypi"),
				"instance PIP_INDEX_URL should override cluster default")
			Expect(envByName).To(HaveKeyWithValue("NPM_CONFIG_REGISTRY", "https://registry.npmmirror.com"),
				"cluster-default NPM_CONFIG_REGISTRY should still be present for names not set on the instance")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
