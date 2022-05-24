// Copyright (C) 2021 ScyllaDB

package local

import (
	"fmt"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/mount-utils"
)

type driver struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedNodeServer
	csi.UnimplementedControllerServer

	name       string
	version    string
	volumesDir string
	nodeName   string

	mounter  mount.Interface
	state    *stateHandler
	limiters map[string]Limiter
}

var _ csi.IdentityServer = &driver{}
var _ csi.NodeServer = &driver{}
var _ csi.ControllerServer = &driver{}

const (
	NodeNameTopologyKey = "local.csi.scylladb.com/node"

	XFS       = "xfs"
	DefaultFS = ""
)

var (
	supportedFSTypes     = []string{XFS, DefaultFS}
	volumeCapAccessModes = []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	}
)

type DriverOption func(d *driver)

func WithLimiter(fsType string, limiter Limiter) func(*driver) {
	return func(d *driver) {
		if d.limiters == nil {
			d.limiters = map[string]Limiter{}
		}
		d.limiters[fsType] = limiter
	}
}

func WithMounter(mounter mount.Interface) func(*driver) {
	return func(d *driver) {
		d.mounter = mounter
	}
}

func NewDriver(name, version, volumesDir, nodeName string, options ...DriverOption) (*driver, error) {
	s, err := newStateHandler(volumesDir)
	if err != nil {
		return nil, fmt.Errorf("can't create state handler: %w", err)
	}

	d := &driver{
		name:       name,
		version:    version,
		volumesDir: volumesDir,
		nodeName:   nodeName,

		mounter: mount.New(""),
		state:   s,
	}

	for _, option := range options {
		option(d)
	}

	if d.limiters == nil {
		xl, err := NewXFSLimiter(volumesDir, s.GetVolumes())
		if err != nil {
			return nil, fmt.Errorf("can't create XFS limiter: %w", err)
		}

		d.limiters = map[string]Limiter{
			XFS:       xl,
			DefaultFS: &NoopLimiter{},
		}
	}

	return d, nil
}

func (d *driver) getTopology() *csi.Topology {
	return &csi.Topology{
		Segments: map[string]string{
			NodeNameTopologyKey: d.nodeName,
		},
	}
}

func (d *driver) validateVolumeCapabilities(volCaps []*csi.VolumeCapability) error {
	if err := d.validateAccessMode(volCaps); err != nil {
		return fmt.Errorf("validate volume access mode: %w", err)
	}

	if err := d.validateAccessType(volCaps); err != nil {
		return fmt.Errorf("validate volume access type: %w", err)
	}

	if err := d.validateFSType(volCaps); err != nil {
		return fmt.Errorf("validate volume filesystem type: %w", err)
	}
	return nil
}

func (d *driver) validateAccessMode(volCaps []*csi.VolumeCapability) error {
	isSupportedAccessMode := func(cap *csi.VolumeCapability) bool {
		for _, m := range volumeCapAccessModes {
			if m == cap.AccessMode.GetMode() {
				return true
			}
		}
		return false
	}

	var invalidModes []string
	for _, c := range volCaps {
		if !isSupportedAccessMode(c) {
			invalidModes = append(invalidModes, c.AccessMode.GetMode().String())
		}
	}
	if len(invalidModes) != 0 {
		return fmt.Errorf("invalid access mode: %s", strings.Join(invalidModes, ","))
	}
	return nil
}

func (d *driver) validateAccessType(volCaps []*csi.VolumeCapability) error {
	for _, c := range volCaps {
		if c.GetMount() == nil {
			return fmt.Errorf("only filesystem volumes are supported")
		}
	}
	return nil
}

func (d *driver) validateFSType(volCaps []*csi.VolumeCapability) error {
	isSupportedFSType := func(cap *csi.VolumeCapability) bool {
		for _, m := range supportedFSTypes {
			if m == cap.GetMount().FsType {
				return true
			}
		}
		return false
	}

	var invalidFStypes []string
	for _, c := range volCaps {
		if !isSupportedFSType(c) {
			invalidFStypes = append(invalidFStypes, c.GetMount().FsType)
		}
	}
	if len(invalidFStypes) != 0 {
		return fmt.Errorf("invalid fstype: %s", strings.Join(invalidFStypes, ","))
	}
	return nil
}

func (d *driver) getLimiter(fs string) Limiter {
	if l, ok := d.limiters[fs]; ok {
		return l
	}
	return d.limiters[DefaultFS]
}
