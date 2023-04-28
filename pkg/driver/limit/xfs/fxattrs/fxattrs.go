// Copyright (c) 2023 ScyllaDB.

package fxattrs

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

type FSXAttrFlags uint32

const (
	FlagRealtime          FSXAttrFlags = 0x00000001
	FlagPreallocated      FSXAttrFlags = 0x00000002
	FlagImmutable         FSXAttrFlags = 0x00000008
	FlagAppend            FSXAttrFlags = 0x00000010
	FlagSync              FSXAttrFlags = 0x00000020
	FlagNoATime           FSXAttrFlags = 0x00000040
	FlagNoDump            FSXAttrFlags = 0x00000080
	FlagRealtimeInherit   FSXAttrFlags = 0x00000100
	FlagProjectInherit    FSXAttrFlags = 0x00000200
	FlagNoSymlinks        FSXAttrFlags = 0x00000400
	FlagExtentSize        FSXAttrFlags = 0x00000800
	FlagExtentSizeInherit FSXAttrFlags = 0x00001000
	FlagNoDefragment      FSXAttrFlags = 0x00002000
	FlagFilestream        FSXAttrFlags = 0x00004000
	FlagDAX               FSXAttrFlags = 0x00008000
	FlagCOWExtentSize     FSXAttrFlags = 0x00010000
	FlagHasAttribute      FSXAttrFlags = 0x80000000
)

const (
	// <uapi/linux/fs.h>

	FS_IOC_FSGETXATTR = 0x801c581f
	FS_IOC_FSSETXATTR = 0x401c5820
)

type FSXAttrs struct {
	Flags         FSXAttrFlags
	ExtentSize    uint32
	ExtentCount   uint32
	ProjectID     uint32
	CoWExtentSize uint32
	_             [8]byte
}

func Get(file *os.File) (*FSXAttrs, error) {
	var attrs FSXAttrs

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, file.Fd(), FS_IOC_FSGETXATTR, uintptr(unsafe.Pointer(&attrs)))
	if errno != 0 {
		return nil, fmt.Errorf("can't get fxattrs of %q: %w", file.Name(), errno)
	}

	return &attrs, nil
}

func Set(file *os.File, attrs *FSXAttrs) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, file.Fd(), FS_IOC_FSSETXATTR, uintptr(unsafe.Pointer(attrs)))
	if errno != 0 {
		return fmt.Errorf("can't set fxattrs on %q: %w", file.Name(), errno)
	}

	return nil
}

func GetProjectID(file *os.File) (uint32, error) {
	fxattrs, err := Get(file)
	if err != nil {
		return 0, fmt.Errorf("can't get project id of %q: %w", file.Name(), err)
	}

	return fxattrs.ProjectID, nil
}

func SetProjectID(file *os.File, projectID uint32) error {
	fxattrs, err := Get(file)
	if err != nil {
		return fmt.Errorf("can't get file attributes of %q: %w", file.Name(), err)
	}

	fxattrs.ProjectID = projectID
	fxattrs.Flags |= FlagProjectInherit

	err = Set(file, fxattrs)
	if err != nil {
		return fmt.Errorf("can't set file attributes on %q: %w", file.Name(), err)
	}

	return nil
}
