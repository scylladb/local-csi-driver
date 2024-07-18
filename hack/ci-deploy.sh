#!/bin/bash
#
# Copyright (C) 2021 ScyllaDB
#
# This script deploys disk-setup and csi-driver.
# Usage: ${0} <driver_image_ref>

set -euxEo pipefail
shopt -s inherit_errexit

source "$( dirname "${BASH_SOURCE[0]}" )/lib/kube.sh"

if [[ -z ${1+x} ]]; then
    echo "Missing driver image ref.\nUsage: ${0} <driver_image_ref>" >&2 >/dev/null
    exit 1
fi

ARTIFACTS_DIR=${ARTIFACTS_DIR:-$( mktemp -d )}
DRIVER_IMAGE_REF=${1}

deploy_dir=${ARTIFACTS_DIR}/deploy/kubernetes
mkdir -p "${deploy_dir}/"{local-csi-driver,disk-setup}

cp ./deploy/kubernetes/local-csi-driver/*.yaml "${deploy_dir}/local-csi-driver"
cp ./example/disk-setup/*.yaml "${deploy_dir}/disk-setup"
cp ./example/storageclass_xfs.yaml "${deploy_dir}/"

for f in $( find "${deploy_dir}"/ -type f -name '*.yaml' ); do
    sed -i -E -e "s~docker.io/scylladb/local-csi-driver(:|@sha256:)[^ ]*~${DRIVER_IMAGE_REF}~" "${f}"
done

kubectl_create -f "${deploy_dir}"/storageclass_xfs.yaml
wait-for-object-creation default storageclass/scylladb-local-xfs

kubectl_create -f "${deploy_dir}"/disk-setup
kubectl -n xfs-disk-setup rollout status --timeout=5m daemonset.apps/xfs-disk-setup

kubectl_create -f "${deploy_dir}"/local-csi-driver
kubectl -n local-csi-driver rollout status --timeout=5m daemonset.apps/local-csi-driver
