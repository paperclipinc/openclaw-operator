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

var _ = Describe("Disk-aware readiness guard", func() {
	const (
		timeout  = time.Second * 60
		interval = time.Second * 2
	)

	Context("When an instance opts in to spec.probes.diskReadiness", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-disk-readiness-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should render the readiness probe as an exec disk check while liveness/startup stay HTTP", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "disk-readiness",
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
					Probes: &openclawv1alpha1.ProbesSpec{
						DiskReadiness: &openclawv1alpha1.DiskReadinessSpec{
							Enabled: resources.Ptr(true),
							MinFree: "128Mi",
						},
					},
				},
			}

			By("Creating an instance with diskReadiness enabled")
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			By("Waiting for the StatefulSet to be reconciled")
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, sts)
			}, timeout, interval).Should(Succeed())

			main := sts.Spec.Template.Spec.Containers[0]

			By("Verifying the readiness probe is an exec disk-aware check")
			Expect(main.ReadinessProbe).NotTo(BeNil())
			Expect(main.ReadinessProbe.Exec).NotTo(BeNil(), "readiness probe should be an exec probe when diskReadiness is enabled")
			Expect(main.ReadinessProbe.HTTPGet).To(BeNil(), "readiness probe should not retain an HTTP handler")
			Expect(main.ReadinessProbe.Exec.Command).To(HaveLen(3))
			script := main.ReadinessProbe.Exec.Command[2]
			Expect(script).To(ContainSubstring(resources.WorkspaceDataMountPath), "exec script should check the default workspace mount")
			Expect(script).To(ContainSubstring("df -Pk"), "exec script should perform a free-space check")
			Expect(script).To(ContainSubstring("-w \"$p\""), "exec script should perform a writability check")
			Expect(script).To(ContainSubstring("134217728"), "exec script should embed the 128Mi threshold in bytes")
			Expect(script).To(ContainSubstring("/readyz"), "exec script should still defer to the gateway /readyz signal")

			By("Verifying liveness and startup probes remain HTTP GET /healthz")
			Expect(main.LivenessProbe).NotTo(BeNil())
			Expect(main.LivenessProbe.HTTPGet).NotTo(BeNil(), "liveness probe should stay HTTP so a full PVC does not CrashLoop")
			Expect(main.LivenessProbe.HTTPGet.Path).To(Equal("/healthz"))
			Expect(main.StartupProbe).NotTo(BeNil())
			Expect(main.StartupProbe.HTTPGet).NotTo(BeNil(), "startup probe should stay HTTP")
			Expect(main.StartupProbe.HTTPGet.Path).To(Equal("/healthz"))

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
