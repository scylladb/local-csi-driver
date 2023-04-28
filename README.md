[![GitHub release](https://img.shields.io/github/tag/scylladb/k8s-local-volume-provisioner.svg?label=release)](https://github.com/scylladb/k8s-local-volume-provisioner/releases)
[![Go](https://github.com/scylladb/k8s-local-volume-provisioner/actions/workflows/build-test.yaml/badge.svg?branch=master)](https://github.com/scylladb/k8s-local-volume-provisioner/actions/workflows/build-test.yaml?query=branch%3Amaster)
[![Go Report Card](https://goreportcard.com/badge/github.com/scylladb/k8s-local-volume-provisioner)](https://goreportcard.com/report/github.com/scylladb/k8s-local-volume-provisioner)
[![Language](https://img.shields.io/badge/Language-Go-blue.svg)](https://golang.org/)
[![LICENSE](https://img.shields.io/github/license/scylladb/k8s-local-volume-provisioner.svg)](https://github.com/scylladb/k8s-local-volume-provisioner/blob/master/LICENSE)

## Local Volume Provisioner

**This repository contains an experimental code. ScyllaDB don't provide any support, nor any guarantees about backward compatibility.** 

The Local Volume Provisioner implements the [Container Storage Interface (CSI)](https://github.com/container-storage-interface/spec/blob/master/spec.md),
a specification for container orchestrators to manage the lifecycle of volumes.

## Features
Local Volume Provisioner supports dynamic provisioning on local disks. It allows storage volumes to be created on-demand
by managing directories created on disks attached to instances. On supported filesystems, directories have quota
limitations to ensure volume size limits.

List of features driver supports:
* Dynamic provisioning - Uses a persistent volume claim (PVC) to dynamically provision a persistent volume (PV).
* Persistent volume capacity limiting - Uses FS Quotas to enforce volume capacity limits.
* Storage Capacity Tracking - Container Orchestration scheduler can fetch information about node capacity and prevent 
from scheduling workloads on nodes not satisfying storage capacity constraints.
* Topology - Volumes are constrained to land on the same node where they were originally created. 

The following CSI features are implemented:
* Controller Service
* Node Service
* Identity Service

### Installation

Provisioner requires existing directory created on host where dynamic volumes will be managed.
Currently, quotas are only supported on XFS filesystems. When the volume directory is using an unsupported filesystem, 
volume sizes aren't limited, and users won't receive any IO error when they overflow the volume.

#### Volume directory
  
Users can create volume directories themselves, or use the provided example which creates a 10GB image on every host in 
the k8s cluster, formats it to XFS and mounts it in a particular location.
To deploy a DaemonSet creating it:
```sh
kubectl apply -f example/disk-setup
kubectl -n xfs-disk-setup rollout status daemonset.apps/xfs-disk-setup
```

#### Driver deployment:

HostPath where volume directory is created on each k8s node must be provided to the driver's DaemonSet via `volumes-dir`
volume.

If you want to deploy the driver:
```sh
kubectl apply -f deploy/kubernetes
kubectl -n local-csi-driver rollout status daemonset.apps/local-csi-driver
```

## Development
Please go through [CSI Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) and
[Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs) to get some basic understanding of CSI 
driver before you start.

### Requirements
* Golang 1.18+
* Kubernetes 1.24+

### Testing
To execute all unit tests and e2e test suites run: `make test`

## License
This library is licensed under the Apache 2.0 License.
