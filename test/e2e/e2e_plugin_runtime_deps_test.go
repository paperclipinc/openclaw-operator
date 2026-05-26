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

// This suite verifies the regression fix for #462: bundled plugin extensions
// (confirmed: discord) crash loop after an OpenClaw image upgrade because they
// import "openclaw/..." as bare ESM specifiers and the Node resolver either
// finds nothing or finds a stale npm-cached package from the previous image.
//
// The fix adds an init-plugin-runtime-deps container that creates the symlink
// ~/.openclaw/plugin-runtime-deps/node_modules/openclaw -> /app on every pod
// start, so bundled plugins always resolve against the current image.
var _ = Describe("init-plugin-runtime-deps symlink (#462)", func() {
	Context("When creating any OpenClawInstance", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-plugin-runtime-deps-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should include an init-plugin-runtime-deps container that points openclaw at /app", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "plugin-runtime-deps",
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

			var c *corev1.Container
			for i := range sts.Spec.Template.Spec.InitContainers {
				if sts.Spec.Template.Spec.InitContainers[i].Name == "init-plugin-runtime-deps" {
					c = &sts.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(c).NotTo(BeNil(), "init-plugin-runtime-deps container missing from StatefulSet")

			// Must mount data at /data with no SubPath, so the symlink is written
			// into the PVC root (visible in the main container under ~/.openclaw).
			var found bool
			for _, m := range c.VolumeMounts {
				if m.Name != "data" {
					continue
				}
				Expect(m.SubPath).To(BeEmpty(),
					"init-plugin-runtime-deps data mount must not use SubPath (got %q)", m.SubPath)
				Expect(m.MountPath).To(Equal("/data"))
				found = true
			}
			Expect(found).To(BeTrue(), "init-plugin-runtime-deps: data volume mount not found")

			// Script must create the version-agnostic symlink, idempotently.
			Expect(c.Command).To(HaveLen(3), "init-plugin-runtime-deps should use sh -c <script>")
			script := c.Command[2]
			Expect(strings.Contains(script, "mkdir -p /data/plugin-runtime-deps/node_modules")).To(BeTrue(),
				"script must create plugin-runtime-deps/node_modules, got:\n%s", script)
			Expect(strings.Contains(script, "ln -sfn /app /data/plugin-runtime-deps/node_modules/openclaw")).To(BeTrue(),
				"script must symlink openclaw -> /app, got:\n%s", script)

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
