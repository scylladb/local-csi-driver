#!/bin/bash
#
# Copyright (C) 2021 ScyllaDB
#
# This script deploys disk-setup and csi-driver.
# Usage: ${0} <driver_image_ref>

set -euxEo pipefail

function wait-for-object-creation {
    for i in {1..30}; do
        { kubectl -n "${1}" get "${2}" && break; } || sleep 1
    done
}

if [[ -z ${1+x} ]]; then
    echo "Missing driver image ref.\nUsage: ${0} <driver_image_ref>" >&2 >/dev/null
    exit 1
fi

function kubectl_create {
    if [[ -z ${REENTRANT+x} ]]; then
        # In an actual CI run we have to enforce that no two objects have the same name.
        kubectl create "$@"
    else
        # For development iterations we want to update the objects.
        kubectl apply "$@"
    fi
}

ARTIFACTS_DIR=${ARTIFACTS_DIR:-$( mktemp -d )}
DRIVER_IMAGE_REF=${1}

deploy_dir=${ARTIFACTS_DIR}/deploy/kubernetes
mkdir -p "${deploy_dir}/"{kubernetes,disk-setup}

cp ./deploy/kubernetes/*.yaml "${deploy_dir}/kubernetes"
cp ./example/disk-setup/*.yaml "${deploy_dir}/disk-setup"
cp ./example/storageclass_xfs.yaml "${deploy_dir}/"

for f in $( find "${deploy_dir}"/ -type f -name '*.yaml' ); do
    sed -i -E -e "s~docker.io/scylladb/k8s-local-volume-provisioner(:|@sha256:)[^ ]*~${DRIVER_IMAGE_REF}~" "${f}"
done

kubectl_create -f "${deploy_dir}"/storageclass_xfs.yaml
wait-for-object-creation default storageclass/local-xfs

kubectl_create -f "${deploy_dir}"/disk-setup
kubectl -n xfs-disk-setup rollout status --timeout=5m daemonset.apps/xfs-disk-setup

kubectl_create -f "${deploy_dir}"/kubernetes
kubectl -n local-csi-driver rollout status --timeout=5m daemonset.apps/local-csi-driver
