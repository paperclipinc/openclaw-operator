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

package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

var _ = Describe("OpenClawSelfConfig Controller", func() {
	const (
		timeout  = 60 * time.Second
		interval = 1 * time.Second
	)

	Context("When processing a valid self-config request", func() {
		var (
			instanceName string
			ns           *corev1.Namespace
		)

		BeforeEach(func() {
			instanceName = "sc-test-" + time.Now().Format("20060102150405")
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc-" + time.Now().Format("150405"),
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			// Create parent instance with self-configure enabled
			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: ns.Name,
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					SelfConfigure: openclawv1alpha1.SelfConfigureSpec{
						Enabled: true,
						AllowedActions: []openclawv1alpha1.SelfConfigAction{
							openclawv1alpha1.SelfConfigActionSkills,
							openclawv1alpha1.SelfConfigActionEnvVars,
						},
					},
					Skills: []string{"existing-skill"},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())
		})

		It("should apply skill changes and transition to Applied", func() {
			sc := &openclawv1alpha1.OpenClawSelfConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "add-skill-req",
					Namespace: ns.Name,
				},
				Spec: openclawv1alpha1.OpenClawSelfConfigSpec{
					InstanceRef: instanceName,
					AddSkills:   []string{"@anthropic/mcp-server-fetch"},
				},
			}
			Expect(k8sClient.Create(ctx, sc)).To(Succeed())

			// Wait for Applied phase
			Eventually(func() openclawv1alpha1.SelfConfigPhase {
				fetched := &openclawv1alpha1.OpenClawSelfConfig{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "add-skill-req", Namespace: ns.Name}, fetched)
				if err != nil {
					return ""
				}
				return fetched.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.SelfConfigPhaseApplied))

			// Verify skill was added to parent instance
			instance := &openclawv1alpha1.OpenClawInstance{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: ns.Name}, instance)).To(Succeed())
			Expect(instance.Spec.Skills).To(ContainElement("@anthropic/mcp-server-fetch"))
			Expect(instance.Spec.Skills).To(ContainElement("existing-skill"))
		})

		It("should deny disallowed action categories", func() {
			sc := &openclawv1alpha1.OpenClawSelfConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "denied-config-req",
					Namespace: ns.Name,
				},
				Spec: openclawv1alpha1.OpenClawSelfConfigSpec{
					InstanceRef: instanceName,
					ConfigPatch: &openclawv1alpha1.RawConfig{
						RawExtension: runtime.RawExtension{Raw: []byte(`{"key":"value"}`)},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sc)).To(Succeed())

			// Wait for Denied phase (config not in allowedActions)
			Eventually(func() openclawv1alpha1.SelfConfigPhase {
				fetched := &openclawv1alpha1.OpenClawSelfConfig{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "denied-config-req", Namespace: ns.Name}, fetched)
				if err != nil {
					return ""
				}
				return fetched.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.SelfConfigPhaseDenied))
		})
	})

	Context("When self-configure is disabled", func() {
		It("should deny all requests", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc-disabled-" + time.Now().Format("150405"),
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			// Create instance without self-configure
			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-sc-instance",
					Namespace: ns.Name,
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{},
			}
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			sc := &openclawv1alpha1.OpenClawSelfConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "denied-sc-req",
					Namespace: ns.Name,
				},
				Spec: openclawv1alpha1.OpenClawSelfConfigSpec{
					InstanceRef: "no-sc-instance",
					AddSkills:   []string{"some-skill"},
				},
			}
			Expect(k8sClient.Create(ctx, sc)).To(Succeed())

			Eventually(func() openclawv1alpha1.SelfConfigPhase {
				fetched := &openclawv1alpha1.OpenClawSelfConfig{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "denied-sc-req", Namespace: ns.Name}, fetched)
				if err != nil {
					return ""
				}
				return fetched.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.SelfConfigPhaseDenied))
		})
	})

	Context("When instance does not exist", func() {
		It("should fail with instance not found", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc-noinstance-" + time.Now().Format("150405"),
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			sc := &openclawv1alpha1.OpenClawSelfConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "orphan-req",
					Namespace: ns.Name,
				},
				Spec: openclawv1alpha1.OpenClawSelfConfigSpec{
					InstanceRef: "nonexistent",
					AddSkills:   []string{"some-skill"},
				},
			}
			Expect(k8sClient.Create(ctx, sc)).To(Succeed())

			Eventually(func() openclawv1alpha1.SelfConfigPhase {
				fetched := &openclawv1alpha1.OpenClawSelfConfig{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "orphan-req", Namespace: ns.Name}, fetched)
				if err != nil {
					return ""
				}
				return fetched.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.SelfConfigPhaseFailed))
		})
	})
})
