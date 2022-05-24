FROM quay.io/scylladb/scylla-operator-images:golang-1.19 AS builder
SHELL ["/bin/bash", "-euEo", "pipefail", "-c"]
WORKDIR /go/src/github.com/scylladb/k8s-local-volume-provisioner
COPY . .
RUN make build --warn-undefined-variables

FROM quay.io/scylladb/scylla-operator-images:node-setup
SHELL ["/bin/bash", "-euEo", "pipefail", "-c"]
COPY --from=builder /go/src/github.com/scylladb/k8s-local-volume-provisioner/local-csi-driver /usr/bin/
COPY --from=builder /go/src/github.com/scylladb/k8s-local-volume-provisioner/local-csi-driver-tests /usr/bin/
ENTRYPOINT ["/usr/bin/local-csi-driver"]
