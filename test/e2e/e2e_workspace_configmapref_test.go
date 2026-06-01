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

var _ = Describe("Workspace ConfigMapRef", func() {
	Context("When creating an instance with spec.workspace.configMapRef", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-ws-cmref-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should merge external ConfigMap files into the workspace", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			// 1. Create the external ConfigMap with workspace files
			externalCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-workspace",
					Namespace: namespace,
				},
				Data: map[string]string{
					"SOUL.md":  "# Soul from external ConfigMap",
					"AGENT.md": "# Agent from external ConfigMap",
				},
			}
			Expect(k8sClient.Create(ctx, externalCM)).Should(Succeed())

			// 2. Create the instance referencing the external ConfigMap
			instanceName := "ws-cmref-test"
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
					Workspace: &openclawv1alpha1.WorkspaceSpec{
						ConfigMapRef: &openclawv1alpha1.ConfigMapNameSelector{
							Name: "agent-workspace",
						},
						InitialFiles: map[string]string{
							"EXTRA.md": "# Inline file",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// 3. Wait for the StatefulSet to be created
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, statefulSet)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			// 4. Verify the operator-managed workspace ConfigMap contains merged content
			workspaceCM := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.WorkspaceConfigMapName(instance),
					Namespace: namespace,
				}, workspaceCM)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// External files should be present
			Expect(workspaceCM.Data).To(HaveKey("SOUL.md"),
				"workspace ConfigMap should contain SOUL.md from external ConfigMap")
			Expect(workspaceCM.Data["SOUL.md"]).To(Equal("# Soul from external ConfigMap"))

			Expect(workspaceCM.Data).To(HaveKey("AGENT.md"),
				"workspace ConfigMap should contain AGENT.md from external ConfigMap")
			Expect(workspaceCM.Data["AGENT.md"]).To(Equal("# Agent from external ConfigMap"))

			// Inline file should be present
			Expect(workspaceCM.Data).To(HaveKey("EXTRA.md"),
				"workspace ConfigMap should contain EXTRA.md from inline initialFiles")
			Expect(workspaceCM.Data["EXTRA.md"]).To(Equal("# Inline file"))

			// Operator-injected files should always be present
			Expect(workspaceCM.Data).To(HaveKey("ENVIRONMENT.md"),
				"workspace ConfigMap should always contain ENVIRONMENT.md")
			Expect(workspaceCM.Data).To(HaveKey("BOOTSTRAP.md"),
				"workspace ConfigMap should always contain BOOTSTRAP.md")

			// 5. Verify init container script has copy commands for external files
			var initConfig *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "init-config" {
					initConfig = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(initConfig).NotTo(BeNil(), "init-config container should exist")
			script := initConfig.Command[2]
			Expect(script).To(ContainSubstring("SOUL.md"),
				"init script should reference SOUL.md from external ConfigMap")
			Expect(script).To(ContainSubstring("AGENT.md"),
				"init script should reference AGENT.md from external ConfigMap")
			Expect(script).To(ContainSubstring("EXTRA.md"),
				"init script should reference EXTRA.md from inline files")

			// 6. Verify WorkspaceReady condition is True
			updatedInstance := &openclawv1alpha1.OpenClawInstance{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, updatedInstance); err != nil {
					return false
				}
				for _, c := range updatedInstance.Status.Conditions {
					if c.Type == openclawv1alpha1.ConditionTypeWorkspaceReady {
						return c.Status == metav1.ConditionTrue
					}
				}
				return false
			}, 30*time.Second, 2*time.Second).Should(BeTrue(),
				"WorkspaceReady condition should be True")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should set WorkspaceReady=False when ConfigMap is missing", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "ws-cmref-missing"
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
					Workspace: &openclawv1alpha1.WorkspaceSpec{
						ConfigMapRef: &openclawv1alpha1.ConfigMapNameSelector{
							Name: "nonexistent-cm",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify WorkspaceReady condition is False
			updatedInstance := &openclawv1alpha1.OpenClawInstance{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, updatedInstance); err != nil {
					return false
				}
				for _, c := range updatedInstance.Status.Conditions {
					if c.Type == openclawv1alpha1.ConditionTypeWorkspaceReady {
						return c.Status == metav1.ConditionFalse && c.Reason == "ConfigMapNotFound"
					}
				}
				return false
			}, 30*time.Second, 2*time.Second).Should(BeTrue(),
				"WorkspaceReady condition should be False with reason ConfigMapNotFound")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
