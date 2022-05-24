// Copyright (c) 2023 ScyllaDB.

package xfs

import (
	"fmt"
	"math/rand"
	"os"
	"path"
	"sync"

	"github.com/pkg/errors"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/limit"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/limit/xfs/fxattrs"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/limit/xfs/quotactl"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/volume"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/util/fs"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/util/slices"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
)

type xfsLimiter struct {
	volumesDir string
	mut        sync.Mutex
}

var _ limit.Limiter = &xfsLimiter{}

func NewXFSLimiter(volumesDir string, volumes []volume.VolumeState) (*xfsLimiter, error) {
	volumesDir = path.Clean(volumesDir)

	fsType, err := fs.GetFilesystem(volumesDir)
	if err != nil {
		return nil, fmt.Errorf("can't get volume dir %q filesystem: %w", volumesDir, err)
	}

	if fsType != "xfs" {
		return nil, fmt.Errorf("volumes path %q is not XFS filesystem", volumesDir)
	}

	entry, err := getMountEntry(volumesDir)
	if err != nil {
		return nil, fmt.Errorf("can't get mount entry of %q: %w", volumesDir, err)
	}

	if entry.Type != fsType {
		return nil, fmt.Errorf("expected %q filesystem at %q mount point, got %q", fsType, volumesDir, entry.Type)
	}

	if !slices.Contains(entry.Opts, "pquota") && !slices.Contains(entry.Opts, "prjquota") {
		return nil, fmt.Errorf("xfs path %q was not mounted with pquota nor prjquota - opts: %q", volumesDir, entry.Opts)
	}

	xl := &xfsLimiter{
		volumesDir: volumesDir,
	}

	for _, v := range volumes {
		err = xl.restoreVolumeQuota(v)
		if err != nil {
			return nil, err
		}
	}

	return xl, nil
}

func (xl *xfsLimiter) restoreVolumeQuota(v volume.VolumeState) error {
	volumePath := v.VolumePath(xl.volumesDir)
	vd, err := os.Open(volumePath)
	if err != nil {
		return fmt.Errorf("can't open file %q: %w", volumePath, err)
	}
	defer func() {
		closeErr := vd.Close()
		if closeErr != nil {
			klog.ErrorS(closeErr, "Failed to close volume path", "directory", volumePath)
		}
	}()

	projectID, err := fxattrs.GetProjectID(vd)
	if err != nil {
		return fmt.Errorf("can't determine project ID of %q: %w", volumePath, err)
	}

	if projectID != v.LimitID {
		return fmt.Errorf("found tempered directory %q, expected %d project ID, got %d", volumePath, v.LimitID, projectID)
	}

	err = xl.SetLimit(v.LimitID, v.Size)
	if err != nil {
		return fmt.Errorf("error restoring quota for volume %q: %w", v.ID, err)
	}

	return nil
}

func (xl *xfsLimiter) NewLimit(directory string) (uint32, error) {
	xl.mut.Lock()
	defer xl.mut.Unlock()

	klog.V(4).InfoS("Generating project ID")
	projectID, err := findFreeProjectID(xl.volumesDir)
	if err != nil {
		return 0, fmt.Errorf("can't generate project ID: %w", err)
	}

	v, err := os.Open(directory)
	if err != nil {
		return 0, fmt.Errorf("can't open path %q: %w", directory, err)
	}
	defer func() {
		closeErr := v.Close()
		if closeErr != nil {
			klog.ErrorS(err, "Failed to close volume directory", "directory", directory)
		}
	}()

	err = fxattrs.SetProjectID(v, projectID)
	if err != nil {
		return 0, fmt.Errorf("can't set quota properties on %q directory: %w", directory, err)
	}

	return projectID, nil
}

func (xl *xfsLimiter) SetLimit(projectID uint32, capacityBytes int64) error {
	xl.mut.Lock()
	defer xl.mut.Unlock()

	klog.V(4).InfoS("Setting project", "projectID", projectID, "capacity", capacityBytes)

	fd, err := os.Open(xl.volumesDir)
	if err != nil {
		return fmt.Errorf("can't open path %q: %w", xl.volumesDir, err)
	}
	defer func() {
		closeErr := fd.Close()
		if closeErr != nil {
			klog.ErrorS(err, "Failed to close filesystem directory", "directory", xl.volumesDir)
		}
	}()

	err = quotactl.SetQuota(fd, quotactl.QuotaTypeProject, &quotactl.DiskQuota{
		Version:      quotactl.FS_DQUOT_VERSION,
		ID:           projectID,
		Flags:        int8(quotactl.QuotaTypeProject),
		FieldMask:    quotactl.FS_DQ_BHARD,
		BlkHardLimit: bytesToBlocks(capacityBytes),
	})
	if err != nil {
		return fmt.Errorf("can't set quota on %d projectID: %w", projectID, err)
	}

	return nil
}

func (xl *xfsLimiter) RemoveLimit(limitID uint32) error {
	return xl.SetLimit(limitID, 0)
}

// XFS Quota block units are in BBs (Basic Blocks) of 512 bytes.
func bytesToBlocks(capacity int64) uint64 {
	return uint64(capacity >> 9)
}

func getMountEntry(mountPoint string) (mount.MountPoint, error) {
	entries, err := mount.New("").List()
	if err != nil {
		return mount.MountPoint{}, fmt.Errorf("can't list mount points at %q: %w", mountPoint, err)
	}

	for _, e := range entries {
		if e.Path == mountPoint {
			return e, nil
		}
	}

	return mount.MountPoint{}, fmt.Errorf("mount entry for mountPoint %q not found", mountPoint)
}

func findFreeProjectID(volumesDir string) (uint32, error) {
	fd, err := os.Open(volumesDir)
	if err != nil {
		return 0, fmt.Errorf("can't open path %q for reading: %w", volumesDir, err)
	}
	defer func() {
		closeErr := fd.Close()
		if closeErr != nil {
			klog.ErrorS(err, "Failed to close directory", "directory", volumesDir)
		}
	}()

	const maxRetries = 1000
	for retries := 0; retries < maxRetries; retries++ {
		id := rand.Uint32()
		_, err := quotactl.GetQuota(fd, quotactl.QuotaTypeProject, id)
		if err != nil {
			if errors.Is(err, quotactl.IDNotFoundErr) {
				return id, nil
			}
			return 0, fmt.Errorf("can't get quota for id %d: %w", id, err)
		}
	}

	return 0, fmt.Errorf("unable to generate free project ID with %d retries", maxRetries)
}
