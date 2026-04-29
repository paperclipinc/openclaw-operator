#!/usr/bin/env bash
# hack/check-helm-rbac-sync.sh
#
# Verifies RBAC from kubebuilder markers (config/rbac/role.yaml) against the
# Helm chart by rendering templates with helm:
#   - Cluster mode (rbac.namespaced=false): manager ClusterRole is a superset
#     of all triples from role.yaml.
#   - Namespaced mode: per-namespace Role contains all namespaced triples
#     except openclawclusterdefaults; the clusterdefaults ClusterRole contains
#     only those triples.

set -euo pipefail

GENERATED="config/rbac/role.yaml"
CHART="charts/openclaw-operator"
HELM_TEMPLATE="${CHART}/templates/rbac.yaml"

if [ ! -f "$GENERATED" ]; then
  echo "::error::Generated RBAC not found at $GENERATED — run 'make manifests' first"
  exit 1
fi
if [ ! -f "$HELM_TEMPLATE" ]; then
  echo "::error::Helm chart RBAC template not found at $HELM_TEMPLATE"
  exit 1
fi
if ! command -v helm >/dev/null 2>&1; then
  echo "::error::helm is required for RBAC sync check"
  exit 1
fi

# Parse kubebuilder-generated role.yaml into sorted "apiGroup|resource|verb" triples.
parse_generated() {
  awk '
    /^- apiGroups:/ {
      for (r = 0; r < nres; r++)
        for (v = 0; v < nvrb; v++)
          print grp "|" res[r] "|" vrb[v]
      delete res; delete vrb; grp = ""; nres = 0; nvrb = 0
      section = "groups"
      next
    }
    /^  resources:/ { section = "resources"; next }
    /^  verbs:/     { section = "verbs";     next }

    section == "groups" && /^  - / {
      s = $0; sub(/^  - /, "", s); gsub(/"/, "", s); grp = s
    }
    section == "resources" && /^  - / {
      s = $0; sub(/^  - /, "", s); gsub(/"/, "", s); res[nres++] = s
    }
    section == "verbs" && /^  - / {
      s = $0; sub(/^  - /, "", s); gsub(/"/, "", s); vrb[nvrb++] = s
    }

    END {
      for (r = 0; r < nres; r++)
        for (v = 0; v < nvrb; v++)
          print grp "|" res[r] "|" vrb[v]
    }
  ' "$GENERATED" | sort -u
}

# Parse rendered Helm RBAC (inline apiGroups/resources/verbs arrays) from stdin.
parse_rendered_inline() {
  awk '
    /\{\{/ { next }
    /^\s*#/ { next }

    /apiGroups:/ {
      s = $0; sub(/.*\[/, "", s); sub(/\].*/, "", s)
      ngroups = split(s, arr, ",")
      for (i = 1; i <= ngroups; i++) {
        g = arr[i]; gsub(/[ "'\''"]/, "", g); groups[i] = g
      }
      next
    }
    /resources:/ {
      s = $0; sub(/.*\[/, "", s); sub(/\].*/, "", s)
      nresources = split(s, arr, ",")
      for (i = 1; i <= nresources; i++) {
        r = arr[i]; gsub(/[ "'\''"]/, "", r); resources[i] = r
      }
      next
    }
    /verbs:/ {
      s = $0; sub(/.*\[/, "", s); sub(/\].*/, "", s)
      nverbs = split(s, arr, ",")
      for (i = 1; i <= nverbs; i++) {
        v = arr[i]; gsub(/[ "'\''"]/, "", v); verbs[i] = v
      }
      for (g = 1; g <= ngroups; g++)
        for (r = 1; r <= nresources; r++)
          for (v = 1; v <= nverbs; v++)
            print groups[g] "|" resources[r] "|" verbs[v]
    }
  ' | sort -u
}

extract_clusterrole_doc() {
  local pattern="$1"
  shift
  helm template helm-rbac-sync "$CHART" \
    --namespace helm-rbac-sync-ns \
    "$@" \
    --show-only templates/rbac.yaml 2>/dev/null \
    | awk -v pat="$pattern" '
      BEGIN { RS="---"; ORS="" }
      $0 ~ /kind: ClusterRole/ && $0 ~ pat { print $0; exit }
    '
}

extract_role_doc() {
  local ns="$1"
  helm template helm-rbac-sync "$CHART" \
    --namespace helm-rbac-sync-ns \
    --set rbac.namespaced=true \
    --set-json "rbac.watchNamespaces=[\"$ns\"]" \
    --show-only templates/rbac.yaml 2>/dev/null \
    | awk -v ns="$ns" '
      BEGIN { RS="---"; ORS="" }
      $0 ~ /(^|\n)kind: Role(\n|$)/ && $0 ~ ("namespace: " ns) && $0 ~ /manager-role/ && $0 !~ /leader-election/ { print $0; exit }
    '
}

GENERATED_TRIPLES=$(parse_generated)

# --- Cluster mode (default Helm values) ---
CLUSTER_CR_DOC=$(extract_clusterrole_doc "manager-role")
CLUSTER_TRIPLES=$(echo "$CLUSTER_CR_DOC" | parse_rendered_inline)
MISSING=$(comm -23 <(echo "$GENERATED_TRIPLES") <(echo "$CLUSTER_TRIPLES"))

if [ -n "$MISSING" ]; then
  echo "::error::Helm cluster-mode ClusterRole is missing permissions from kubebuilder markers."
  echo ""
  echo "The following triples are in config/rbac/role.yaml but NOT in the rendered manager ClusterRole:"
  echo ""
  echo "$MISSING" | while IFS='|' read -r g r v; do
    group="$g"
    if [ -z "$group" ]; then group='""'; fi
    echo "  apiGroup=$group  resource=$r  verb=$v"
  done
  exit 1
fi

echo "Helm cluster-mode manager ClusterRole is in sync with kubebuilder markers."

# --- Namespaced mode ---
NS_DUMMY="helm-rbac-sync-workloads"
CD_DOC=$(extract_clusterrole_doc "clusterdefaults-reader" --set rbac.namespaced=true \
  --set-json "rbac.watchNamespaces=[\"$NS_DUMMY\"]")
CD_TRIPLES=$(echo "$CD_DOC" | parse_rendered_inline)
EXPECTED_CD=$(echo "$GENERATED_TRIPLES" | grep '|openclawclusterdefaults|' || true)

MISSING_CD=$(comm -23 <(echo "$EXPECTED_CD") <(echo "$CD_TRIPLES"))
if [ -n "$MISSING_CD" ]; then
  echo "::error::Helm namespaced clusterdefaults ClusterRole is missing openclawclusterdefaults triples."
  echo "$MISSING_CD"
  exit 1
fi

EXTRA_CD=$(comm -13 <(echo "$EXPECTED_CD") <(echo "$CD_TRIPLES"))
if [ -n "$EXTRA_CD" ]; then
  echo "::error::Helm clusterdefaults ClusterRole must only grant openclawclusterdefaults (unexpected triples)."
  echo "$EXTRA_CD"
  exit 1
fi

ROLE_DOC=$(extract_role_doc "$NS_DUMMY")
ROLE_TRIPLES=$(echo "$ROLE_DOC" | parse_rendered_inline)
EXPECTED_NS=$(echo "$GENERATED_TRIPLES" | grep -v '|openclawclusterdefaults|' || true)

MISSING_NS=$(comm -23 <(echo "$EXPECTED_NS") <(echo "$ROLE_TRIPLES"))
if [ -n "$MISSING_NS" ]; then
  echo "::error::Helm namespaced Role is missing permissions (excluding clusterdefaults)."
  echo "$MISSING_NS"
  exit 1
fi

echo "Helm namespaced Role and clusterdefaults ClusterRole match kubebuilder markers."
