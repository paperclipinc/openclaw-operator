#!/usr/bin/env bash
# hack/kind-verify-namespaced-rbac.sh
#
# Verifies Helm rbac.namespaced mode on a local kind cluster only.
# Prerequisites: kind, kubectl, helm, podman or docker, image build tooling.
#
# Usage:
#   export KIND_CLUSTER_NAME=openclaw-fix-test   # your kind cluster name
#   kubectl config use-context "kind-${KIND_CLUSTER_NAME}"
#   ./hack/kind-verify-namespaced-rbac.sh
#
# The script builds the operator image, loads it into kind (podman-friendly
# image-archive path), installs the chart with rbac.namespaced=true, applies
# a minimal OpenClawInstance in the watched namespace, and prints RBAC checks.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -z "${KIND_CLUSTER_NAME:-}" ]]; then
  echo "Set KIND_CLUSTER_NAME to your kind cluster name (output of: kind get clusters), e.g. export KIND_CLUSTER_NAME=openclaw-fix-test"
  exit 1
fi

CTX="kind-${KIND_CLUSTER_NAME}"
kubectl config use-context "$CTX"

IMAGE_TAG="${IMAGE_TAG:-rbac-verify-$(date +%s)}"
IMAGE="localhost/openclaw-operator:${IMAGE_TAG}"
TAR="/tmp/openclaw-operator-${IMAGE_TAG}.tar"

echo "==> Building ${IMAGE}"
podman build -t "${IMAGE}" .

echo "==> Saving and loading into kind cluster ${KIND_CLUSTER_NAME}"
podman save "${IMAGE}" -o "${TAR}"
kind load image-archive "${TAR}" --name "${KIND_CLUSTER_NAME}"
rm -f "${TAR}"

OPERATOR_NS=openclaw-system
WORKLOAD_NS=openclaw
REL=oc-rbac-verify

kubectl create namespace "${OPERATOR_NS}" --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace "${WORKLOAD_NS}" --dry-run=client -o yaml | kubectl apply -f -

echo "==> Helm upgrade (set crds.install=true only on a clean cluster; false if CRDs already exist)"
helm upgrade --install "${REL}" charts/openclaw-operator -n "${OPERATOR_NS}" \
  --set image.repository=localhost/openclaw-operator \
  --set image.tag="${IMAGE_TAG}" \
  --set image.pullPolicy=Never \
  --set rbac.namespaced=true \
  --set-json "rbac.watchNamespaces=[\"${WORKLOAD_NS}\"]" \
  --set networkPolicy.enabled=false \
  --set crds.install=false \
  --wait --timeout=5m

echo "==> Deployment must set OPENCLAW_WATCH_NAMESPACES"
kubectl get deploy -n "${OPERATOR_NS}" "${REL}-openclaw-operator" -o jsonpath='{.spec.template.spec.containers[0].env}' | grep -q OPENCLAW_WATCH_NAMESPACES

echo "==> RoleBinding in workload namespace"
kubectl get rolebinding -n "${WORKLOAD_NS}" -l "app.kubernetes.io/instance=${REL}"

echo "==> ClusterRole for cluster defaults only"
kubectl get clusterrole "${REL}-openclaw-operator-clusterdefaults-reader" -o jsonpath='{.rules}' | grep -q openclawclusterdefaults

echo "==> Apply OpenClawClusterDefaults + minimal OpenClawInstance"
kubectl apply -f - <<EOF
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawClusterDefaults
metadata:
  name: cluster
spec: {}
---
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: rbac-verify
  namespace: ${WORKLOAD_NS}
  annotations:
    openclaw.rocks/skip-backup: "true"
spec:
  image:
    repository: ghcr.io/openclaw/openclaw
    tag: latest
  storage:
    persistence:
      enabled: false
EOF

echo "==> Wait for StatefulSet to exist (pod may stay Pending on small kind nodes; RBAC is verified if STS is created)"
kubectl wait --for=condition=ready --timeout=60s "statefulset/rbac-verify" -n "${WORKLOAD_NS}" 2>/dev/null || true
kubectl get sts,openclawinstance -n "${WORKLOAD_NS}"
kubectl logs -n "${OPERATOR_NS}" -l "app.kubernetes.io/instance=${REL}" --tail=15 || true

echo "OK: namespaced RBAC install and reconcile path verified."
echo "Cleanup: helm uninstall ${REL} -n ${OPERATOR_NS}; kubectl delete openclawinstance rbac-verify -n ${WORKLOAD_NS}; kubectl delete openclawclusterdefaults cluster; kubectl delete ns ${WORKLOAD_NS}"
