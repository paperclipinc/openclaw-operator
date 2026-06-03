package conformance

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Conformance suite: categories live in sibling files:
//   - negative_test.go             webhook + CEL deny paths
//   - idempotency_test.go          re-reconcile no-op canary
//   - gitops_coexistence_test.go   SSA does not flap with external writers
//   - upgrade_test.go              prior-release -> HEAD chart upgrade
//   - failure_modes_test.go        operator restart mid-reconcile recovery
//
// All scenarios run against a live kind cluster with the operator installed.
// The suite is skipped when KUBECONFIG is unset so `go test ./...` stays green
// on workstations without a cluster.
func TestConformance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "openclaw-operator conformance suite")
}

var (
	suiteCtx    context.Context
	suiteCancel context.CancelFunc
)

var _ = BeforeSuite(func() {
	suiteCtx, suiteCancel = context.WithCancel(context.Background())
	SetDefaultEventuallyTimeout(5 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)
	if os.Getenv("KUBECONFIG") == "" {
		Skip("KUBECONFIG not set: conformance suite requires a live kind cluster with the operator installed")
	}
})

var _ = AfterSuite(func() {
	if suiteCancel != nil {
		suiteCancel()
	}
})
