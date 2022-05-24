// Copyright (C) 2021 ScyllaDB

package local

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/util/errors"
)

const (
	volumeStateFileSuffix = ".json"
)

type stateHandler struct {
	workspacePath string

	mut     sync.RWMutex
	volumes map[string]*VolumeState
}

type VolumeState struct {
	Name       string `json:"name"`
	ID         string `json:"id"`
	LimitID    uint16 `json:"limitID"`
	Size       int64  `json:"size"`
	Path       string `json:"path"`
	Filesystem string `json:"filesystem"`
}

func newStateHandler(workspacePath string) (*stateHandler, error) {
	volumes := map[string]*VolumeState{}

	err := filepath.Walk(workspacePath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path != workspacePath && info != nil && info.IsDir() {
			return filepath.SkipDir
		}

		if strings.HasSuffix(path, volumeStateFileSuffix) {
			volume, err := parseVolumeStateFile(path)
			if err != nil {
				return fmt.Errorf("can't parse volume state file at %q: %w", path, err)
			}
			volumes[volume.ID] = volume
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("can't read volume state files at %q: %w", workspacePath, err)
	}

	return &stateHandler{
		workspacePath: workspacePath,
		mut:           sync.RWMutex{},
		volumes:       volumes,
	}, nil
}

func parseVolumeStateFile(path string) (*VolumeState, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("can't open file at %q: %w", path, err)
	}
	defer f.Close()

	v := &VolumeState{}
	if err := json.NewDecoder(f).Decode(v); err != nil {
		return nil, fmt.Errorf("can't parse file at %q: %w", path, err)
	}

	return v, nil
}

func (s *stateHandler) getVolumeStatePath(id string) string {
	return fmt.Sprintf("%s%s", path.Join(s.workspacePath, id), volumeStateFileSuffix)
}

func (s *stateHandler) GetVolumeStateByName(name string) *VolumeState {
	s.mut.RLock()
	defer s.mut.RUnlock()
	for _, v := range s.volumes {
		if v.Name == name {
			return v
		}
	}

	return nil
}

func (s *stateHandler) GetVolumeStateByID(id string) *VolumeState {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.volumes[id]
}

func (s *stateHandler) SaveVolumeState(volume *VolumeState) (err error) {
	statePath := s.getVolumeStatePath(volume.ID)

	f, err := os.Create(statePath)
	if err != nil {
		return fmt.Errorf("can't open state file %q: %w", statePath, err)
	}

	defer func() { err = errors.NewAggregate([]error{err, f.Close()}) }()

	if err := json.NewEncoder(f).Encode(volume); err != nil {
		return fmt.Errorf("can't encode state file %q: %w", statePath, err)
	}

	s.mut.Lock()
	defer s.mut.Unlock()
	s.volumes[volume.ID] = volume

	return nil
}

func (s *stateHandler) DeleteVolumeState(id string) error {
	statePath := s.getVolumeStatePath(id)
	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("can't remove volume state file at %q: %w", statePath, err)
	}

	s.mut.Lock()
	defer s.mut.Unlock()
	for _, v := range s.volumes {
		if v.ID == id {
			delete(s.volumes, v.ID)
			break
		}
	}

	return nil
}

func (s *stateHandler) GetTotalVolumesSize() int64 {
	s.mut.RLock()
	defer s.mut.RUnlock()
	total := int64(0)
	for _, v := range s.volumes {
		total += v.Size
	}
	return total
}

func (s *stateHandler) GetVolumes() []VolumeState {
	s.mut.RLock()
	defer s.mut.RUnlock()

	volumes := make([]VolumeState, 0, len(s.volumes))
	for _, v := range s.volumes {
		volumes = append(volumes, *v)
	}

	return volumes
}
