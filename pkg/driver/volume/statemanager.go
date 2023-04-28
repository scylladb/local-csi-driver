// Copyright (c) 2023 ScyllaDB.

package volume

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync"

	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
)

const (
	volumeStateFileExtension = "json"
	MetadataFileMaxSize      = 4 * 1024
)

type AccessType int

const (
	MountAccess AccessType = iota
	BlockAccess
)

type VolumeState struct {
	Name    string `json:"name"`
	ID      string `json:"id"`
	LimitID uint32 `json:"limitID"`
	Size    int64  `json:"size"`
}

func (vs *VolumeState) VolumePath(volumesDir string) string {
	return filepath.Join(volumesDir, vs.ID)
}

func (vs *VolumeState) IsEmpty() bool {
	return len(vs.Name) == 0 || len(vs.ID) == 0
}

type StateManager struct {
	workspacePath string

	mut              sync.RWMutex
	volumes          map[string]*VolumeState
	volumeNameToID   map[string]string
	volumesTotalSize int64
}

func NewStateManager(workspacePath string) (*StateManager, error) {
	volumes := map[string]*VolumeState{}
	volumeNameToID := map[string]string{}
	var volumesTotalSize int64

	err := filepath.WalkDir(workspacePath, func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if fpath != workspacePath && d.IsDir() {
			return filepath.SkipDir
		}

		if path.Ext(fpath) == fmt.Sprintf(".%s", volumeStateFileExtension) {
			vs, err := parseVolumeStateFile(fpath)
			if err != nil {
				return fmt.Errorf("can't parse volume state file at %q: %w", fpath, err)
			}

			if vs.IsEmpty() {
				klog.Warningf("Ignoring %q state file because it doesn't contain volume information", fpath)
				return nil
			}

			volumes[vs.ID] = vs
			volumeNameToID[vs.Name] = vs.ID
			volumesTotalSize += vs.Size
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("can't read volume state files at %q: %w", workspacePath, err)
	}

	return &StateManager{
		workspacePath:    workspacePath,
		mut:              sync.RWMutex{},
		volumes:          volumes,
		volumeNameToID:   volumeNameToID,
		volumesTotalSize: volumesTotalSize,
	}, nil
}

func (s *StateManager) getVolumeStatePath(id string) string {
	volumePath := path.Join(s.workspacePath, id)
	return fmt.Sprintf("%s.%s", volumePath, volumeStateFileExtension)
}

func (s *StateManager) GetVolumeStateByName(name string) *VolumeState {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.volumes[s.volumeNameToID[name]]
}

func (s *StateManager) GetVolumeStateByID(id string) *VolumeState {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.volumes[id]
}

func (s *StateManager) SaveVolumeState(volume *VolumeState) (err error) {
	statePath := s.getVolumeStatePath(volume.ID)

	f, err := os.Create(statePath)
	if err != nil {
		return fmt.Errorf("can't open state file %q: %w", statePath, err)
	}

	defer func() {
		closeErr := f.Close()
		err = errors.NewAggregate([]error{err, closeErr})
	}()

	err = json.NewEncoder(f).Encode(volume)
	if err != nil {
		return fmt.Errorf("can't encode state file %q: %w", statePath, err)
	}

	s.mut.Lock()
	defer s.mut.Unlock()
	s.volumes[volume.ID] = volume
	s.volumeNameToID[volume.Name] = volume.ID
	s.volumesTotalSize += volume.Size

	return nil
}

func (s *StateManager) DeleteVolumeState(id string) error {
	statePath := s.getVolumeStatePath(id)
	err := os.Remove(statePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("can't remove volume state file at %q: %w", statePath, err)
	}

	s.mut.Lock()
	defer s.mut.Unlock()
	v, ok := s.volumes[id]
	if ok {
		delete(s.volumeNameToID, v.Name)
		delete(s.volumes, id)
		s.volumesTotalSize -= v.Size
	}

	return nil
}

func (s *StateManager) GetTotalVolumesSize() int64 {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.volumesTotalSize
}

func (s *StateManager) GetVolumes() []VolumeState {
	s.mut.RLock()
	defer s.mut.RUnlock()

	volumes := make([]VolumeState, 0, len(s.volumes))
	for _, v := range s.volumes {
		volumes = append(volumes, *v)
	}

	return volumes
}

func parseVolumeStateFile(path string) (vs *VolumeState, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("can't open file at %q: %w", path, err)
	}
	defer func() {
		closeErr := f.Close()
		if closeErr != nil {
			err = errors.NewAggregate([]error{err, closeErr})
		}
	}()

	vs = &VolumeState{}
	err = json.NewDecoder(f).Decode(vs)
	if err != nil {
		return nil, fmt.Errorf("can't parse file at %q: %w", path, err)
	}

	return vs, nil
}
