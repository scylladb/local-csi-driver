#!/bin/bash
#
# Copyright (C) 2024 ScyllaDB
#

set -euExo pipefail
shopt -s inherit_errexit

source "$( dirname "${BASH_SOURCE[0]}" )/../../lib/kube.sh"

SO_IMAGE="${SO_IMAGE:-quay.io/scylladb/scylla-operator:latest}"

# gather-artifacts is a self sufficient function that collects artifacts without depending on any external objects.
# $1- target directory
function gather-artifacts {
  if [ -z "${1+x}" ]; then
    echo -e "Missing target directory.\nUsage: ${FUNCNAME[0]} target_directory" > /dev/stderr
    exit 2
  fi

  if [ -z "${SO_IMAGE+x}" ]; then
    echo "SO_IMAGE can't be empty" > /dev/stderr
    exit 2
  fi

  kubectl create namespace gather-artifacts --dry-run=client -o=yaml | kubectl_apply -f=-
  kubectl create clusterrolebinding gather-artifacts --clusterrole=cluster-admin --serviceaccount=gather-artifacts:default --dry-run=client -o=yaml | kubectl_apply -f=-
  kubectl create -n=gather-artifacts pdb must-gather --selector='app=must-gather' --max-unavailable=0 --dry-run=client -o=yaml | kubectl_apply -f=-

  kubectl_create -n=gather-artifacts -f=- <<EOF
apiVersion: v1
kind: Pod
metadata:
  labels:
    app: must-gather
  name: must-gather
spec:
  restartPolicy: Never
  containers:
  - name: wait-for-artifacts
    command:
    - /usr/bin/sleep
    - infinity
    image: "${SO_IMAGE}"
    imagePullPolicy: Always
    volumeMounts:
    - name: artifacts
      mountPath: /tmp/artifacts
  - name: must-gather
    args:
    - must-gather
    - --all-resources
    - --loglevel=2
    - --dest-dir=/tmp/artifacts
    image: "${SO_IMAGE}"
    imagePullPolicy: Always
    volumeMounts:
    - name: artifacts
      mountPath: /tmp/artifacts
  volumes:
  - name: artifacts
    emptyDir: {}
EOF
  kubectl -n=gather-artifacts wait --for=condition=Ready pod/must-gather

  exit_code="$( wait-for-container-exit-with-logs gather-artifacts must-gather must-gather )"

  kubectl -n=gather-artifacts cp --retries=42 -c=wait-for-artifacts must-gather:/tmp/artifacts "${1}"
  ls -l "${1}"

  kubectl -n=gather-artifacts delete pod/must-gather --wait=false

  if [[ "${exit_code}" -ne "0" ]]; then
    echo "Collecting artifacts using must-gather failed"
    exit "${exit_code}"
  fi
}

function gather-artifacts-on-exit {
  gather-artifacts "${ARTIFACTS}/must-gather"
}
