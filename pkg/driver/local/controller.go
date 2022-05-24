// Copyright (C) 2021 ScyllaDB

package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/util/uuid"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type AccessType int

const (
	MountAccess AccessType = iota
	BlockAccess
)

func (d *driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(4).Infof("CreateVolume: called with args %+v", *req)

	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Name missing in request")
	}
	caps := req.GetVolumeCapabilities()
	if caps == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities missing in request")
	}

	if err := d.validateVolumeCapabilities(caps); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Volume capabilities not supported: %s", err))
	}

	var accessTypeMount, accessTypeBlock bool
	var requestedAccessType AccessType
	requestedFilesystem := DefaultFS

	for _, c := range caps {
		if c.GetBlock() != nil {
			accessTypeBlock = true
			requestedAccessType = BlockAccess
		}
		if c.GetMount() != nil {
			accessTypeMount = true
			requestedAccessType = MountAccess

			requestedFilesystem = c.GetMount().FsType
		}
	}
	if accessTypeBlock && accessTypeMount {
		return nil, status.Error(codes.InvalidArgument, "Can't have both block and mount access type")
	}

	capacity := req.GetCapacityRange().GetRequiredBytes()

	if vs := d.state.GetVolumeStateByName(req.GetName()); vs != nil {
		if vs.Size < capacity {
			return nil, status.Errorf(codes.AlreadyExists, "Volume with the same name: %s but with different size already exist", req.GetName())
		}

		if vs.Filesystem != requestedFilesystem {
			return nil, status.Errorf(codes.AlreadyExists, "Volume with the same name: %s but with filesystem already exist", req.GetName())
		}

		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      vs.ID,
				CapacityBytes: vs.Size,
				VolumeContext: req.GetParameters(),
				ContentSource: req.GetVolumeContentSource(),
				AccessibleTopology: []*csi.Topology{
					d.getTopology(),
				},
			},
		}, nil
	}

	volumeID := uuid.MustRandom().String()

	limitID, err := d.createVolume(volumeID, req.GetName(), capacity, requestedAccessType, requestedFilesystem)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Can't create volume: %s", err)
	}

	volumeState := &VolumeState{
		Name:       req.GetName(),
		ID:         volumeID,
		LimitID:    limitID,
		Size:       capacity,
		Path:       d.getVolumePath(volumeID),
		Filesystem: requestedFilesystem,
	}

	if err := d.state.SaveVolumeState(volumeState); err != nil {
		if removeLimitErr := d.getLimiter(requestedFilesystem).RemoveLimit(limitID); err != nil {
			klog.Errorf("can't remove the limit %d, it's going to leak: %w", limitID, removeLimitErr)
		}

		return nil, status.Errorf(codes.Internal, "Can't save volume state: %s", err)
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
			VolumeContext: req.GetParameters(),
			ContentSource: req.GetVolumeContentSource(),
			AccessibleTopology: []*csi.Topology{
				d.getTopology(),
			},
		},
	}, nil
}

func (d *driver) createVolume(volID, name string, capacity int64, volAccessType AccessType, requestedFilesystem string) (uint16, error) {
	path := d.getVolumePath(volID)

	switch volAccessType {
	case MountAccess:
		klog.V(2).Infof("Creating directory at %s", path)
		if err := os.MkdirAll(path, 0700); err != nil {
			return 0, fmt.Errorf("can't create directory at %q: %w", path, err)
		}

		limiter := d.getLimiter(requestedFilesystem)
		klog.V(2).Infof("Initializing new limit at %s with %d limit using %s limiter", path, capacity, limiter.GetName())

		id, err := limiter.InitLimit(path)
		if err != nil {
			return 0, fmt.Errorf("can't init new limit: %w", err)
		}
		if err := limiter.SetLimit(id, capacity); err != nil {
			return 0, fmt.Errorf("can't set limit %d with %d capacity: %w", id, capacity, err)
		}

		klog.V(2).Infof("New limit %d created successfully at %s", id, path)
		return id, nil
	default:
		return 0, fmt.Errorf("unsupported access type %v", volAccessType)
	}
}

func (d *driver) getVolumePath(volID string) string {
	return filepath.Join(d.volumesDir, volID)
}

func (d *driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.V(4).Infof("DeleteVolume: called with args %+v", *req)
	volID := req.GetVolumeId()
	if volID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	path := d.getVolumePath(volID)
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "Failed to delete volume mount %v at %s: %s", volID, path, err)
	}
	klog.V(2).Infof("Volume %q directory at %q is removed", volID, path)

	volumeState := d.state.GetVolumeStateByID(volID)
	if volumeState != nil {
		if err := d.getLimiter(volumeState.Filesystem).RemoveLimit(volumeState.LimitID); err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to delete volume limit %v: %s", volID, err)
		}
		klog.V(2).Infof("Limit %d at path %q removed", volumeState.LimitID, volumeState.Path)
	}

	if err := d.state.DeleteVolumeState(volID); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to delete volume state %v: %s", volID, err)
	}
	klog.V(2).Infof("Volume %q state removed", volID)

	return &csi.DeleteVolumeResponse{}, nil
}

func (d *driver) ControllerGetCapabilities(ctx context.Context, request *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: d.getControllerServiceCapabilities(),
	}, nil
}

func (d *driver) getControllerServiceCapabilities() []*csi.ControllerServiceCapability {
	cs := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_GET_CAPACITY,
	}

	var csc []*csi.ControllerServiceCapability
	for _, c := range cs {
		csc = append(csc, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: c,
				},
			},
		})
	}
	return csc
}

func (d *driver) validateControllerServiceRequest(ct csi.ControllerServiceCapability_RPC_Type) error {
	if ct == csi.ControllerServiceCapability_RPC_UNKNOWN {
		return nil
	}

	for _, c := range d.getControllerServiceCapabilities() {
		if ct == c.GetRpc().GetType() {
			return nil
		}
	}
	return status.Errorf(codes.InvalidArgument, "unsupported capability %s", ct)
}

func (d *driver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	klog.V(4).Infof("GetCapacity: called with args %+v", *req)

	var stat unix.Statfs_t
	if err := unix.Statfs(d.volumesDir, &stat); err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("cannot check statfs of %s: %w", d.volumesDir, err).Error())
	}

	metadataSize := len(d.state.GetVolumes()) * MetadataFileMaxSize
	return &csi.GetCapacityResponse{
		AvailableCapacity: stat.Bsize*int64(stat.Blocks) - d.state.GetTotalVolumesSize() - int64(metadataSize),
	}, nil
}

func (d *driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "VolumeID is missing in request")
	}

	if volume := d.state.GetVolumeStateByID(req.GetVolumeId()); volume == nil {
		return nil, status.Error(codes.NotFound, "VolumeID does not exists")
	}

	caps := req.GetVolumeCapabilities()
	if caps == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities are missing in request")
	}

	if err := d.validateVolumeCapabilities(caps); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Volume capabilities not supported: %s", err))
	}

	return &csi.ValidateVolumeCapabilitiesResponse{}, nil
}
