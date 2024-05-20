// Copyright (c) 2023 ScyllaDB.

package volume

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/limit"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/util/slices"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
)

type VolumeStatistics struct {
	AvailableBytes  int64
	TotalBytes      int64
	UsedBytes       int64
	AvailableInodes int64
	TotalInodes     int64
	UsedInodes      int64
}

type VolumeManager struct {
	volumesDir string
	mounter    mount.Interface
	state      *StateManager
	limiter    limit.Limiter
}

type VolumeManagerOption func(v *VolumeManager)

func WithLimiter(limiter limit.Limiter) func(*VolumeManager) {
	return func(v *VolumeManager) {
		v.limiter = limiter
	}
}

func WithMounter(mounter mount.Interface) func(*VolumeManager) {
	return func(v *VolumeManager) {
		v.mounter = mounter
	}
}

func NewVolumeManager(volumesDir string, sm *StateManager, options ...VolumeManagerOption) (*VolumeManager, error) {
	v := &VolumeManager{
		volumesDir: volumesDir,

		mounter: mount.New(""),
		state:   sm,
		limiter: &limit.NoopLimiter{},
	}

	for _, option := range options {
		option(v)
	}

	return v, nil
}

func (v *VolumeManager) CreateVolume(volID, name string, capacity int64, volAccessType AccessType) error {
	availableCapacity, err := v.GetAvailableCapacity()
	if err != nil {
		return fmt.Errorf("requested volume capacity of %dB exceedes available one (%dB)", capacity, availableCapacity)
	}

	path := v.getVolumePath(volID)

	if !slices.Contains(v.SupportedAccessTypes(), volAccessType) {
		return fmt.Errorf("unsupported access type %v", volAccessType)
	}

	klog.V(2).InfoS("Creating volume directory", "path", path)
	err = os.Mkdir(path, 0770)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("can't create volume directory at %q: %w", path, err)
	}

	limitID, err := v.limiter.NewLimit(path)
	if err != nil {
		errs := []error{
			fmt.Errorf("can't init new limit: %w", err),
		}

		rmErr := os.Remove(path)
		if rmErr != nil {
			errs = append(errs, fmt.Errorf("can't remove volume directory: %w", rmErr))
		}

		return errors.NewAggregate(errs)
	}

	klog.V(2).InfoS("New limit initialized", "limitID", limitID, "path", path)

	volumeState := &VolumeState{
		Name:    name,
		ID:      volID,
		LimitID: limitID,
		Size:    capacity,
	}

	err = v.state.SaveVolumeState(volumeState)
	if err != nil {
		errs := []error{
			fmt.Errorf("failed to save volume state: %w", err),
		}

		removeDirErr := os.Remove(path)
		if removeDirErr != nil {
			errs = append(errs, fmt.Errorf("failed to remove volume directory: %w", removeDirErr))
		}

		removeLimitErr := v.limiter.RemoveLimit(limitID)
		if removeLimitErr != nil {
			errs = append(errs, fmt.Errorf("failed to remove volume limit: %w", removeLimitErr))
		}

		return errors.NewAggregate(errs)
	}

	err = v.limiter.SetLimit(limitID, capacity)
	if err != nil {
		errs := []error{
			fmt.Errorf("failed to save volume state: %w", err),
		}

		removeDirErr := os.Remove(path)
		if removeDirErr != nil {
			errs = append(errs, fmt.Errorf("failed to remove volume directory: %w", removeDirErr))
		}

		removeLimitErr := v.limiter.RemoveLimit(limitID)
		if removeLimitErr != nil {
			errs = append(errs, fmt.Errorf("failed to remove volume limit: %w", removeLimitErr))
		}

		removeStateFileErr := v.state.DeleteVolumeState(volID)
		if removeStateFileErr != nil {
			errs = append(errs, fmt.Errorf("failed to remove volume state file: %w", removeStateFileErr))
		}

		return errors.NewAggregate(errs)
	}

	return nil
}

func (v *VolumeManager) DeleteVolume(volID string) error {
	vs := v.state.GetVolumeStateByID(volID)

	path := v.getVolumePath(volID)
	err := os.RemoveAll(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("can't delete mount of volume %q at %q: %w", volID, path, err)
	}
	if err == nil {
		klog.V(2).InfoS("Removed volume directory", "volume", volID, "path", path)
	}

	if vs != nil {
		err = v.limiter.RemoveLimit(vs.LimitID)
		if err != nil {
			return fmt.Errorf("can't delete state of volume %q: %w", volID, err)
		}
		klog.V(2).InfoS("Removed limit", "limitID", vs.LimitID)
	}

	err = v.state.DeleteVolumeState(volID)
	if err != nil {
		return fmt.Errorf("can't delete state of volume %q: %w", volID, err)
	}
	klog.V(2).InfoS("Removed volume state file", "volume", volID)

	return nil
}

func (v *VolumeManager) GetAvailableCapacity() (int64, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(v.volumesDir, &stat)
	if err != nil {
		return 0, fmt.Errorf("can't check statfs of %q: %w", v.volumesDir, err)
	}

	// Reserve space for 1 more volume metadata to return max allocatable space.
	metadataSize := (len(v.state.GetVolumes()) + 1) * MetadataFileMaxSize
	capacity := stat.Bsize*int64(stat.Blocks) - v.state.GetTotalVolumesSize() - int64(metadataSize)

	return capacity, nil
}

func (v *VolumeManager) GetVolumeStatistics(volumePath string) (*VolumeStatistics, error) {
	statfs := &unix.Statfs_t{}
	err := unix.Statfs(volumePath, statfs)
	if err != nil {
		err = fmt.Errorf("can't get statfs on path %q: %w", volumePath, err)
		return nil, err
	}

	return &VolumeStatistics{
		AvailableBytes:  int64(statfs.Bavail) * statfs.Bsize,
		TotalBytes:      int64(statfs.Blocks) * statfs.Bsize,
		UsedBytes:       (int64(statfs.Blocks) - int64(statfs.Bfree)) * statfs.Bsize,
		AvailableInodes: int64(statfs.Ffree),
		TotalInodes:     int64(statfs.Files),
		UsedInodes:      int64(statfs.Files) - int64(statfs.Ffree),
	}, nil
}

func (v *VolumeManager) Mount(volumeID, targetPath, fsType string, mountOptions []string) error {
	path := v.getVolumePath(volumeID)

	err := os.Mkdir(targetPath, 0770)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("can't create target path at %q: %w", targetPath, err)
	}

	klog.V(2).InfoS("Mounting volume directory", "path", path, "targetPath", targetPath)
	err = v.mounter.Mount(path, targetPath, fsType, mountOptions)
	if err != nil {
		return fmt.Errorf("can't mount device %q at %q: %w", path, targetPath, err)
	}

	return nil
}

func (v *VolumeManager) Unmount(targetPath string) error {
	err := v.mounter.Unmount(targetPath)
	if err != nil {
		return fmt.Errorf("failed to unmount target path at %q: %w", targetPath, err)
	}

	err = os.Remove(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove target path at %q: %w", targetPath, err)
	}

	return nil
}

func (v *VolumeManager) SupportedAccessTypes() []AccessType {
	return []AccessType{MountAccess}
}

func (v *VolumeManager) SupportedFilesystems() []string {
	return []string{"", "xfs"}
}

func (v *VolumeManager) GetVolumeStateByID(id string) *VolumeState {
	return v.state.GetVolumeStateByID(id)
}

func (v *VolumeManager) GetVolumeStateByName(name string) *VolumeState {
	return v.state.GetVolumeStateByName(name)
}

func (v *VolumeManager) getVolumePath(volID string) string {
	return filepath.Join(v.volumesDir, volID)
}
