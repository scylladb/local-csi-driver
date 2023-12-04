FROM quay.io/scylladb/scylla-operator-images:golang-1.21 AS builder
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]
WORKDIR /go/src/github.com/scylladb/k8s-local-volume-provisioner
COPY . .
RUN make build --warn-undefined-variables GO_BUILD_PACKAGES="./cmd/local-csi-driver-tests ./vendor/k8s.io/kubernetes/cmd/kubectl"

FROM quay.io/scylladb/scylla-operator-images:base-ubuntu-22.04
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]

WORKDIR /go/src/github.com/scylladb/k8s-local-volume-provisioner
# This sucks but upstream csi tests need this.
COPY vendor/k8s.io/kubernetes/test/e2e/testing-manifests/storage-csi/. vendor/k8s.io/kubernetes/test/e2e/testing-manifests/storage-csi/

COPY --from=builder /go/src/github.com/scylladb/k8s-local-volume-provisioner/local-csi-driver-tests /usr/bin/
COPY --from=builder /go/src/github.com/scylladb/k8s-local-volume-provisioner/kubectl /usr/bin/
ENTRYPOINT ["/usr/bin/local-csi-driver-tests"]
