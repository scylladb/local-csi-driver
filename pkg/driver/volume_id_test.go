// Copyright (c) 2026 ScyllaDB.

package driver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/scylladb/local-csi-driver/pkg/driver/volume"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

const testVolumeID = "2ae33540-8731-4fa5-8f6a-3e5ee4e9b45f"

func newVolumeIDTestDriver(t *testing.T) (*driver, *mount.FakeMounter, string) {
	t.Helper()
	root := t.TempDir()
	volumesDir := filepath.Join(root, "volumes")
	if err := os.MkdirAll(filepath.Join(volumesDir, testVolumeID), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	sm, err := volume.NewStateManager(volumesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.SaveVolumeState(&volume.VolumeState{ID: testVolumeID, Name: "test-volume", Size: 1024}); err != nil {
		t.Fatal(err)
	}
	mounter := mount.NewFakeMounter(nil)
	vm, err := volume.NewVolumeManager(volumesDir, sm, volume.WithMounter(mounter))
	if err != nil {
		t.Fatal(err)
	}
	return NewDriver("test-driver", "test", "test-node", vm), mounter, root
}

func TestDeleteVolumeID(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id   string
		code codes.Code
	}{
		{"", codes.InvalidArgument},
		{".", codes.OK},
		{"..", codes.OK},
		{"../outside", codes.OK},
		{"/outside", codes.OK},
		{"not-a-uuid", codes.OK},
		{testVolumeID + "/..", codes.OK},
		{"9b2a15dc-1a54-4f23-8810-a926aa9f8783", codes.OK},
		{testVolumeID, codes.OK},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			d, _, root := newVolumeIDTestDriver(t)
			_, err := d.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: tc.id})
			if status.Code(err) != tc.code {
				t.Fatalf("expected %v, got %v", tc.code, err)
			}
			if data, err := os.ReadFile(filepath.Join(root, "outside")); err != nil || string(data) != "keep" {
				t.Fatalf("unrelated file changed: data=%q, err=%v", data, err)
			}
			for _, name := range []string{testVolumeID, testVolumeID + ".json"} {
				_, err := os.Stat(filepath.Join(root, "volumes", name))
				if tc.id == testVolumeID {
					if !os.IsNotExist(err) {
						t.Errorf("expected %q to be removed, got %v", name, err)
					}
				} else if err != nil {
					t.Errorf("existing volume file %q was affected: %v", name, err)
				}
			}
		})
	}
}

func TestNodePublishVolumeID(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id   string
		code codes.Code
	}{
		{"", codes.InvalidArgument},
		{".", codes.InvalidArgument},
		{"..", codes.InvalidArgument},
		{"../outside", codes.InvalidArgument},
		{"/outside", codes.InvalidArgument},
		{"not-a-uuid", codes.InvalidArgument},
		{testVolumeID + "/..", codes.InvalidArgument},
		{"9b2a15dc-1a54-4f23-8810-a926aa9f8783", codes.NotFound},
		{testVolumeID, codes.OK},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			d, mounter, root := newVolumeIDTestDriver(t)
			target := filepath.Join(root, "target")
			_, err := d.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
				VolumeId:   tc.id,
				TargetPath: target,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
					AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
				},
			})
			if status.Code(err) != tc.code {
				t.Fatalf("expected %v, got %v", tc.code, err)
			}
			mounts, err := mounter.List()
			if err != nil {
				t.Fatal(err)
			}
			if tc.code == codes.OK {
				if len(mounts) != 1 || mounts[0].Device != filepath.Join(root, "volumes", testVolumeID) || mounts[0].Path != target {
					t.Fatalf("unexpected mounts: %#v", mounts)
				}
			} else {
				if len(mounts) != 0 {
					t.Errorf("rejected publish mounted a volume: %#v", mounts)
				}
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Errorf("rejected publish created the target: %v", err)
				}
			}
		})
	}
}
