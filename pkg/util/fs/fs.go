// Copyright (c) 2023 ScyllaDB.

package fs

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// XFS magic number from statfs
const xfsMagic = 0x58465342

func GetFilesystem(path string) (string, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(path, &stat)
	if err != nil {
		return "", fmt.Errorf("can't statfs %q: %w", path, err)
	}

	switch stat.Type {
	case xfsMagic:
		return "xfs", nil
	default:
		return fmt.Sprintf("0x%x", stat.Type), nil
	}
}
