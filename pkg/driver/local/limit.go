package local

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"

	"github.com/scylladb/k8s-local-volume-provisioner/pkg/util/slices"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
)

type Limiter interface {
	GetName() string
	InitLimit(directory string) (uint16, error)
	RemoveLimit(limitID uint16) error
	SetLimit(limitID uint16, capacity int64) error
}

type NoopLimiter struct {
}

func (l *NoopLimiter) GetName() string {
	return "NoOp"
}

func (l *NoopLimiter) InitLimit(directory string) (uint16, error) {
	return 0, nil
}

func (l *NoopLimiter) RemoveLimit(limitID uint16) error {
	return nil
}

func (l *NoopLimiter) SetLimit(limitID uint16, capacity int64) error {
	return nil
}

type xfsLimiter struct {
	fsPath string

	freeProjectIDs    map[uint16]struct{}
	freeProjectIDsMut *sync.Mutex
}

func NewXFSLimiter(volumesDir string, volumes []VolumeState) (*xfsLimiter, error) {
	if _, err := os.Stat(volumesDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("volumes path %s does not exist", volumesDir)
	}

	_, err := exec.LookPath("xfs_quota")
	if err != nil {
		return nil, fmt.Errorf("cannot find xfs_quota cli in PATH")
	}

	freeProjectIDs := make(map[uint16]struct{}, math.MaxUint16)
	for id := uint16(1); id < math.MaxUint16; id++ {
		freeProjectIDs[id] = struct{}{}
	}
	for _, v := range volumes {
		delete(freeProjectIDs, v.LimitID)
	}

	xl := &xfsLimiter{
		fsPath:            volumesDir,
		freeProjectIDs:    freeProjectIDs,
		freeProjectIDsMut: &sync.Mutex{},
	}

	err = xl.restoreQuotas(volumes)
	if err != nil {
		return nil, fmt.Errorf("error restoring quotas: %w", err)
	}

	return xl, nil
}

func isXfs(xfsPath string) (bool, error) {
	klog.V(4).Infof("Checking if path %q is XFS", xfsPath)
	cmd := exec.Command("stat", "-f", "-c", "%T", xfsPath)
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}

	fsType := strings.TrimSpace(string(out))
	klog.V(4).Infof("Path %q filesystem type %q", xfsPath, fsType)

	return fsType == "xfs", nil
}

func getMountEntry(mountpoint, fstype string) (mount.MountPoint, error) {
	entries, err := mount.New(mountpoint).List()
	if err != nil {
		return mount.MountPoint{}, fmt.Errorf("can't list mount points at %q: %w", mountpoint, err)
	}
	for _, e := range entries {
		if e.Path == mountpoint && e.Type == fstype {
			return e, nil
		}
	}
	return mount.MountPoint{}, fmt.Errorf("mount entry for mountpoint %s, fstype %s not found", mountpoint, fstype)
}

func (xl *xfsLimiter) restoreQuotas(volumes []VolumeState) error {
	for _, v := range volumes {
		if err := xl.SetLimit(v.LimitID, v.Size); err != nil {
			return fmt.Errorf("error restoring quota for volume %s: %w", v.ID, err)
		}
	}

	return nil
}

func (xl *xfsLimiter) GetName() string {
	return "XFS"
}

func (xl *xfsLimiter) InitLimit(directory string) (uint16, error) {
	isXfsFS, err := isXfs(xl.fsPath)
	if err != nil {
		return 0, fmt.Errorf("can't check if volumes path %s is an XFS filesystem: %v", xl.fsPath, err)
	}
	if !isXfsFS {
		return 0, fmt.Errorf("volumes path %s is not an XFS filesystem", xl.fsPath)
	}
	klog.V(4).Infof("Verified if path %q is XFS partition: %v", xl.fsPath, isXfsFS)

	entry, err := getMountEntry(path.Clean(xl.fsPath), "xfs")
	if err != nil {
		return 0, err
	}

	if !slices.Contains(entry.Opts, "pquota") && !slices.Contains(entry.Opts, "prjquota") {
		return 0, fmt.Errorf("xfs path %s was not mounted with pquota nor prjquota - opts: %s", xl.fsPath, entry.Opts)
	}

	klog.V(4).Infof("Generating project ID")
	projectID, err := xl.generateID()
	if err != nil {
		return 0, fmt.Errorf("can't generate project ID: %w", err)
	}

	klog.V(4).Infof("Adding project %d", projectID)
	cmd := exec.Command("xfs_quota", "-x", "-c", fmt.Sprintf("project -s -p %s %d", directory, projectID), xl.fsPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		xl.freeProjectIDsMut.Lock()
		defer xl.freeProjectIDsMut.Unlock()
		xl.freeProjectIDs[projectID] = struct{}{}
		return 0, fmt.Errorf("xfs_quota failed with error: %w, output: %s", err, out)
	}

	return projectID, nil
}

func (xl *xfsLimiter) RemoveLimit(projectID uint16) error {
	klog.V(4).Infof("Removing project %d", projectID)

	cmd := exec.Command("xfs_quota", "-x", "-c", fmt.Sprintf("limit -p bhard=0 bhard=0 isoft=0 ihard=0 %d", projectID), xl.fsPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xfs_quota failed with error: %w, output: %s", err, out)
	}

	xl.freeProjectIDsMut.Lock()
	defer xl.freeProjectIDsMut.Unlock()
	xl.freeProjectIDs[projectID] = struct{}{}

	return nil
}

func (xl *xfsLimiter) SetLimit(projectID uint16, capacity int64) error {
	if _, ok := xl.freeProjectIDs[projectID]; ok {
		return fmt.Errorf("project with id %v not found", projectID)
	}

	klog.V(4).Infof("Setting project %d quota %d", projectID, capacity)
	cmd := exec.Command("xfs_quota", "-x", "-c", fmt.Sprintf("limit -p bhard=%d %d", capacity, projectID), xl.fsPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xfs_quota failed with error: %w, output: %s", err, out)
	}

	return nil
}

func (xl *xfsLimiter) generateID() (uint16, error) {
	xl.freeProjectIDsMut.Lock()
	defer xl.freeProjectIDsMut.Unlock()

	for id := range xl.freeProjectIDs {
		delete(xl.freeProjectIDs, id)
		return id, nil
	}

	return 0, fmt.Errorf("project ID pool exhausted")
}
