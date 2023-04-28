// Copyright (c) 2023 ScyllaDB.

package driver

import (
	"context"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/limit"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/util/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

func (d *driver) NodeGetCapabilities(ctx context.Context, request *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: nil,
	}, nil
}

func (d *driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(4).InfoS("New request", "server", "node", "function", "NodePublishVolume", "request", protosanitizer.StripSecrets(req))

	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	err := d.validateVolumeCapabilities([]*csi.VolumeCapability{volCap})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Volume capability not supported: %s", err))
	}

	if volCap.GetMount() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability access type must be mount")
	}

	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	for _, mf := range volCap.GetMount().MountFlags {
		mountOptions = append(mountOptions, mf)
	}

	mountOptions = slices.Unique(mountOptions)

	err = d.volumeManager.Mount(volumeID, targetPath, volCap.GetMount().FsType, mountOptions)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to publish volume: %v", err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(4).InfoS("New request", "server", "node", "function", "NodePublishVolume", "request", protosanitizer.StripSecrets(req))

	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	err := d.volumeManager.Unmount(targetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmount volume at path %q: %v", targetPath, err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (d *driver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	klog.V(4).InfoS("New request", "server", "node", "function", "NodeGetInfo", "request", protosanitizer.StripSecrets(req))

	return &csi.NodeGetInfoResponse{
		NodeId:             d.nodeName,
		MaxVolumesPerNode:  int64(limit.MaxLimits),
		AccessibleTopology: d.getNodeAccessibleTopology(),
	}, nil
}
