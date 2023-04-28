// Copyright (c) 2023 ScyllaDB.

package driver

import (
	"fmt"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/volume"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/util/slices"
	"k8s.io/apimachinery/pkg/util/errors"
)

type driver struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedNodeServer
	csi.UnimplementedControllerServer

	name          string
	version       string
	nodeName      string
	volumeManager *volume.VolumeManager
	mut           sync.Mutex
}

var _ csi.IdentityServer = &driver{}
var _ csi.NodeServer = &driver{}
var _ csi.ControllerServer = &driver{}

const (
	NodeNameTopologyKey = "local.csi.scylladb.com/node"
)

var (
	volumeCapAccessModes = []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	}
)

func NewDriver(name, version, nodeName string, volumeManager *volume.VolumeManager) *driver {
	return &driver{
		name:     name,
		version:  version,
		nodeName: nodeName,

		volumeManager: volumeManager,
		mut:           sync.Mutex{},
	}
}

func (d *driver) getNodeAccessibleTopology() *csi.Topology {
	return &csi.Topology{
		Segments: map[string]string{
			NodeNameTopologyKey: d.nodeName,
		},
	}
}

func (d *driver) getVolumeAccessibleTopology() []*csi.Topology {
	return []*csi.Topology{
		d.getNodeAccessibleTopology(),
	}
}

func (d *driver) validateVolumeCapabilities(volCaps []*csi.VolumeCapability) error {
	var errs []error

	for _, volCap := range volCaps {
		if !slices.Contains(volumeCapAccessModes, volCap.AccessMode.GetMode()) {
			errs = append(errs, fmt.Errorf("unsupported access mode %q", volCap.AccessMode.GetMode().String()))
		}

		if volCap.GetMount() == nil {
			errs = append(errs, fmt.Errorf("only filesystem volumes are supported"))
		}

		if volCap.GetMount() != nil && !slices.Contains(d.volumeManager.SupportedFilesystems(), volCap.GetMount().FsType) {
			errs = append(errs, fmt.Errorf("unsupported fsType %q", volCap.GetMount().FsType))
		}
	}

	err := errors.NewAggregate(errs)
	if err != nil {
		return err
	}

	return nil
}

func (d *driver) validateVolumeParameters(parameters map[string]string) error {
	var errs []error
	for k := range parameters {
		switch k {
		default:
			errs = append(errs, fmt.Errorf("unsupported volume parameter key: %q", k))
		}
	}

	err := errors.NewAggregate(errs)
	if err != nil {
		return err
	}

	return nil
}
