# renovate: datasource=docker depName=golang packageName=quay.io/scylladb/scylla-operator-images versioning=regex:^golang-(?<major>\d+)\.(?<minor>\d+)$
FROM quay.io/scylladb/scylla-operator-images:golang-1.25 AS builder
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]
WORKDIR /go/src/github.com/scylladb/local-csi-driver
COPY . .
RUN make build --warn-undefined-variables GO_BUILD_PACKAGES="./cmd/local-csi-driver-tests ./vendor/k8s.io/kubernetes/cmd/kubectl"

# renovate: datasource=docker depName=base-ubi-minimal packageName=quay.io/scylladb/scylla-operator-images versioning=regex:^base-ubi-(?<major>\d+)\.(?<minor>\d+)-minimal$
FROM quay.io/scylladb/scylla-operator-images:base-ubi-9.6-minimal
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]

WORKDIR /go/src/github.com/scylladb/local-csi-driver
# This sucks but upstream csi tests need this.
COPY vendor/k8s.io/kubernetes/test/e2e/testing-manifests/storage-csi/. vendor/k8s.io/kubernetes/test/e2e/testing-manifests/storage-csi/

COPY --from=builder /go/src/github.com/scylladb/local-csi-driver/local-csi-driver-tests /usr/bin/
COPY --from=builder /go/src/github.com/scylladb/local-csi-driver/kubectl /usr/bin/
ENTRYPOINT ["/usr/bin/local-csi-driver-tests"]
