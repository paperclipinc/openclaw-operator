#!/usr/bin/env bash
# hack/check-helm-cluster-rbac.sh
#
# Regression guard for #529: OpenClawClusterDefaults is a cluster-scoped
# resource, so the operator must always be granted access to it via a
# ClusterRole -- including when it is configured to watch only specific
# namespaces. A namespaced Role cannot grant access to a cluster-scoped
# resource; without a ClusterRole the manager cache fails to sync and the
# operator shuts down.
#
# This renders the chart with `helm template` in both modes and asserts:
#   1. namespaced mode (watchNamespaces set) renders a ClusterRole that grants
#      openclawclusterdefaults, and the per-namespace Roles do NOT carry it;
#   2. cluster-wide mode (no watchNamespaces) keeps openclawclusterdefaults in
#      the manager ClusterRole.

set -euo pipefail

CHART="charts/openclaw-operator"
RESOURCE="openclawclusterdefaults"

if ! command -v helm >/dev/null 2>&1; then
  echo "::error::helm is required to run this check"
  exit 1
fi

fail() {
  echo "::error::$1"
  exit 1
}

# Emit "<kind>|<name>" for every Role/ClusterRole document whose rules mention
# $RESOURCE. Splits the rendered manifest into YAML documents and uses a plain
# substring check, so it is robust to flow- vs block-style rule arrays and quoting.
roles_granting_resource() {
  awk -v res="$RESOURCE" '
    BEGIN { RS = "\n---\n" }
    {
      kind = ""; name = ""
      n = split($0, lines, "\n")
      for (i = 1; i <= n; i++) {
        if (lines[i] ~ /^kind:/)                       { sub(/^kind:[ ]*/, "", lines[i]); kind = lines[i] }
        else if (lines[i] ~ /^  name:/ && name == "")  { sub(/^  name:[ ]*/, "", lines[i]); name = lines[i] }
      }
      if ((kind == "Role" || kind == "ClusterRole") && index($0, res) > 0)
        print kind "|" name
    }
  '
}

echo "==> Rendering chart in namespaced mode (watchNamespaces={openclaw,team-a})"
NS_RENDER=$(helm template oc "$CHART" --namespace openclaw --set 'watchNamespaces={openclaw,team-a}')
NS_GRANTS=$(echo "$NS_RENDER" | roles_granting_resource)

if ! echo "$NS_GRANTS" | grep -q "^ClusterRole|"; then
  echo "Rendered roles granting $RESOURCE:"
  echo "${NS_GRANTS:-<none>}"
  fail "namespaced mode does not grant $RESOURCE via a ClusterRole (#529)"
fi
if echo "$NS_GRANTS" | grep -q "^Role|"; then
  echo "$NS_GRANTS"
  fail "namespaced mode grants $RESOURCE via a namespaced Role, which is ineffective for a cluster-scoped resource (#529)"
fi
echo "    OK: ClusterRole grants $RESOURCE; no namespaced Role does."

echo "==> Rendering chart in cluster-wide mode (no watchNamespaces)"
CW_RENDER=$(helm template oc "$CHART" --namespace openclaw)
CW_GRANTS=$(echo "$CW_RENDER" | roles_granting_resource)

if ! echo "$CW_GRANTS" | grep -q "^ClusterRole|"; then
  echo "Rendered roles granting $RESOURCE:"
  echo "${CW_GRANTS:-<none>}"
  fail "cluster-wide mode does not grant $RESOURCE via a ClusterRole"
fi
echo "    OK: ClusterRole grants $RESOURCE."

echo "Helm chart grants cluster-scoped $RESOURCE access in both watch modes."
