// Copyright (c) 2023 ScyllaDB.

package driver

import (
	"context"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"github.com/scylladb/local-csi-driver/pkg/driver/volume"
	"github.com/scylladb/local-csi-driver/pkg/util/slices"
	"github.com/scylladb/local-csi-driver/pkg/util/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/klog/v2"
)

func (d *driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(4).InfoS("New request", "server", "controller", "function", "CreateVolume", "request", protosanitizer.StripSecrets(req))

	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Name missing in request")
	}
	caps := req.GetVolumeCapabilities()
	if caps == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities missing in request")
	}

	err := d.validateVolumeCapabilities(caps)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Unsupported volume capabilities: %v", err))
	}

	parameters := req.GetParameters()
	err = d.validateVolumeParameters(parameters)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Unsupported volume parameters: %v", err))
	}

	var accessTypeMount, accessTypeBlock bool
	var requestedAccessType volume.AccessType
	var requestedFilesystem string

	for _, c := range caps {
		if c.GetBlock() != nil {
			accessTypeBlock = true
			requestedAccessType = volume.BlockAccess
		} else if c.GetMount() != nil {
			accessTypeMount = true
			requestedAccessType = volume.MountAccess

			requestedFilesystem = c.GetMount().FsType
		} else {
			return nil, status.Errorf(codes.InvalidArgument, "Unsupported volume capability access type %v", c.GetAccessType())
		}
	}

	if accessTypeBlock && accessTypeMount {
		return nil, status.Error(codes.InvalidArgument, "Can't have both block and mount access type")
	}

	if !slices.Contains(d.volumeManager.SupportedAccessTypes(), requestedAccessType) {
		return nil, status.Errorf(codes.InvalidArgument, "Unsupported access type")
	}

	if !slices.Contains(d.volumeManager.SupportedFilesystems(), requestedFilesystem) {
		return nil, status.Errorf(codes.InvalidArgument, "Unsupported filesystem: %q", requestedFilesystem)
	}

	if req.AccessibilityRequirements != nil && len(req.AccessibilityRequirements.Requisite) > 0 {
		volumeAccessibleTopology := d.getVolumeAccessibleTopology()

		var accessibleTopologySegments []map[string]string
		var requiredTopologySegments []map[string]string

		for _, accessibleTopology := range volumeAccessibleTopology {
			accessibleTopologySegments = append(accessibleTopologySegments, accessibleTopology.Segments)
		}

		for _, requiredTopology := range req.AccessibilityRequirements.Requisite {
			requiredTopologySegments = append(requiredTopologySegments, requiredTopology.Segments)
		}

		if !equality.Semantic.DeepEqual(requiredTopologySegments, accessibleTopologySegments) {
			return nil, status.Errorf(codes.ResourceExhausted, "Cannot satisfy accessibility requirements")
		}
	}

	capacity := req.GetCapacityRange().GetRequiredBytes()

	vs := d.volumeManager.GetVolumeStateByName(req.GetName())
	if vs != nil {
		if vs.Size != capacity {
			return nil, status.Errorf(codes.AlreadyExists, "Volume with %q name but with different size already exist", req.GetName())
		}

		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:           vs.ID,
				CapacityBytes:      capacity,
				ContentSource:      req.GetVolumeContentSource(),
				AccessibleTopology: d.getVolumeAccessibleTopology(),
			},
		}, nil
	}

	volumeID := uuid.MustRandom().String()

	// Serialize volume creation to ensure we won't allocate more than we actually can, as
	// node capacity information is published in intervals.
	d.mut.Lock()
	defer d.mut.Unlock()

	availableCapacity, err := d.volumeManager.GetAvailableCapacity()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Cannot check node capacity: %v", err)
	}

	if capacity > availableCapacity {
		return nil, status.Errorf(codes.OutOfRange, "Requested capacity is bigger than available: %d", availableCapacity)
	}

	err = d.volumeManager.CreateVolume(volumeID, req.GetName(), capacity, requestedAccessType)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Can't create volume: %s", err)
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:           volumeID,
			CapacityBytes:      req.GetCapacityRange().GetRequiredBytes(),
			ContentSource:      req.GetVolumeContentSource(),
			AccessibleTopology: d.getVolumeAccessibleTopology(),
		},
	}, nil
}

func (d *driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.V(4).InfoS("New request", "server", "controller", "function", "DeleteVolume", "request", protosanitizer.StripSecrets(req))
	volID := req.GetVolumeId()
	if volID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	err := d.volumeManager.DeleteVolume(volID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to delete volume: %v", err)
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (d *driver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	klog.V(4).InfoS("New request", "server", "controller", "function", "GetCapacity", "request", protosanitizer.StripSecrets(req))

	capacity, err := d.volumeManager.GetAvailableCapacity()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Cannot check node capacity: %v", err)
	}

	return &csi.GetCapacityResponse{
		AvailableCapacity: capacity,
	}, nil
}

func (d *driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	klog.V(4).InfoS("New request", "server", "controller", "function", "ValidateVolumeCapabilities", "request", protosanitizer.StripSecrets(req))

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "VolumeID is missing in request")
	}

	v := d.volumeManager.GetVolumeStateByID(volumeID)
	if v == nil {
		return nil, status.Errorf(codes.NotFound, "Volume with VolumeID %q does not exists", volumeID)
	}

	volumeContext := req.GetVolumeContext()
	if len(volumeContext) != 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Expected VolumeContext to be empty but got %v", volumeContext)
	}

	caps := req.GetVolumeCapabilities()
	if caps == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities are missing in request")
	}

	err := d.validateVolumeCapabilities(caps)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Unsupported volume capabilites: %s", err))
	}

	parameters := req.GetParameters()
	err = d.validateVolumeParameters(parameters)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Unsupported volume parameters: %s", err))
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
			VolumeContext:      req.GetVolumeContext(),
			Parameters:         req.GetParameters(),
		},
	}, nil
}

func (d *driver) ControllerGetCapabilities(ctx context.Context, request *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
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

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: csc,
	}, nil
}
