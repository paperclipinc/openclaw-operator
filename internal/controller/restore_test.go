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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

var _ = Describe("Restore from Backup", func() {
	const (
		timeout  = time.Second * 30
		interval = time.Millisecond * 250
	)

	Context("When creating an instance without restoreFrom", func() {
		It("Should proceed normally to Running without restore", func() {
			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-restore-test",
					Namespace: "default",
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			instanceKey := types.NamespacedName{Name: "no-restore-test", Namespace: "default"}

			// Should reach Running phase without going through Restoring
			Eventually(func() string {
				inst := &openclawv1alpha1.OpenClawInstance{}
				if err := k8sClient.Get(ctx, instanceKey, inst); err != nil {
					return ""
				}
				return inst.Status.Phase
			}, timeout, interval).Should(BeElementOf(
				openclawv1alpha1.PhaseRunning,
				openclawv1alpha1.PhaseProvisioning,
			))

			// Verify no restore Job was created
			jobList := &batchv1.JobList{}
			Expect(k8sClient.List(ctx, jobList)).Should(Succeed())
			for _, job := range jobList.Items {
				Expect(job.Labels).NotTo(HaveKeyWithValue("openclaw.rocks/job-type", "restore"))
			}

			// Clean up: add skip-backup BEFORE deleting to prevent backup flow
			Eventually(func() error {
				inst := &openclawv1alpha1.OpenClawInstance{}
				if err := k8sClient.Get(ctx, instanceKey, inst); err != nil {
					return err
				}
				if inst.Annotations == nil {
					inst.Annotations = map[string]string{}
				}
				inst.Annotations[AnnotationSkipBackup] = "true"
				return k8sClient.Update(ctx, inst)
			}, timeout, interval).Should(Succeed())
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
			// Wait for full deletion
			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceKey, &openclawv1alpha1.OpenClawInstance{})
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When creating an instance with restoreFrom", func() {
		It("Should enter Restoring phase and create a restore Job", func() {
			// Ensure S3 credentials exist
			s3Secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      BackupSecretName,
					Namespace: "default",
				},
				Data: map[string][]byte{
					"S3_BUCKET":            []byte("test-bucket"),
					"S3_ACCESS_KEY_ID":     []byte("key123"),
					"S3_SECRET_ACCESS_KEY": []byte("secret456"),
					"S3_ENDPOINT":          []byte("https://s3.example.com"),
				},
			}
			_ = k8sClient.Create(ctx, s3Secret)

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "restore-test",
					Namespace: "default",
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					RestoreFrom: "backups/cus_123/old-instance/2026-01-01T000000Z",
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			instanceKey := types.NamespacedName{Name: "restore-test", Namespace: "default"}

			// Should enter Restoring phase
			Eventually(func() string {
				inst := &openclawv1alpha1.OpenClawInstance{}
				if err := k8sClient.Get(ctx, instanceKey, inst); err != nil {
					return ""
				}
				return inst.Status.Phase
			}, timeout, interval).Should(Equal(openclawv1alpha1.PhaseRestoring))

			// Verify restore Job was created
			Eventually(func() bool {
				job := &batchv1.Job{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "restore-test-restore",
					Namespace: "default",
				}, job)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Verify the Job has correct labels
			job := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "restore-test-restore",
				Namespace: "default",
			}, job)).Should(Succeed())
			Expect(job.Labels["openclaw.rocks/job-type"]).To(Equal("restore"))

			// Clean up: delete the instance with skip-backup
			Eventually(func() error {
				inst := &openclawv1alpha1.OpenClawInstance{}
				if err := k8sClient.Get(ctx, instanceKey, inst); err != nil {
					return err
				}
				if inst.Annotations == nil {
					inst.Annotations = map[string]string{}
				}
				inst.Annotations[AnnotationSkipBackup] = "true"
				return k8sClient.Update(ctx, inst)
			}, timeout, interval).Should(Succeed())
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
