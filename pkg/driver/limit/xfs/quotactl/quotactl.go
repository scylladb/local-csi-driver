// Copyright (c) 2023 ScyllaDB.

package quotactl

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
	"k8s.io/mount-utils"
)

type QuotaType uint

const (
	QuotaTypeUser QuotaType = iota
	QuotaTypeGroup
	QuotaTypeProject
)

const (
	// https://github.com/torvalds/linux/blob/master/include/uapi/linux/dqblk_xfs.h

	Q_XQUOTAON = (('X' << 8) + (iota + 1)) << 8
	Q_XQUOTAOFF
	Q_XGETQUOTA
	Q_XSETQLIM
	Q_XGETQSTAT
	Q_XQUOTARM
	Q_XQUOTASYNC
	Q_XGETQSTATV
	Q_XGETNEXTQUOTA
)

const (
	// https://github.com/torvalds/linux/blob/master/include/uapi/linux/dqblk_xfs.h

	FS_DQUOT_VERSION   = 1
	FS_QSTATV_VERSION1 = 1

	FS_DQ_ISOFT      = 1 << 0
	FS_DQ_IHARD      = 1 << 1
	FS_DQ_BSOFT      = 1 << 2
	FS_DQ_BHARD      = 1 << 3
	FS_DQ_RTBSOFT    = 1 << 4
	FS_DQ_RTBHARD    = 1 << 5
	FS_DQ_BTIMER     = 1 << 6
	FS_DQ_ITIMER     = 1 << 7
	FS_DQ_RTBTIMER   = 1 << 8
	FS_DQ_TIMER_MASK = FS_DQ_BTIMER | FS_DQ_ITIMER | FS_DQ_RTBTIMER
	FS_DQ_BWARNS     = 1 << 9
	FS_DQ_IWARNS     = 1 << 10
	FS_DQ_RTBWARNS   = 1 << 11
	FS_DQ_WARNS_MASK = FS_DQ_BWARNS | FS_DQ_IWARNS | FS_DQ_RTBWARNS
	FS_DQ_BCOUNT     = 1 << 12
	FS_DQ_ICOUNT     = 1 << 13
	FS_DQ_RTBCOUNT   = 1 << 14
	FS_DQ_ACCT_MASK  = FS_DQ_BCOUNT | FS_DQ_ICOUNT | FS_DQ_RTBCOUNT
)

type DiskQuota struct {
	Version          int8
	Flags            int8
	FieldMask        uint16
	ID               uint32
	BlkHardLimit     uint64
	BlkSoftLimit     uint64
	InodeHardLimit   uint64
	INodeSoftLimit   uint64
	BlocksCount      uint64
	InodeCount       uint64
	INodeTimer       int32
	BlockTimer       int32
	InodeWarnings    uint16
	BlockWarnings    uint16
	_                int32
	RTBlockHardLimit uint64
	RTBlockSoftLimit uint64
	RTBlocksCount    uint64
	RTBlockTimer     int32
	RTBlockWarnings  uint16
	_                int16
	_                [8]byte
}

var (
	IDNotFoundErr = errors.New("id not found")
)

// GetQuota returns quota information for the provided ID and quota type.
func GetQuota(fsPath string, quotaType QuotaType, id uint32) (*DiskQuota, error) {
	device, err := getMountDevice(fsPath)
	if err != nil {
		return nil, fmt.Errorf("can't get block device backing file %q: %w", fsPath, err)
	}

	quota := DiskQuota{
		Version: FS_DQUOT_VERSION,
	}

	// https://github.com/torvalds/linux/blob/master/include/uapi/linux/dqblk_xfs.h
	cmd := Q_XGETQUOTA | (quotaType & 0x00ff)

	_, _, errno := unix.Syscall6(unix.SYS_QUOTACTL, uintptr(cmd), uintptr(unsafe.Pointer(device)), uintptr(id), uintptr(unsafe.Pointer(&quota)), 0, 0)
	if errno != 0 {
		return nil, transformErrno(errno)
	}

	return &quota, nil
}

// SetQuota sets disk quota limits.
func SetQuota(fsPath string, quotaType QuotaType, dq *DiskQuota) error {
	device, err := getMountDevice(fsPath)
	if err != nil {
		return fmt.Errorf("can't get device of mount point %q: %w", fsPath, err)
	}

	// https://github.com/torvalds/linux/blob/master/include/uapi/linux/dqblk_xfs.h
	cmd := Q_XSETQLIM | (quotaType & 0x00ff)

	_, _, errno := unix.Syscall6(unix.SYS_QUOTACTL, uintptr(cmd), uintptr(unsafe.Pointer(device)), uintptr(dq.ID), uintptr(unsafe.Pointer(dq)), 0, 0)
	if errno != 0 {
		return transformErrno(errno)
	}

	return nil
}

func getMountDevice(mountPoint string) (*byte, error) {
	entries, err := mount.New("").List()
	if err != nil {
		return nil, fmt.Errorf("can't list mount points at %q: %w", mountPoint, err)
	}

	for _, e := range entries {
		if e.Path == mountPoint {
			deviceArg, err := unix.BytePtrFromString(e.Device)
			if err != nil {
				return nil, fmt.Errorf("can't create byte ptr from string %q: %w", e.Device, err)
			}

			return deviceArg, nil
		}
	}

	return nil, fmt.Errorf("cannot find device of mount point %q", mountPoint)
}

func transformErrno(err syscall.Errno) error {
	switch err {
	case 0:
		return nil
	case syscall.ENOENT:
		return IDNotFoundErr
	default:
		return err
	}
}
