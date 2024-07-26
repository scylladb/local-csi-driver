#!/usr/bin/env bash
#
# Copyright (C) 2023 ScyllaDB
#

set -euExo pipefail
shopt -s inherit_errexit

if [ -z "${ARTIFACTS+x}" ]; then
  echo "ARTIFACTS can't be empty" > /dev/stderr
  exit 2
fi

source "$( dirname "${BASH_SOURCE[0]}" )/../lib/kube.sh"
source "$( dirname "${BASH_SOURCE[0]}" )/lib/e2e.sh"
parent_dir="$( dirname "${BASH_SOURCE[0]}" )"

trap gather-artifacts-on-exit EXIT

if [ -z ${E2E_SUITE+x} ]; then
  echo "E2E_SUITE can't be empty" > /dev/stderr
  exit 2
fi

if [ -z ${E2E_IMAGE+x} ]; then
  echo "E2E_IMAGE can't be empty" > /dev/stderr
  exit 2
fi

if [ -z ${E2E_TESTS_IMAGE+x} ]; then
  echo "E2E_TESTS_IMAGE can't be empty" > /dev/stderr
  exit 2
fi

if [ -z ${ARTIFACTS+x} ]; then
  echo "ARTIFACTS can't be empty" > /dev/stderr
  exit 2
fi

REENTRANT=${REENTRANT:-false}
export REENTRANT

field_manager=run-e2e-script

timeout -v 10m ./hack/ci-deploy.sh "${E2E_IMAGE}"

kubectl -n=local-csi-driver patch daemonset/local-csi-driver --type=json -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--loglevel=4"}]'
kubectl -n=local-csi-driver rollout status daemonset/local-csi-driver

# Pre-create e2e namespace to be available to artifacts collection if something we to go wrong while deploying the stack.
kubectl create namespace e2e --dry-run=client -o=yaml | kubectl_create -f=-
kubectl create clusterrolebinding e2e --clusterrole=cluster-admin --serviceaccount=e2e:default --dry-run=client -o=yaml | kubectl_create -f=-
kubectl create -n=e2e pdb my-pdb --selector='app=e2e' --min-available=1 --dry-run=client -o=yaml | kubectl_create -f=-

kubectl_create -n=e2e -f=- <<EOF
apiVersion: v1
kind: Pod
metadata:
  labels:
    app: e2e
  name: e2e
spec:
  restartPolicy: Never
  containers:
  - name: wait-for-artifacts
    command:
    - /usr/bin/sleep
    - infinity
    image: "${E2E_TESTS_IMAGE}"
    imagePullPolicy: Always
    volumeMounts:
    - name: artifacts
      mountPath: /tmp/artifacts
  - name: e2e
    command:
    - local-csi-driver-tests
    - run
    - "${E2E_SUITE}"
    - --loglevel=2
    - --color=false
    - --artifacts-dir=/tmp/artifacts
    image: "${E2E_TESTS_IMAGE}"
    imagePullPolicy: Always
    volumeMounts:
    - name: artifacts
      mountPath: /tmp/artifacts
  volumes:
  - name: artifacts
    emptyDir: {}
EOF
kubectl -n=e2e wait --for=condition=Ready pod/e2e

exit_code="$( wait-for-container-exit-with-logs e2e e2e e2e )"

kubectl -n=e2e cp --retries=42 e2e:/tmp/artifacts -c=wait-for-artifacts "${ARTIFACTS}"
ls -l "${ARTIFACTS}"

kubectl -n=e2e delete pod/e2e --wait=false

if [[ "${exit_code}" != "0" ]]; then
  echo "E2E tests failed"
  exit "${exit_code}"
fi

wait
echo "E2E tests finished successfully"
