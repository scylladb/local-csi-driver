# renovate: datasource=docker depName=golang packageName=quay.io/scylladb/scylla-operator-images versioning=regex:^golang-(?<major>\d+)\.(?<minor>\d+)$
FROM quay.io/scylladb/scylla-operator-images:golang-1.25 AS builder
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]
WORKDIR /go/src/github.com/scylladb/local-csi-driver
COPY . .
RUN make build --warn-undefined-variables GO_BUILD_PACKAGES=./cmd/local-csi-driver

# renovate: datasource=docker depName=base-ubi-minimal packageName=quay.io/scylladb/scylla-operator-images versioning=regex:^base-ubi-(?<major>\d+)\.(?<minor>\d+)-minimal$
FROM quay.io/scylladb/scylla-operator-images:base-ubi-9.6-minimal
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]

RUN microdnf install -y --enablerepo=almalinux-base-9 xfsprogs && \
    microdnf clean all && \
    rm -rf /var/cache/dnf/*

COPY --from=builder /go/src/github.com/scylladb/local-csi-driver/local-csi-driver /usr/bin/
ENTRYPOINT ["/usr/bin/local-csi-driver"]
