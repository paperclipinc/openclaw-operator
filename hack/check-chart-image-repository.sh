#!/usr/bin/env bash
# hack/check-chart-image-repository.sh
#
# Verifies that the Helm chart's default image.repository points at the
# canonical paperclipinc GHCR namespace.
#
# Background: chart releases 0.32.0 to 0.34.4 shipped with a stale
# 'ghcr.io/openclaw-rocks/openclaw-operator' default left over from the
# openclaw-rocks -> paperclipinc org rename. The old namespace returns 403 on
# anonymous pull, so deploying those charts failed silently with
# ImagePullBackOff. See: https://github.com/paperclipinc/openclaw-operator/issues/536
#
# This guard fails the build if the default ever drifts away from the
# canonical namespace again.

set -euo pipefail

VALUES="charts/openclaw-operator/values.yaml"
EXPECTED="ghcr.io/paperclipinc/openclaw-operator"

if [ ! -f "$VALUES" ]; then
  echo "::error::Chart values not found at $VALUES"
  exit 1
fi

# Extract the first 'repository:' under the top-level 'image:' block.
ACTUAL=$(awk '
  /^image:/ { in_image = 1; next }
  in_image && /^[^[:space:]]/ { in_image = 0 }
  in_image && /^[[:space:]]+repository:/ {
    sub(/^[[:space:]]+repository:[[:space:]]*/, "")
    gsub(/["'"'"']/, "")
    print
    exit
  }
' "$VALUES")

if [ -z "$ACTUAL" ]; then
  echo "::error::Could not find image.repository in $VALUES"
  exit 1
fi

if [ "$ACTUAL" != "$EXPECTED" ]; then
  echo "::error::Chart image.repository is '$ACTUAL', expected '$EXPECTED'."
  echo "The chart must default to the canonical paperclipinc GHCR namespace so"
  echo "published charts pull successfully. See issue #536."
  exit 1
fi

echo "Chart image.repository is correct: $ACTUAL"
