FROM quay.io/scylladb/scylla-operator-images:golang-1.24 AS builder
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]
WORKDIR /go/src/github.com/scylladb/local-csi-driver
COPY . .
RUN make build --warn-undefined-variables GO_BUILD_PACKAGES=./cmd/local-csi-driver

FROM quay.io/scylladb/scylla-operator-images:base-ubi-9.5-minimal
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]

RUN microdnf install -y --enablerepo=almalinux-base-9 xfsprogs && \
    microdnf clean all && \
    rm -rf /var/cache/dnf/*

COPY --from=builder /go/src/github.com/scylladb/local-csi-driver/local-csi-driver /usr/bin/
ENTRYPOINT ["/usr/bin/local-csi-driver"]
