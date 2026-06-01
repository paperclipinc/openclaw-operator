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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

var _ = Describe("Instance Suspension", func() {
	const (
		timeout  = time.Second * 120
		interval = time.Second * 2
	)

	Context("When creating a suspended instance", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-suspended-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should scale StatefulSet to 0 and set phase to Suspended, then resume on unsuspend", func() {
			instanceName := "suspended-test"

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
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
					Suspended: true,
				},
			}

			By("Creating a suspended instance")
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			By("Verifying StatefulSet has 0 replicas")
			Eventually(func() *int32 {
				sts := &appsv1.StatefulSet{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, sts)
				if err != nil {
					return nil
				}
				return sts.Spec.Replicas
			}, timeout, interval).Should(Equal(resources.Ptr(int32(0))))

			By("Verifying phase is Suspended")
			Eventually(func() string {
				inst := &openclawv1alpha1.OpenClawInstance{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: namespace}, inst)
				return inst.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.PhaseSuspended))

			By("Verifying Ready condition is False with reason Suspended")
			Eventually(func() string {
				inst := &openclawv1alpha1.OpenClawInstance{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: namespace}, inst)
				cond := meta.FindStatusCondition(inst.Status.Conditions, openclawv1alpha1.ConditionTypeReady)
				if cond == nil {
					return ""
				}
				return cond.Reason
			}, timeout, interval).Should(Equal("Suspended"))

			By("Verifying non-runtime resources still exist (Service)")
			Eventually(func() error {
				svc := &corev1.Service{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ServiceName(instance),
					Namespace: namespace,
				}, svc)
			}, timeout, interval).Should(Succeed())

			By("Unsuspending the instance")
			Eventually(func() error {
				inst := &openclawv1alpha1.OpenClawInstance{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: namespace}, inst); err != nil {
					return err
				}
				inst.Spec.Suspended = false
				return k8sClient.Update(ctx, inst)
			}, timeout, interval).Should(Succeed())

			By("Verifying StatefulSet scales back to 1 replica")
			Eventually(func() *int32 {
				sts := &appsv1.StatefulSet{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, sts)
				if err != nil {
					return nil
				}
				return sts.Spec.Replicas
			}, timeout, interval).Should(Equal(resources.Ptr(int32(1))))

			By("Verifying phase transitions back to Running")
			Eventually(func() string {
				inst := &openclawv1alpha1.OpenClawInstance{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: namespace}, inst)
				return inst.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.PhaseRunning))

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
