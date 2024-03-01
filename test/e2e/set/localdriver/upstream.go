// Copyright (c) 2023 ScyllaDB.

package localdriver

import (
	"context"

	g "github.com/onsi/ginkgo/v2"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/storage/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/storage/names"
	kubeframework "k8s.io/kubernetes/test/e2e/framework"
	storageframework "k8s.io/kubernetes/test/e2e/storage/framework"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"
)

type localCsiDriver struct {
}

func (d *localCsiDriver) GetDriverInfo() *storageframework.DriverInfo {
	return &storageframework.DriverInfo{
		Name:            "local.csi.scylladb.com",
		SupportedFsType: sets.NewString("xfs", ""),
		TopologyKeys:    []string{driver.NodeNameTopologyKey},
		Capabilities: map[storageframework.Capability]bool{
			storageframework.CapPersistence:      true,
			storageframework.CapExec:             true,
			storageframework.CapSingleNodeVolume: true,
			storageframework.CapTopology:         true,
			storageframework.CapCapacity:         true,
		},
	}
}

func (d *localCsiDriver) SkipUnsupportedTest(pattern storageframework.TestPattern) {}

func (d *localCsiDriver) PrepareTest(ctx context.Context, f *kubeframework.Framework) *storageframework.PerTestConfig {
	return &storageframework.PerTestConfig{
		Driver:    d,
		Prefix:    "local-csi",
		Framework: f,
		DriverNamespace: &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "local-csi-driver",
			},
		},
	}
}

func (d *localCsiDriver) GetDynamicProvisionStorageClass(ctx context.Context, config *storageframework.PerTestConfig, fsType string) *v1.StorageClass {
	defaultBindingMode := storagev1.VolumeBindingWaitForFirstConsumer
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: names.SimpleNameGenerator.GenerateName("local-csi-sc-"),
		},
		Provisioner:       "local.csi.scylladb.com",
		VolumeBindingMode: &defaultBindingMode,
		Parameters: map[string]string{
			"csi.storage.k8s.io/fstype": fsType,
		},
	}
}

var _ storageframework.DynamicPVTestDriver = &localCsiDriver{}

var _ = g.Describe("CSI upstream", func() {
	defer g.GinkgoRecover()

	driver := &localCsiDriver{}
	driverInfo := driver.GetDriverInfo()

	g.Context(driverInfo.Name, driverInfo.TestTags, func() {
		storageframework.DefineTestSuites(driver, []func() storageframework.TestSuite{
			testsuites.InitVolumesTestSuite,
			testsuites.InitVolumeIOTestSuite,
			testsuites.InitVolumeModeTestSuite,
			testsuites.InitSubPathTestSuite,
			testsuites.InitProvisioningTestSuite,
			testsuites.InitMultiVolumeTestSuite,
			testsuites.InitCapacityTestSuite,
			testsuites.InitTopologyTestSuite,
		})
	})
})
