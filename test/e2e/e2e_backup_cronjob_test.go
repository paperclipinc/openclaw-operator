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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

var _ = Describe("Periodic Backup CronJob", func() {
	const (
		timeout  = time.Second * 60
		interval = time.Second * 1
	)

	Context("When creating an instance with spec.backup.schedule", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-backup-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			// Create the s3-backup-credentials Secret in the operator namespace.
			// The operator reads credentials from its own namespace, so we create
			// it in the namespace where the operator runs (set via OPERATOR_NAMESPACE
			// env or defaulting to "openclaw-operator-system").
			operatorNS := os.Getenv("OPERATOR_NAMESPACE")
			if operatorNS == "" {
				operatorNS = "openclaw-operator-system"
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-backup-credentials",
					Namespace: operatorNS,
				},
				StringData: map[string]string{
					"S3_BUCKET":            "test-bucket",
					"S3_ACCESS_KEY_ID":     "test-key",
					"S3_SECRET_ACCESS_KEY": "test-secret",
					"S3_ENDPOINT":          "https://s3.example.com",
				},
			}
			err := k8sClient.Create(ctx, secret)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		AfterEach(func() {
			// Clean up the S3 credentials Secret so other tests that expect
			// "no S3 credentials" are not affected by our setup.
			operatorNS := os.Getenv("OPERATOR_NAMESPACE")
			if operatorNS == "" {
				operatorNS = "openclaw-operator-system"
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-backup-credentials",
					Namespace: operatorNS,
				},
			}
			_ = k8sClient.Delete(ctx, secret)

			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should create a CronJob when schedule is set", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "backup-cron-test"
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
					Backup: openclawv1alpha1.BackupSpec{
						Schedule: "0 2 * * *",
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify CronJob is created
			cronJob := &batchv1.CronJob{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName + "-backup-periodic",
					Namespace: namespace,
				}, cronJob)
			}, timeout, interval).Should(Succeed())

			Expect(cronJob.Spec.Schedule).To(Equal("0 2 * * *"))
			Expect(cronJob.Spec.ConcurrencyPolicy).To(Equal(batchv1.ForbidConcurrent))
			Expect(cronJob.Spec.StartingDeadlineSeconds).NotTo(BeNil())
			Expect(*cronJob.Spec.StartingDeadlineSeconds).To(Equal(int64(600)))
			Expect(cronJob.Spec.JobTemplate.Spec.ActiveDeadlineSeconds).NotTo(BeNil())
			Expect(*cronJob.Spec.JobTemplate.Spec.ActiveDeadlineSeconds).To(Equal(int64(3600)))

			// Verify mirror Secret was created in instance namespace
			mirrorSecret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName + "-s3-credentials",
					Namespace: namespace,
				}, mirrorSecret)
			}, timeout, interval).Should(Succeed())
			Expect(mirrorSecret.Data).To(HaveKey("S3_ACCESS_KEY_ID"))
			Expect(mirrorSecret.Data).To(HaveKey("S3_SECRET_ACCESS_KEY"))

			// Verify credentials use secretKeyRef (not plaintext in Job spec)
			container := cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
			for _, e := range container.Env {
				if e.Name == "S3_ACCESS_KEY_ID" || e.Name == "S3_SECRET_ACCESS_KEY" {
					Expect(e.Value).To(BeEmpty(), "credential env var %s should not have plaintext Value", e.Name)
					Expect(e.ValueFrom).NotTo(BeNil(), "credential env var %s should use ValueFrom", e.Name)
					Expect(e.ValueFrom.SecretKeyRef).NotTo(BeNil())
					Expect(e.ValueFrom.SecretKeyRef.Name).To(Equal(instanceName + "-s3-credentials"))
				}
			}

			// Verify ScheduledBackupReady condition
			Eventually(func() bool {
				updatedInstance := &openclawv1alpha1.OpenClawInstance{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, updatedInstance); err != nil {
					return false
				}
				for _, c := range updatedInstance.Status.Conditions {
					if c.Type == openclawv1alpha1.ConditionTypeScheduledBackupReady && c.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())

			// Verify CronJob is deleted via owner reference
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName + "-backup-periodic",
					Namespace: namespace,
				}, cronJob)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should delete CronJob when schedule is removed", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "backup-cron-remove"
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
					Backup: openclawv1alpha1.BackupSpec{
						Schedule: "0 3 * * *",
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for CronJob to be created
			cronJob := &batchv1.CronJob{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName + "-backup-periodic",
					Namespace: namespace,
				}, cronJob)
			}, timeout, interval).Should(Succeed())

			// Remove the schedule
			updatedInstance := &openclawv1alpha1.OpenClawInstance{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      instanceName,
				Namespace: namespace,
			}, updatedInstance)).Should(Succeed())
			updatedInstance.Spec.Backup.Schedule = ""
			Expect(k8sClient.Update(ctx, updatedInstance)).Should(Succeed())

			// Verify CronJob is deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName + "-backup-periodic",
					Namespace: namespace,
				}, cronJob)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Clean up
			Expect(k8sClient.Delete(ctx, updatedInstance)).Should(Succeed())
		})

		It("Should set serviceAccountName on CronJob pod when spec.backup.serviceAccountName is set", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "backup-cron-sa"
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
					Backup: openclawv1alpha1.BackupSpec{
						Schedule:           "0 5 * * *",
						ServiceAccountName: "my-irsa-sa",
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify CronJob is created with the ServiceAccountName
			cronJob := &batchv1.CronJob{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName + "-backup-periodic",
					Namespace: namespace,
				}, cronJob)
			}, timeout, interval).Should(Succeed())

			podSpec := cronJob.Spec.JobTemplate.Spec.Template.Spec
			Expect(podSpec.ServiceAccountName).To(Equal("my-irsa-sa"))

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should use --s3-env-auth in CronJob when static credentials are omitted", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			// Recreate the s3-backup-credentials Secret without static credentials
			operatorNS := os.Getenv("OPERATOR_NAMESPACE")
			if operatorNS == "" {
				operatorNS = "openclaw-operator-system"
			}
			// Delete existing secret and recreate without AK/SK
			existingSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-backup-credentials",
					Namespace: operatorNS,
				},
			}
			_ = k8sClient.Delete(ctx, existingSecret)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "s3-backup-credentials",
					Namespace: operatorNS,
				}, existingSecret)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			envAuthSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-backup-credentials",
					Namespace: operatorNS,
				},
				StringData: map[string]string{
					"S3_BUCKET":   "test-bucket",
					"S3_ENDPOINT": "https://s3.us-east-1.amazonaws.com",
				},
			}
			Expect(k8sClient.Create(ctx, envAuthSecret)).Should(Succeed())

			instanceName := "backup-cron-envauth"
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
					Backup: openclawv1alpha1.BackupSpec{
						Schedule: "0 6 * * *",
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify CronJob is created with --s3-env-auth=true in the rclone command
			cronJob := &batchv1.CronJob{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName + "-backup-periodic",
					Namespace: namespace,
				}, cronJob)
			}, timeout, interval).Should(Succeed())

			container := cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
			Expect(container.Command[2]).To(ContainSubstring("--s3-env-auth=true"))
			Expect(container.Command[2]).NotTo(ContainSubstring("--s3-access-key-id"))

			// Verify no static credential env vars
			var envNames []string
			for _, e := range container.Env {
				envNames = append(envNames, e.Name)
			}
			Expect(envNames).To(ContainElement("S3_ENDPOINT"))
			Expect(envNames).NotTo(ContainElement("S3_ACCESS_KEY_ID"))
			Expect(envNames).NotTo(ContainElement("S3_SECRET_ACCESS_KEY"))

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should include --s3-region flag and S3_REGION env var when S3_REGION is set in credentials Secret", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			// Recreate the s3-backup-credentials Secret with S3_REGION
			operatorNS := os.Getenv("OPERATOR_NAMESPACE")
			if operatorNS == "" {
				operatorNS = "openclaw-operator-system"
			}
			existingSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-backup-credentials",
					Namespace: operatorNS,
				},
			}
			_ = k8sClient.Delete(ctx, existingSecret)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "s3-backup-credentials",
					Namespace: operatorNS,
				}, existingSecret)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			regionSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-backup-credentials",
					Namespace: operatorNS,
				},
				StringData: map[string]string{
					"S3_BUCKET":            "test-bucket",
					"S3_ACCESS_KEY_ID":     "test-key",
					"S3_SECRET_ACCESS_KEY": "test-secret",
					"S3_ENDPOINT":          "https://s3.us-west-2.amazonaws.com",
					"S3_REGION":            "us-west-2",
				},
			}
			Expect(k8sClient.Create(ctx, regionSecret)).Should(Succeed())

			instanceName := "backup-cron-region"
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
					Backup: openclawv1alpha1.BackupSpec{
						Schedule: "0 7 * * *",
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify CronJob is created with --s3-region in the rclone command
			cronJob := &batchv1.CronJob{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName + "-backup-periodic",
					Namespace: namespace,
				}, cronJob)
			}, timeout, interval).Should(Succeed())

			container := cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
			Expect(container.Command[2]).To(ContainSubstring("--s3-region"))

			// Verify S3_REGION env var is present
			var regionEnv *corev1.EnvVar
			for i, e := range container.Env {
				if e.Name == "S3_REGION" {
					regionEnv = &container.Env[i]
					break
				}
			}
			Expect(regionEnv).NotTo(BeNil())
			Expect(regionEnv.Value).To(Equal("us-west-2"))

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should propagate nodeSelector and tolerations to CronJob pod template", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "backup-cron-nodeselector"
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
					Backup: openclawv1alpha1.BackupSpec{
						Schedule: "0 4 * * *",
					},
					Availability: openclawv1alpha1.AvailabilitySpec{
						NodeSelector: map[string]string{
							"openclaw.rocks/nodepool": "openclaw",
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "openclaw.rocks/dedicated",
								Operator: corev1.TolerationOpEqual,
								Value:    "openclaw",
								Effect:   corev1.TaintEffectNoSchedule,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify CronJob is created with nodeSelector and tolerations
			cronJob := &batchv1.CronJob{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName + "-backup-periodic",
					Namespace: namespace,
				}, cronJob)
			}, timeout, interval).Should(Succeed())

			podSpec := cronJob.Spec.JobTemplate.Spec.Template.Spec
			Expect(podSpec.NodeSelector).To(HaveKeyWithValue("openclaw.rocks/nodepool", "openclaw"))
			Expect(podSpec.Tolerations).To(HaveLen(1))
			Expect(podSpec.Tolerations[0].Key).To(Equal("openclaw.rocks/dedicated"))
			Expect(podSpec.Tolerations[0].Value).To(Equal("openclaw"))
			Expect(podSpec.Tolerations[0].Effect).To(Equal(corev1.TaintEffectNoSchedule))

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
