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

// This suite verifies the opt-out behavior for operator-managed BOOTSTRAP.md
// injection. OpenClaw deletes BOOTSTRAP.md after applying it. If the operator
// re-injects on every pod restart, the agent runs bootstrap repeatedly (#463).
// spec.workspace.bootstrap.enabled=false suppresses both the ConfigMap entry
// and the init-script re-copy.
var _ = Describe("Workspace bootstrap disable (#463)", func() {
	Context("When spec.workspace.bootstrap.enabled is false", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-bootstrap-disable-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should omit BOOTSTRAP.md from the workspace ConfigMap and init script", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			falseVal := false
			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-bootstrap",
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
						Persistence: openclawv1alpha1.PersistenceSpec{
							Size: "1Gi",
						},
					},
					Workspace: &openclawv1alpha1.WorkspaceSpec{
						Bootstrap: openclawv1alpha1.BootstrapSpec{
							Enabled: &falseVal,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for the operator-managed workspace ConfigMap.
			workspaceCM := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.WorkspaceConfigMapName(instance),
					Namespace: namespace,
				}, workspaceCM)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			Expect(workspaceCM.Data).NotTo(HaveKey("BOOTSTRAP.md"),
				"workspace ConfigMap must not contain BOOTSTRAP.md when bootstrap.enabled=false")
			Expect(workspaceCM.Data).To(HaveKey("ENVIRONMENT.md"),
				"ENVIRONMENT.md is still always injected")

			// Wait for the StatefulSet so we can inspect the init script.
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, sts)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			var initConfig *corev1.Container
			for i := range sts.Spec.Template.Spec.InitContainers {
				if sts.Spec.Template.Spec.InitContainers[i].Name == "init-config" {
					initConfig = &sts.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(initConfig).NotTo(BeNil(), "init-config container should exist")
			Expect(initConfig.Command).To(HaveLen(3))
			script := initConfig.Command[2]
			Expect(script).NotTo(ContainSubstring("BOOTSTRAP.md"),
				"init script must not reference BOOTSTRAP.md when bootstrap.enabled=false")
			Expect(script).To(ContainSubstring("ENVIRONMENT.md"),
				"init script should still seed ENVIRONMENT.md")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
