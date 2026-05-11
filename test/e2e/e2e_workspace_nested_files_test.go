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
	"sigs.k8s.io/controller-runtime/pkg/client"

	openclawv1alpha1 "github.com/openclawrocks/openclaw-operator/api/v1alpha1"
	"github.com/openclawrocks/openclaw-operator/internal/resources"
)

// Regression for #482: nested paths in spec.workspace.initialFiles must
// reconcile cleanly. Before the fix the operator wrote raw keys like
// "agents/AGENT.md" into the ConfigMap data, which Kubernetes rejected with
// "must consist of alphanumeric characters, '-', '_' or '.'", leaving the
// workspace ConfigMap unreconciled.
var _ = Describe("Workspace initialFiles with nested paths", func() {
	Context("When initialFiles keys contain '/'", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-ws-nested-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should reconcile the workspace ConfigMap with encoded keys and a mkdir+cp init script", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "ws-nested-test"
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
						InitialFiles: map[string]string{
							"agents/AGENT.md":         "# Agent",
							"skills/redmine/SKILL.md": "# Skill",
							"SOUL.md":                 "# Soul",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// StatefulSet must be created — proves reconcile reached the end.
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, statefulSet)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			// Workspace ConfigMap exists with encoded keys, never raw '/'.
			workspaceCM := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.WorkspaceConfigMapName(instance),
					Namespace: namespace,
				}, workspaceCM)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			Expect(workspaceCM.Data).To(HaveKeyWithValue("agents--AGENT.md", "# Agent"))
			Expect(workspaceCM.Data).To(HaveKeyWithValue("skills--redmine--SKILL.md", "# Skill"))
			Expect(workspaceCM.Data).To(HaveKeyWithValue("SOUL.md", "# Soul"))
			for k := range workspaceCM.Data {
				Expect(k).NotTo(ContainSubstring("/"),
					"ConfigMap data key %q must not contain '/'", k)
			}

			// Init script must recreate the nested layout on the workspace volume.
			var initConfig *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "init-config" {
					initConfig = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(initConfig).NotTo(BeNil(), "init-config container should exist")
			script := initConfig.Command[2]
			Expect(script).To(ContainSubstring("mkdir -p /data/workspace/'agents'"))
			Expect(script).To(ContainSubstring("mkdir -p /data/workspace/'skills/redmine'"))
			Expect(script).To(ContainSubstring(
				"cp /workspace-init/'agents--AGENT.md' /data/workspace/'agents/AGENT.md'"))
			Expect(script).To(ContainSubstring(
				"cp /workspace-init/'skills--redmine--SKILL.md' /data/workspace/'skills/redmine/SKILL.md'"))

			// The pre-fix behavior surfaced an "InvalidFilename" /
			// "ReconcileFailed" event referencing the ConfigMap "Invalid value"
			// error. The fact that we reached this point with the StatefulSet
			// and workspace ConfigMap created already proves the fix; verify
			// no such event was recorded.
			events := &corev1.EventList{}
			Expect(k8sClient.List(ctx, events, client.InNamespace(namespace))).To(Succeed())
			for _, e := range events.Items {
				Expect(e.Message).NotTo(ContainSubstring("must consist of alphanumeric characters"),
					"event %q should not reference the pre-fix ConfigMap validation error", e.Name)
			}

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
