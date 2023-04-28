// Copyright (c) 2023 ScyllaDB.

package fs

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func GetFilesystem(path string) (string, error) {
	cmd := exec.Command("stat", "-f", "-c", "%T", path)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("can't run stat on %q: %w, stdout: %q, stderr: %q", path, err, stdout.String(), stderr.String())
	}

	fsType := strings.TrimSpace(stdout.String())
	if len(fsType) == 0 {
		return "", fmt.Errorf("can't get filesystem information about %q", path)
	}

	return fsType, nil
}
