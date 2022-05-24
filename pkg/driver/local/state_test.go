package local

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

	if err := json.NewEncoder(f).Encode(volumeState); err != nil {
		return fmt.Errorf("can't encode volume state: %w", err)
	}

	return nil
}

func newVolumeState(id, name string) *VolumeState {
	return &VolumeState{
		ID:         id,
		Name:       name,
		LimitID:    1,
		Size:       1024,
		Path:       path.Join("/some/path/to", id),
		Filesystem: "xfs",
	}
}

func TestStateHandler(t *testing.T) {
	t.Parallel()

	tt := []struct {
		name          string
		existingFiles map[string]*VolumeState
		trigger       func(st *stateHandler) error
		check         func(dir string, st *stateHandler) error
	}{
		{
			name: "existing state is preloaded",
			existingFiles: map[string]*VolumeState{
				"volume-1-uuid.json": newVolumeState("volume-1-uuid", "volume-1"),
			},
			trigger: func(st *stateHandler) error {
				return nil
			},
			check: func(dir string, st *stateHandler) error {
				expectedState := newVolumeState("volume-1-uuid", "volume-1")
				vs := st.GetVolumeStateByID("volume-1-uuid")
				if !reflect.DeepEqual(vs, expectedState) {
					return fmt.Errorf("expected %#v, got %#v", expectedState, vs)
				}
				return nil
			},
		},
		{
			name:          "returns nil state when volume is unknown",
			existingFiles: map[string]*VolumeState{},
			trigger: func(st *stateHandler) error {
				return nil
			},
			check: func(dir string, st *stateHandler) error {
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
			trigger: func(st *stateHandler) error {
				return st.DeleteVolumeState("volume-1-uuid")
			},
			check: func(dir string, st *stateHandler) error {
				if vs := st.GetVolumeStateByID("volume-1-uuid"); vs != nil {
					return fmt.Errorf("expected nil volume state, got %#v", vs)
				}

				stateFilePath := path.Join(dir, "volume-1-uuid.json")
				if _, err := os.Stat(stateFilePath); !os.IsNotExist(err) {
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
			trigger: func(st *stateHandler) error {
				vs := newVolumeState("volume-1-uuid", "volume-1")
				vs.Size = 666
				return st.SaveVolumeState(vs)
			},
			check: func(dir string, st *stateHandler) error {
				expectedVs := newVolumeState("volume-1-uuid", "volume-1")
				expectedVs.Size = 666

				vs := st.GetVolumeStateByID("volume-1-uuid")
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
				if err := json.NewDecoder(stateFile).Decode(fsVs); err != nil {
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
			trigger: func(st *stateHandler) error {
				return nil
			},
			check: func(dir string, st *stateHandler) error {
				expectedState := newVolumeState("volume-1-uuid", "volume-1")
				vs := st.GetVolumeStateByName("volume-1")
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
			trigger: func(st *stateHandler) error {
				return nil
			},
			check: func(dir string, st *stateHandler) error {
				totalSize := st.GetTotalVolumesSize()
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

			tempDir, err := os.MkdirTemp(os.TempDir(), "state-handler-")
			defer os.RemoveAll(tempDir)

			for fileName, vs := range tc.existingFiles {
				filePath := path.Join(tempDir, fileName)
				if err := writeVolumeState(filePath, vs); err != nil {
					t.Fatalf("can't write initial state to file %q: %v", filePath, err)
				}
			}

			sh, err := newStateHandler(tempDir)
			if err != nil {
				t.Fatal(err)
			}

			if err := tc.trigger(sh); err != nil {
				t.Fatal(err)
			}

			if err := tc.check(tempDir, sh); err != nil {
				t.Fatal(err)
			}
		})
	}
}
