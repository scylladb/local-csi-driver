#!/usr/bin/env bash
#
# Copyright (C) 2023 ScyllaDB
#

set -euExo pipefail
shopt -s inherit_errexit

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

function kubectl_create {
    if [[ "${REENTRANT}" != "true" ]]; then
        # In an actual CI run we have to enforce that no two objects have the same name.
        kubectl create --field-manager="${field_manager}" "$@"
    else
        # For development iterations we want to update the objects.
        kubectl apply --server-side=true --field-manager="${field_manager}" --force-conflicts "$@"
    fi
}

function gather-artifacts {
  kubectl -n e2e run --restart=Never --image="quay.io/scylladb/scylla-operator:latest" --labels='app=must-gather' --command=true must-gather -- bash -euExo pipefail -O inherit_errexit -c "function wait-for-artifacts { touch /tmp/done && until [[ -f '/tmp/exit' ]]; do sleep 1; done } && trap wait-for-artifacts EXIT && mkdir /tmp/artifacts && scylla-operator must-gather --all-resources --loglevel=2 --dest-dir=/tmp/artifacts"
  kubectl -n e2e wait --for=condition=Ready pod/must-gather

  # Setup artifacts transfer when finished and unblock the must-gather pod when done.
  (
    function unblock-must-gather-pod {
      kubectl -n e2e exec pod/must-gather -- bash -euEo pipefail -O inherit_errexit -c "touch /tmp/exit"
    }
    trap unblock-must-gather-pod EXIT

    kubectl -n e2e exec pod/must-gather -- bash -euEo pipefail -O inherit_errexit -c "until [[ -f /tmp/done ]]; do sleep 1; done; ls -l /tmp/artifacts"
    kubectl -n e2e cp --retries=42 must-gather:/tmp/artifacts "${ARTIFACTS}/must-gather"
    ls -l "${ARTIFACTS}"
  ) &
  must_gather_bg_pid=$!

  kubectl -n e2e logs -f pod/must-gather
  exit_code=$( kubectl -n e2e get pods/must-gather --output='jsonpath={.status.containerStatuses[0].state.terminated.exitCode}' )
  kubectl -n e2e delete pod/must-gather --wait=false

  if [[ "${exit_code}" != "0" ]]; then
    echo "Collecting artifacts using must-gather failed"
    exit "${exit_code}"
  fi

  wait "${must_gather_bg_pid}"
}

function handle-exit {
  gather-artifacts || "Error gathering artifacts" > /dev/stderr
}

trap handle-exit EXIT

# Pre-create e2e namespace to be available to artifacts collection if something we to go wrong while deploying the stack.
kubectl create namespace e2e --dry-run=client -o yaml | kubectl_create -f -
kubectl create clusterrolebinding e2e --clusterrole=cluster-admin --serviceaccount=e2e:default --dry-run=client -o yaml | kubectl_create -f -

timeout -v 10m ./hack/ci-deploy.sh "${E2E_IMAGE}"

kubectl -n=local-csi-driver patch daemonset/local-csi-driver --type=json -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--loglevel=4"}]'
kubectl -n=local-csi-driver rollout status daemonset/local-csi-driver

kubectl create -n e2e pdb my-pdb --selector='app=e2e' --min-available=1 --dry-run=client -o yaml | kubectl_create -f -

kubectl -n e2e run --restart=Never --image="${E2E_TESTS_IMAGE}" --labels='app=e2e' --command=true e2e -- bash -euExo pipefail -O inherit_errexit -c "function wait-for-artifacts { touch /tmp/done && until [[ -f '/tmp/exit' ]]; do sleep 1; done } && trap wait-for-artifacts EXIT && mkdir /tmp/artifacts && local-csi-driver-tests run '${E2E_SUITE}' --loglevel=2 --color=false --artifacts-dir=/tmp/artifacts"
kubectl -n e2e wait --for=condition=Ready pod/e2e

# Setup artifacts transfer when finished and unblock the e2e pod when done.
(
  function unblock-e2e-pod {
    kubectl -n e2e exec pod/e2e -- bash -euEo pipefail -O inherit_errexit -c "touch /tmp/exit"
  }
  trap unblock-e2e-pod EXIT

  kubectl -n e2e exec pod/e2e -- bash -euEo pipefail -O inherit_errexit -c "until [[ -f /tmp/done ]]; do sleep 1; done; ls -l /tmp/artifacts"
  kubectl -n e2e cp --retries=42 e2e:/tmp/artifacts "${ARTIFACTS}"
  ls -l "${ARTIFACTS}"
) &
e2e_bg_pid=$!

kubectl -n e2e logs -f pod/e2e
exit_code=$( kubectl -n e2e get pods/e2e --output='jsonpath={.status.containerStatuses[0].state.terminated.exitCode}' )
kubectl -n e2e delete pod/e2e --wait=false

wait "${e2e_bg_pid}" || ( echo "Collecting e2e artifacts failed" && exit 2 )

if [[ "${exit_code}" != "0" ]]; then
  echo "E2E tests failed"
  exit "${exit_code}"
fi

echo "E2E tests finished successfully"
