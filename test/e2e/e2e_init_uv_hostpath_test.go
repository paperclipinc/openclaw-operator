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

// This suite verifies the regression fix for #448: init-uv failing with
// "mkdir: cannot create directory '/home/openclaw/.local/bin': Permission
// denied" on hostPath-backed PVCs (e.g. Rancher local-path-provisioner on
// Talos). kind itself uses a hostPath-backed local-path storage class, so a
// persistent OpenClawInstance running on kind exercises the same code path.
//
// The fix mounts the full data volume in init-uv and init-pip (instead of
// SubPath .local). That lets the non-root init container create .local,
// .cache, .config, and skills with UID 1000 ownership before any SubPath
// mount is attempted. Without this, kubelet creates the missing SubPath
// directories as root:root and fsGroup cannot chown hostPath volumes.
var _ = Describe("Init containers on hostPath-backed PVCs (#448)", func() {
	Context("When creating a persistent OpenClawInstance", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-init-hostpath-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should configure init-uv/init-pip to mount the full data volume (not SubPath .local)", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "init-hostpath"
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

			var initUv, initPip *corev1.Container
			for i := range sts.Spec.Template.Spec.InitContainers {
				c := &sts.Spec.Template.Spec.InitContainers[i]
				switch c.Name {
				case "init-uv":
					initUv = c
				case "init-pip":
					initPip = c
				}
			}
			Expect(initUv).NotTo(BeNil(), "init-uv container missing from StatefulSet")
			Expect(initPip).NotTo(BeNil(), "init-pip container missing from StatefulSet")

			// Neither init container may mount the data PVC via a SubPath.
			// SubPath directories created by kubelet are root:root on
			// hostPath-backed PVCs where fsGroup ownership is not applied,
			// so a non-root init container could not write into them.
			assertFullDataMount := func(c *corev1.Container) {
				var found bool
				for _, m := range c.VolumeMounts {
					if m.Name != "data" {
						continue
					}
					Expect(m.SubPath).To(BeEmpty(),
						"%s: data mount must not use SubPath (got %q) - SubPath dirs are created root:root on hostPath PVCs",
						c.Name, m.SubPath)
					Expect(m.MountPath).To(Equal("/data"),
						"%s: data mount path should be /data", c.Name)
					found = true
				}
				Expect(found).To(BeTrue(), "%s: data volume mount not found", c.Name)
			}
			assertFullDataMount(initUv)
			assertFullDataMount(initPip)

			// init-uv must pre-create the SubPath dirs so later containers
			// mounting .local, .cache, and skills via SubPath inherit the
			// UID 1000 ownership from the pre-existing directory.
			Expect(initUv.Command).To(HaveLen(3), "init-uv should use sh -c <script>")
			script := initUv.Command[2]
			for _, dir := range []string{"/data/.local/bin", "/data/.cache", "/data/.config", "/data/skills"} {
				Expect(strings.Contains(script, dir)).To(BeTrue(),
					"init-uv script must pre-create %s with UID 1000 ownership, got:\n%s", dir, script)
			}

			// init-pip must set HOME=/data so ensurepip --user writes to
			// /data/.local (the now-mounted path) instead of the old
			// /home/openclaw/.local SubPath.
			var homeVal string
			for _, e := range initPip.Env {
				if e.Name == "HOME" {
					homeVal = e.Value
				}
			}
			Expect(homeVal).To(Equal("/data"),
				"init-pip HOME should be /data so ~/.local resolves to /data/.local")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
