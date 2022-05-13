# TODO: extract builder and base image into its own repo for reuse and to speed up builds
FROM docker.io/library/ubuntu:20.04 AS builder
SHELL ["/bin/bash", "-euEo", "pipefail", "-c"]
ENV GOPATH=/go \
    GOROOT=/usr/local/go
ENV PATH=$PATH:$GOROOT/bin:$GOPATH/bin
RUN apt-get update; \
    apt-get install -y --no-install-recommends make git curl gzip ca-certificates jq; \
    apt-get clean; \
    curl --fail -L https://storage.googleapis.com/golang/go1.18.linux-amd64.tar.gz | tar -C /usr/local -xzf -
WORKDIR /go/src/github.com/scylladb/xfs-csi-driver
COPY . .
RUN make build --warn-undefined-variables

FROM docker.io/library/ubuntu:20.04
SHELL ["/bin/bash", "-euEo", "pipefail", "-c"]
RUN apt-get update; \
    DEBIAN_FRONTEND=noninteractive TZ=Etc/UTC apt-get install -y --no-install-recommends xfsprogs; \
    apt-get clean;
COPY --from=builder /go/src/github.com/scylladb/xfs-csi-driver/xfs-csi-driver /usr/bin/
ENTRYPOINT ["/usr/bin/xfs-csi-driver"]
