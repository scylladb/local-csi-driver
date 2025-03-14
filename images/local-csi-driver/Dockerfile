FROM quay.io/scylladb/scylla-operator-images:golang-1.23 AS builder
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]
WORKDIR /go/src/github.com/scylladb/local-csi-driver
COPY . .
RUN make build --warn-undefined-variables GO_BUILD_PACKAGES=./cmd/local-csi-driver

FROM quay.io/scylladb/scylla-operator-images:base-ubuntu-22.04
SHELL ["/bin/bash", "-euEo", "pipefail", "-O", "inherit_errexit", "-c"]

RUN apt-get update && \
    apt-get install -y --no-install-recommends xfsprogs && \
    apt-get clean  && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /go/src/github.com/scylladb/local-csi-driver/local-csi-driver /usr/bin/
ENTRYPOINT ["/usr/bin/local-csi-driver"]
