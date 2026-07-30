# Table of Contents

- [Before 1.0.0](#versions-released-before-100)

## Unreleased

<!-- Group entries under: Highlights, Upgrade requirements, Deprecations, Features & Enhancements,
     Bug fixes, Other changes, Dependencies. Omit the sections that don't apply. -->

### Highlights

- Local Volume Provisioner is no longer experimental. The experimental notice has been removed from the project.
  [#120](https://github.com/scylladb/local-csi-driver/pull/120)

### Upgrade requirements

- The provisioner's `ClusterRole` now requires the `patch` verb on `persistentvolumes`, needed by the
  `HonorPVReclaimPolicy` feature that `csi-provisioner` v5 enables unconditionally. Applying the manifests shipped in
  `deploy/kubernetes` grants it automatically; if you manage the provisioner's RBAC yourself, grant it before
  upgrading, or provisioning will fail with a permission error.
  [#127](https://github.com/scylladb/local-csi-driver/pull/127)

### Bug fixes

- Fixed volume publishing failing when the parent directories of the target path did not already exist on the node,
  which could leave pods waiting indefinitely for their volumes to be mounted.
  [#111](https://github.com/scylladb/local-csi-driver/pull/111)

### Dependencies

- The CSI sidecars were updated: `csi-node-driver-registrar` from `v2.6.3` to `v2.17.0`, `livenessprobe` from `v2.8.0`
  to `v2.19.0` and `csi-provisioner` from `v3.3.0` to `v5.3.0`.
  [#127](https://github.com/scylladb/local-csi-driver/pull/127)
- Kubernetes dependencies were updated to 1.36.
  [#124](https://github.com/scylladb/local-csi-driver/pull/124)
- Go was updated to 1.26.
  [#122](https://github.com/scylladb/local-csi-driver/pull/122)
- The base image was updated to UBI 9.7.
  [#118](https://github.com/scylladb/local-csi-driver/pull/118)
- Kubernetes staging module versions are now resolved to exact versions, so builds no longer pick up floating patch
  releases.
  [#115](https://github.com/scylladb/local-csi-driver/pull/115)

## Versions released before 1.0.0

For versions released before 1.0.0, the changelog information can be found in
[GitHub Releases](https://github.com/scylladb/local-csi-driver/releases) as a list of pull requests grouped by category
that went into a release.
