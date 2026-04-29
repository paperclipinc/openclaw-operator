/*
Copyright 2026 OpenClaw.rocks

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package e2e

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/openclawrocks/openclaw-operator/api/v1alpha1"
)

// Optional smoke test when the operator is deployed with Helm rbac.namespaced=true
// and OPENCLAW_WATCH_NAMESPACES matching E2E_NAMESPACED_RBAC_NAMESPACE.
// Install example:
//
//	helm upgrade --install oc charts/openclaw-operator -n openclaw \
//	  --set rbac.namespaced=true --set 'rbac.watchNamespaces={YOUR_NS}'
//
// Then: E2E_NAMESPACED_RBAC=true E2E_NAMESPACED_RBAC_NAMESPACE=YOUR_NS ginkgo run ./test/e2e/...
var _ = Describe("Namespaced RBAC smoke (optional)", func() {
	const (
		timeout  = time.Second * 120
		interval = time.Second * 2
	)

	BeforeEach(func() {
		if os.Getenv("E2E_NAMESPACED_RBAC") != "true" {
			Skip("set E2E_NAMESPACED_RBAC=true and deploy the operator with matching OPENCLAW_WATCH_NAMESPACES")
		}
	})

	It("reconciles an OpenClawInstance in the watched namespace", func() {
		ns := os.Getenv("E2E_NAMESPACED_RBAC_NAMESPACE")
		if ns == "" {
			Skip("E2E_NAMESPACED_RBAC_NAMESPACE is required when E2E_NAMESPACED_RBAC=true")
		}

		name := "smoke-ns-rbac"
		instance := &openclawv1alpha1.OpenClawInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Annotations: map[string]string{
					"openclaw.rocks/skip-backup": "true",
				},
			},
			Spec: openclawv1alpha1.OpenClawInstanceSpec{
				Image: openclawv1alpha1.ImageSpec{
					Repository: "ghcr.io/openclaw/openclaw",
					Tag:        "latest",
				},
			},
		}

		if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
			Skip("Skipping when E2E_SKIP_RESOURCE_VALIDATION=true")
		}

		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, instance) }()

		sts := &appsv1.StatefulSet{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, sts)
		}, timeout, interval).Should(Succeed())
	})
})
