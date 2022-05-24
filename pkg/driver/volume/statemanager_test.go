// Copyright (c) 2023 ScyllaDB.

package volume

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"reflect"
	"testing"
)

func writeVolumeState(path string, volumeState *VolumeState) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("can't open file at %q: %w", path, err)
	}
	defer f.Close()

	err = json.NewEncoder(f).Encode(volumeState)
	if err != nil {
		return fmt.Errorf("can't encode volume state: %w", err)
	}

	return nil
}

func newVolumeState(id, name string) *VolumeState {
	return &VolumeState{
		ID:      id,
		Name:    name,
		LimitID: 1,
		Size:    1024,
	}
}

func TestStateManager(t *testing.T) {
	t.Parallel()

	tt := []struct {
		name          string
		existingFiles map[string]*VolumeState
		trigger       func(sm *StateManager) error
		check         func(dir string, st *StateManager) error
	}{
		{
			name: "existing state is preloaded",
			existingFiles: map[string]*VolumeState{
				"volume-1-uuid.json": newVolumeState("volume-1-uuid", "volume-1"),
			},
			trigger: func(sm *StateManager) error {
				return nil
			},
			check: func(dir string, sm *StateManager) error {
				expectedState := newVolumeState("volume-1-uuid", "volume-1")
				vs := sm.GetVolumeStateByID("volume-1-uuid")
				if !reflect.DeepEqual(vs, expectedState) {
					return fmt.Errorf("expected %#v, got %#v", expectedState, vs)
				}
				return nil
			},
		},
		{
			name:          "returns nil state when volume is unknown",
			existingFiles: map[string]*VolumeState{},
			trigger: func(sm *StateManager) error {
				return nil
			},
			check: func(dir string, st *StateManager) error {
				vs := st.GetVolumeStateByID("volume-1-uuid")
				if vs != nil {
					return fmt.Errorf("expected nil, got %#v", vs)
				}

				vs = st.GetVolumeStateByName("volume-1")
				if vs != nil {
					return fmt.Errorf("expected nil, got %#v", vs)
				}
				return nil
			},
		},
		{
			name: "state information is lost on delete",
			existingFiles: map[string]*VolumeState{
				"volume-1-uuid.json": newVolumeState("volume-1-uuid", "volume-1"),
			},
			trigger: func(sm *StateManager) error {
				return sm.DeleteVolumeState("volume-1-uuid")
			},
			check: func(dir string, sm *StateManager) error {
				vs := sm.GetVolumeStateByID("volume-1-uuid")
				if vs != nil {
					return fmt.Errorf("expected nil volume state, got %#v", vs)
				}

				stateFilePath := path.Join(dir, "volume-1-uuid.json")
				_, err := os.Stat(stateFilePath)
				if !os.IsNotExist(err) {
					return fmt.Errorf("expected volume state file to be deleted from %q, but it exists", stateFilePath)
				}

				return nil
			},
		},
		{
			name: "state information is propagated to fs",
			existingFiles: map[string]*VolumeState{
				"volume-1-uuid.json": newVolumeState("volume-1-uuid", "volume-1"),
			},
			trigger: func(sm *StateManager) error {
				vs := newVolumeState("volume-1-uuid", "volume-1")
				vs.Size = 666
				return sm.SaveVolumeState(vs)
			},
			check: func(dir string, sm *StateManager) error {
				expectedVs := newVolumeState("volume-1-uuid", "volume-1")
				expectedVs.Size = 666

				vs := sm.GetVolumeStateByID("volume-1-uuid")
				if vs == nil {
					return fmt.Errorf("expected non-nil volume state, got nil")
				}

				if !reflect.DeepEqual(expectedVs, vs) {
					return fmt.Errorf("expected %#v, got %#v", expectedVs, vs)
				}

				stateFilePath := path.Join(dir, "volume-1-uuid.json")
				stateFile, err := os.Open(stateFilePath)
				if err != nil {
					return fmt.Errorf("can't open file %q: %w", stateFilePath, err)
				}
				defer stateFile.Close()

				fsVs := &VolumeState{}
				err = json.NewDecoder(stateFile).Decode(fsVs)
				if err != nil {
					return fmt.Errorf("can't decode state file from fs %q: %w", stateFilePath, err)
				}

				if !reflect.DeepEqual(expectedVs, fsVs) {
					return fmt.Errorf("expected %#v at %q, got %#v", expectedVs, stateFilePath, vs)
				}

				return nil
			},
		},
		{
			name: "GetVolumeStateByName queries volume state by name",
			existingFiles: map[string]*VolumeState{
				"volume-1-uuid.json": newVolumeState("volume-1-uuid", "volume-1"),
			},
			trigger: func(st *StateManager) error {
				return nil
			},
			check: func(dir string, sm *StateManager) error {
				expectedState := newVolumeState("volume-1-uuid", "volume-1")
				vs := sm.GetVolumeStateByName("volume-1")
				if !reflect.DeepEqual(vs, expectedState) {
					return fmt.Errorf("expected %#v, got %#v", expectedState, vs)
				}
				return nil
			},
		},
		{
			name: "GetTotalVolumesSize returns accumulated known volume sizes",
			existingFiles: map[string]*VolumeState{
				"volume-1-uuid.json": newVolumeState("volume-1-uuid", "volume-1"),
				"volume-2-uuid.json": newVolumeState("volume-2-uuid", "volume-2"),
			},
			trigger: func(st *StateManager) error {
				return nil
			},
			check: func(dir string, sm *StateManager) error {
				totalSize := sm.GetTotalVolumesSize()
				expectedTotalSize := newVolumeState("", "").Size * 2
				if totalSize != expectedTotalSize {
					return fmt.Errorf("expected total volumes size %d, got %d", expectedTotalSize, totalSize)
				}
				return nil
			},
		},
	}

	for i := range tt {
		tc := tt[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir, err := os.MkdirTemp(os.TempDir(), "state-manager-")
			defer os.RemoveAll(tempDir)

			for fileName, vs := range tc.existingFiles {
				filePath := path.Join(tempDir, fileName)
				err = writeVolumeState(filePath, vs)
				if err != nil {
					t.Fatalf("can't write initial state to file %q: %v", filePath, err)
				}
			}

			sm, err := NewStateManager(tempDir)
			if err != nil {
				t.Fatal(err)
			}

			err = tc.trigger(sm)
			if err != nil {
				t.Fatal(err)
			}

			err = tc.check(tempDir, sm)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
