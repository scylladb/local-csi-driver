#!/bin/bash
set -euo pipefail

# Script adapted from https://github.com/kubernetes/kubernetes/issues/79384#issuecomment-521493597.
#
# This script pins k8s.io staging module versions to use with k8s.io/kubernetes.
#
# NOTE: This script assumes Kubernetes follows its standard versioning policy where staging modules
# are versioned consistently with the main release (v1.X.Y → v0.X.Y for staging).
# Ref: https://github.com/kubernetes/community/blob/306dc3f8a15f5e6fd9eefe51ebf1865aa13b1ef4/contributors/devel/sig-architecture/staging.md.

VERSION=$(cat go.mod | grep "k8s.io/kubernetes v" | sed "s/^.*v\([0-9.]*\).*/\1/")
echo "Updating k8s.io go.mod replace directives for k8s.io/kubernetes@v$VERSION"

STAGING_VERSION="0.${VERSION#*.}"

MODS=($(
    curl -sS https://raw.githubusercontent.com/kubernetes/kubernetes/v${VERSION}/go.mod |
    sed -n 's|.*k8s.io/\(.*\) => ./staging/src/k8s.io/.*|k8s.io/\1|p'
))

# Apply replace directives for each staging module
for MOD in "${MODS[@]}"; do
    echo "Applying go.mod -replace=${MOD}=${MOD}@v${STAGING_VERSION}"
    go mod edit "-replace=${MOD}=${MOD}@v${STAGING_VERSION}"
done

go get "k8s.io/kubernetes@v${VERSION}"
