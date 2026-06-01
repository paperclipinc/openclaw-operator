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

var _ = Describe("Additional Workspaces", func() {
	Context("When creating an instance with additionalWorkspaces", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-ws-addl-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should create namespaced keys in the workspace ConfigMap and init script", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			// 1. Create external ConfigMap for the "work" agent workspace
			externalCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-agent-files",
					Namespace: namespace,
				},
				Data: map[string]string{
					"SOUL.md": "# Work agent soul from ConfigMap",
				},
			}
			Expect(k8sClient.Create(ctx, externalCM)).Should(Succeed())

			// 2. Create instance with additionalWorkspaces
			instanceName := "ws-addl-test"
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
						AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
							{
								Name: "work",
								ConfigMapRef: &openclawv1alpha1.ConfigMapNameSelector{
									Name: "work-agent-files",
								},
								InitialFiles: map[string]string{
									"AGENT.md": "# Inline work agent file",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// 3. Wait for StatefulSet
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, statefulSet)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			// 4. Verify operator-managed workspace ConfigMap has namespaced keys
			workspaceCM := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.WorkspaceConfigMapName(instance),
					Namespace: namespace,
				}, workspaceCM)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// External file should be present with namespaced key
			soulKey := resources.AdditionalWorkspaceCMKey("work", "SOUL.md")
			Expect(workspaceCM.Data).To(HaveKey(soulKey),
				"workspace ConfigMap should contain namespaced SOUL.md from external ConfigMap")
			Expect(workspaceCM.Data[soulKey]).To(Equal("# Work agent soul from ConfigMap"))

			// Inline file should be present with namespaced key
			agentKey := resources.AdditionalWorkspaceCMKey("work", "AGENT.md")
			Expect(workspaceCM.Data).To(HaveKey(agentKey),
				"workspace ConfigMap should contain namespaced AGENT.md from inline initialFiles")
			Expect(workspaceCM.Data[agentKey]).To(Equal("# Inline work agent file"))

			// ENVIRONMENT.md should be injected for additional workspace
			envKey := resources.AdditionalWorkspaceCMKey("work", "ENVIRONMENT.md")
			Expect(workspaceCM.Data).To(HaveKey(envKey),
				"workspace ConfigMap should contain ENVIRONMENT.md for additional workspace")

			// Default workspace operator files should also be present
			Expect(workspaceCM.Data).To(HaveKey("ENVIRONMENT.md"))
			Expect(workspaceCM.Data).To(HaveKey("BOOTSTRAP.md"))

			// 5. Verify init script has mkdir and copy commands for additional workspace
			var initConfig *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "init-config" {
					initConfig = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(initConfig).NotTo(BeNil(), "init-config container should exist")
			script := initConfig.Command[2]
			Expect(script).To(ContainSubstring("workspace-work"),
				"init script should reference workspace-work directory")
			Expect(script).To(ContainSubstring(soulKey),
				"init script should reference namespaced SOUL.md key")

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

		It("Should set WorkspaceReady=False when additional workspace ConfigMap is missing", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "ws-addl-missing"
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
						AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
							{
								Name: "work",
								ConfigMapRef: &openclawv1alpha1.ConfigMapNameSelector{
									Name: "nonexistent-work-cm",
								},
							},
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
