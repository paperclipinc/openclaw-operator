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
	"encoding/json"
	"os"
	"strings"
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

var _ = Describe("Skill Pack Resolution", func() {
	// The test skill pack lives in this repo at test/testdata/skill-packs/test-skill/.
	// The e2e test uses the current branch (via E2E_SKILL_PACK_REF) to resolve it
	// from GitHub, exercising the full GitHub Contents API flow.

	Context("When creating an instance with a pack: skill", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-skillpacks-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should resolve the skill pack and seed files into workspace ConfigMap", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			// Use the branch ref from env, or default to main.
			// CI sets E2E_SKILL_PACK_REF to the PR branch so the test
			// resolves the skill pack from the branch being tested.
			ref := os.Getenv("E2E_SKILL_PACK_REF")
			if ref == "" {
				ref = "main"
			}

			packRef := "pack:paperclipinc/openclaw-operator/test/testdata/skill-packs/test-skill@" + ref

			instanceName := "skillpack-test"

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
					Skills: []string{packRef},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for the StatefulSet to be created (signals reconciliation completed)
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, statefulSet)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			// Verify the workspace ConfigMap was created with skill pack files
			workspaceCM := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.WorkspaceConfigMapName(instance),
					Namespace: namespace,
				}, workspaceCM)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// Check that skill pack files are in the workspace ConfigMap
			skillMDKey := resources.SkillPackCMKey("skills/test-skill/SKILL.md")
			Expect(workspaceCM.Data).To(HaveKey(skillMDKey),
				"workspace ConfigMap should contain the SKILL.md file")
			Expect(workspaceCM.Data[skillMDKey]).To(ContainSubstring("test-skill"),
				"SKILL.md content should mention test-skill")

			helloKey := resources.SkillPackCMKey("skills/test-skill/scripts/hello.sh")
			Expect(workspaceCM.Data).To(HaveKey(helloKey),
				"workspace ConfigMap should contain the hello.sh script")
			Expect(workspaceCM.Data[helloKey]).To(ContainSubstring("Hello from test-skill"),
				"hello.sh content should contain the expected output")

			// Verify init container script has mkdir and cp for skill pack files
			var initConfig *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "init-config" {
					initConfig = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(initConfig).NotTo(BeNil(), "init-config container should exist")
			script := initConfig.Command[2]
			Expect(script).To(ContainSubstring("mkdir -p /data/workspace/'skills/test-skill/scripts'"),
				"init script should create skill pack directories")
			Expect(script).To(ContainSubstring(skillMDKey),
				"init script should copy SKILL.md from workspace ConfigMap")
			Expect(script).To(ContainSubstring(helloKey),
				"init script should copy hello.sh from workspace ConfigMap")

			// Default Replace policy (#564): pack files are copied
			// unconditionally and tracked in a manifest so a pinned revision
			// bump refreshes already-seeded contents.
			Expect(script).NotTo(ContainSubstring("[ -f /data/workspace/'skills/test-skill/SKILL.md' ] ||"),
				"Replace policy should not seed pack files conditionally")
			Expect(script).To(ContainSubstring("mv /data/.skillpack-manifest.new /data/.skillpack-manifest"),
				"init script should maintain the skill pack manifest (#564)")

			// Verify the config ConfigMap has skill entries injected
			configCM := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, configCM)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			configJSON := configCM.Data["openclaw.json"]
			Expect(configJSON).NotTo(BeEmpty(), "config ConfigMap should contain openclaw.json")

			var config map[string]interface{}
			Expect(json.Unmarshal([]byte(configJSON), &config)).To(Succeed())

			// Navigate to skills.entries
			skillsSection, ok := config["skills"].(map[string]interface{})
			if ok {
				entries, ok := skillsSection["entries"].(map[string]interface{})
				if ok {
					testEntry, ok := entries["test-skill"].(map[string]interface{})
					Expect(ok).To(BeTrue(), "config should have test-skill entry")
					Expect(testEntry["enabled"]).To(BeTrue(), "test-skill should be enabled")
				}
			}

			// pack: skills should NOT appear in the init-skills container
			// (they are handled by the init-config container, not clawhub/npm)
			for _, c := range statefulSet.Spec.Template.Spec.InitContainers {
				if c.Name == "init-skills" {
					script := strings.Join(c.Command, " ")
					Expect(script).NotTo(ContainSubstring("pack:"),
						"init-skills should not contain pack: entries")
				}
			}

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
