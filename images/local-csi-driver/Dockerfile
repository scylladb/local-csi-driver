# renovate: datasource=docker depName=golang packageName=quay.io/scylladb/scylla-operator-images versioning=regex:^golang-(?<major>\d+)\.(?<minor>\d+)$
FROM quay.io/scylladb/scylla-operator-images:golang-1.26 AS builder
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]
WORKDIR /go/src/github.com/scylladb/local-csi-driver
COPY . .
RUN make build --warn-undefined-variables GO_BUILD_PACKAGES=./cmd/local-csi-driver

# renovate: datasource=docker depName=base-ubi-minimal packageName=quay.io/scylladb/scylla-operator-images versioning=regex:^base-ubi-(?<major>\d+)\.(?<minor>\d+)-minimal$
FROM quay.io/scylladb/scylla-operator-images:base-ubi-9.8-minimal
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]

LABEL org.opencontainers.image.title="Local CSI Driver" \
      org.opencontainers.image.description="CSI driver provisioning local volumes for ScyllaDB" \
      org.opencontainers.image.authors="ScyllaDB Operator Team" \
      org.opencontainers.image.source="https://github.com/scylladb/local-csi-driver/" \
      org.opencontainers.image.documentation="https://operator.docs.scylladb.com" \
      org.opencontainers.image.url="https://hub.docker.com/r/scylladb/local-csi-driver" \
      org.opencontainers.image.vendor="ScyllaDB" \
      name="Local CSI Driver" \
      maintainer="ScyllaDB Operator Team" \
      vendor="ScyllaDB" \
      version="see-image-tag" \
      release="see-image-tag" \
      summary="CSI driver provisioning local volumes for ScyllaDB" \
      description="Provisions local volumes backed by directories on a node's local filesystem, with optional XFS quota enforcement."

RUN microdnf install -y --enablerepo=almalinux-base-9 xfsprogs && \
    microdnf clean all && \
    rm -rf /var/cache/dnf/*

COPY --from=builder /go/src/github.com/scylladb/local-csi-driver/local-csi-driver /usr/bin/

COPY /LICENSE /licenses/LICENSE

ENTRYPOINT ["/usr/bin/local-csi-driver"]
