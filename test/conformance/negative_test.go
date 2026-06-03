package conformance

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// negativeCase describes one deny scenario exercised via kubectl apply. On the
// default Helm install the validating webhook is not deployed (the chart ships
// webhook.enabled=false and cmd/main.go does not register it), so the deny
// paths exercised here are the ones the API server enforces unconditionally:
// CRD CEL rules (x-kubernetes-validations) and structural-schema constraints
// (enums, required fields). Webhook-only business rules are listed but skipped
// with a reason so a future engineer who wires the webhook into the chart can
// flip them on.
type negativeCase struct {
	name             string
	yaml             string
	wantErrSubstring string
	skip             string
}

var negativeCases = []negativeCase{
	// CEL: image tag empty and no digest is rejected at the API server.
	{
		name: "deny: image tag empty with no digest (CEL)",
		yaml: `apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: neg-image-empty-tag
spec:
  image:
    repository: ghcr.io/openclaw/openclaw
    tag: ""
`,
		wantErrSubstring: "image",
	},

	// Structural schema: image.pullPolicy is an enum; an unknown value is
	// rejected by the API server regardless of the webhook.
	{
		name: "deny: image.pullPolicy invalid enum value",
		yaml: `apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: neg-bad-pullpolicy
spec:
  image:
    repository: ghcr.io/openclaw/openclaw
    tag: v1.0.0
    pullPolicy: Sometimes
`,
		wantErrSubstring: "pullPolicy",
	},

	// Webhook-only rule: resource limits required. Skipped until the webhook
	// is deployed by the chart.
	{
		name: "deny: missing resource limits (webhook)",
		yaml: `apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: neg-no-limits
spec:
  image:
    repository: ghcr.io/openclaw/openclaw
    tag: v1.0.0
`,
		wantErrSubstring: "limits",
		skip:             "validating webhook is not deployed by the default Helm install (webhook.enabled=false)",
	},

	// Webhook-only rule: running as root (UID 0) is rejected. Skipped until
	// the webhook is deployed by the chart.
	{
		name: "deny: runAsUser 0 / root (webhook)",
		yaml: `apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: neg-root
spec:
  image:
    repository: ghcr.io/openclaw/openclaw
    tag: v1.0.0
  resources:
    limits:
      cpu: "2"
      memory: 2Gi
  security:
    podSecurityContext:
      runAsUser: 0
`,
		wantErrSubstring: "root",
		skip:             "validating webhook is not deployed by the default Helm install (webhook.enabled=false)",
	},
}

var _ = Describe("negative: API server deny paths (CEL and structural schema)", Ordered, func() {
	var ns string

	BeforeAll(func() {
		ns = freshNamespace("neg-test")
		DeferCleanup(func() {
			deleteNamespace(ns)
		})
	})

	for _, tc := range negativeCases {
		It(tc.name, func() {
			if tc.skip != "" {
				Skip(tc.skip)
			}

			_, err := kubectlApply(addNamespace(tc.yaml, ns))
			Expect(err).To(HaveOccurred(), "expected denial but apply succeeded")
			Expect(err.Error()).To(ContainSubstring(tc.wantErrSubstring),
				"error message should mention %q", tc.wantErrSubstring)
		})
	}
})
