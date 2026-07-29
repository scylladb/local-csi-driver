#!/bin/bash
#
# Copyright (C) 2024 ScyllaDB
#

set -euExo pipefail
shopt -s inherit_errexit

FIELD_MANAGER="${FIELD_MANAGER:-so-default}"

function kubectl_apply {
  kubectl apply --server-side=true --field-manager="${FIELD_MANAGER}" --force-conflicts "$@"
}

function kubectl_create {
    if [[ -z "${REENTRANT+x}" ]]; then
        # In an actual CI run we have to enforce that no two objects have the same name.
        kubectl create --field-manager="${FIELD_MANAGER}" "$@"
    else
        # For development iterations we want to update the objects.
        kubectl_apply "$@"
    fi
}

function wait-for-object-creation {
    for i in {1..30}; do
        { kubectl -n "${1}" get "${2}" && break; } || sleep 1
    done
}

# Waits until the provisioner published CSIStorageCapacity for a storage class.
# Capacity is published on a poll interval, so right after the driver starts there
# is a window in which no capacity exists yet. Tests that create a storage class
# and expect its capacity shortly after would race with it.
# $1 - namespace the driver publishes capacity in
# $2 - storage class name
function wait-for-storage-capacity {
    local capacities
    for i in {1..60}; do
        capacities="$( kubectl -n="${1}" get csistoragecapacities.storage.k8s.io -o=go-template='{{ range .items }}{{ if eq .storageClassName "'"${2}"'" }}{{ .metadata.name }}{{ end }}{{ end }}' || true )"
        if [[ -n "${capacities}" ]]; then
            return 0
        fi
        sleep 2
    done

    echo "timed out waiting for CSIStorageCapacity of storage class ${2} in namespace ${1}" > /dev/stderr
    kubectl -n="${1}" get csistoragecapacities.storage.k8s.io > /dev/stderr
    return 1
}

# $1 - namespace
# $2 - pod name
# $3 - container name
function wait-for-container-exit-with-logs {
  exit_code=""
  while [[ "${exit_code}" == "" ]]; do
    kubectl -n="${1}" logs -f pod/"${2}" -c="${3}" > /dev/stderr || echo "kubectl logs failed before pod has finished, retrying..." > /dev/stderr
    exit_code="$( kubectl -n="${1}" get pods/"${2}" --template='{{ range .status.containerStatuses }}{{ if and (eq .name "'"${3}"'") (ne .state.terminated.exitCode nil) }}{{ .state.terminated.exitCode }}{{ end }}{{ end }}' )"
  done
  echo -n "${exit_code}"
}
