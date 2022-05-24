// Copyright (C) 2021 ScyllaDB

package local

import (
	"context"
	"fmt"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
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
	klog.V(4).Infof("NodePublishVolume: called with args %+v", *req)

	volumeId := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	if err := d.validateVolumeCapabilities([]*csi.VolumeCapability{volCap}); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Volume capability not supported: %s", err))
	}

	if volCap.GetMount() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability access type must be mount")
	}

	options := []string{"bind"}
	if req.GetReadonly() {
		options = append(options, "ro")
	}

	path := d.getVolumePath(volumeId)

	if err := os.Mkdir(targetPath, 0700); err != nil && !os.IsExist(err) {
		return nil, status.Errorf(codes.Internal, "Failed to create target path %s", err)
	}

	klog.V(2).Infof("Mounting %s at %s with options %s", path, targetPath, options)
	if err := d.mounter.Mount(path, targetPath, "", options); err != nil {
		_ = os.Remove(targetPath)
		return nil, status.Errorf(codes.Internal, "Failed to mount device %q at %q: %s", path, targetPath, err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(4).Infof("NodeUnpublishVolume: called with args %+v", *req)

	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	if err := d.mounter.Unmount(targetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmount target path %s", err)
	}

	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "Failed to remove target path %s", err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (d *driver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	klog.V(4).Infof("NodeGetInfo: called with args %+v", req)

	return &csi.NodeGetInfoResponse{
		NodeId:             d.nodeName,
		AccessibleTopology: d.getTopology(),
	}, nil
}
