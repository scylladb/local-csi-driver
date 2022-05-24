// Copyright (C) 2021 ScyllaDB

package localdriver

import (
	"net"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/local"
	"google.golang.org/grpc"
	"k8s.io/mount-utils"
)

func TestSanity(t *testing.T) {
	o.RegisterFailHandler(g.Fail)
	g.RunSpecs(t, "Sanity Suite")
}

var _ = g.Describe("Local CSI Driver", func() {
	var (
		config  sanity.TestConfig
		wg      *sync.WaitGroup
		workDir string
		server  *grpc.Server
	)

	g.BeforeEach(func() {
		dir, err := os.MkdirTemp(os.TempDir(), "sanity-csi-")
		o.Expect(err).ToNot(o.HaveOccurred())
		workDir = dir

		endpoint := "unix:" + filepath.Join(dir, "csi.sock")

		config = sanity.NewTestConfig()
		config.Address = endpoint
		config.TargetPath = path.Join(dir, "target")
		config.StagingPath = path.Join(dir, "staging")

		volumesDir := path.Join(dir, "volumes")

		o.Expect(os.Mkdir(volumesDir, 0700)).To(o.Succeed())

		d, err := local.NewDriver(
			"local-csi-driver",
			"0.0.0-sanity-tests",
			volumesDir,
			"node-name",
			local.WithMounter(mount.NewFakeMounter(nil)),
			local.WithLimiter(local.DefaultFS, &local.NoopLimiter{}),
		)

		listener, err := net.Listen("unix", filepath.Join(dir, "csi.sock"))
		o.Expect(err).ToNot(o.HaveOccurred())

		server = grpc.NewServer()

		csi.RegisterIdentityServer(server, d)
		csi.RegisterControllerServer(server, d)
		csi.RegisterNodeServer(server, d)

		wg = &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()

			_ = server.Serve(listener)
			_ = listener.Close()
		}()
	})

	g.AfterEach(func() {
		server.GracefulStop()
		wg.Wait()

		o.Expect(os.RemoveAll(workDir)).To(o.Succeed())
	})

	g.Describe("CSI sanity", func() {
		sanity.GinkgoTest(&config)
	})
})
