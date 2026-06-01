#!/usr/bin/env bash
# sync-bundle-crds.sh - Sync CRDs from config/crd/bases/ into the OLM bundle.
#
# OLM installs every CRD manifest present in bundle/manifests/. If a CRD is
# added under config/crd/bases/ but not copied here, OperatorHub installs a
# stale subset and `kubectl apply` of the missing kinds fails with
# "no matches for kind". Keep them in lockstep with config/crd/bases.
#
# The CSV (clusterserviceversion.yaml) is hand-maintained; this script only
# syncs the raw CRD YAML, so remember to add new CRDs to the CSV's
# spec.customresourcedefinitions.owned list as well.
#
# Usage:
#   bash hack/sync-bundle-crds.sh          # copy CRDs into the bundle
#   bash hack/sync-bundle-crds.sh --check  # verify they are in sync (CI mode)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CRD_SRC="${REPO_ROOT}/config/crd/bases"
CRD_DST="${REPO_ROOT}/bundle/manifests"

CHECK_MODE=false
if [[ "${1:-}" == "--check" ]]; then
    CHECK_MODE=true
fi

mkdir -p "$CRD_DST"

status=0
for crd_file in "$CRD_SRC"/*.yaml; do
    name=$(basename "$crd_file")
    if $CHECK_MODE; then
        if ! diff -q "$crd_file" "$CRD_DST/$name" >/dev/null 2>&1; then
            echo "::error::OLM bundle CRD is out of sync: $name"
            echo "Run 'make sync-bundle-crds' and commit the result."
            diff -u "$CRD_DST/$name" "$crd_file" 2>/dev/null || true
            status=1
        fi
    else
        cp "$crd_file" "$CRD_DST/$name"
        echo "Synced: bundle/manifests/$name"
    fi
done

if $CHECK_MODE && [[ $status -eq 0 ]]; then
    echo "OLM bundle CRDs are in sync."
fi
exit $status
