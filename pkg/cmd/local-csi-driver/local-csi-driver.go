// Copyright (C) 2021 ScyllaDB

package local_csi_driver

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/cmdutil"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/local"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/genericclioptions"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/signals"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/version"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/errors"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"
)

type LocalDriverOptions struct {
	DriverName string
	Endpoint   string
	VolumesDir string
	NodeName   string
}

func (o *LocalDriverOptions) Validate() error {
	var errs []error

	if len(o.Endpoint) == 0 {
		errs = append(errs, fmt.Errorf("endpoint cannot be empty"))
	}

	if _, err := o.parseUnixEndpoint(o.Endpoint); err != nil {
		errs = append(errs, err)
	}

	if len(o.VolumesDir) == 0 {
		errs = append(errs, fmt.Errorf("volumesDir cannot be empty"))
	}

	if len(o.NodeName) == 0 {
		errs = append(errs, fmt.Errorf("nodeName cannot be empty"))
	}

	return errors.NewAggregate(errs)
}

func (o *LocalDriverOptions) Complete() error {
	return nil
}

func (o *LocalDriverOptions) Run(streams genericclioptions.IOStreams, cmd *cobra.Command) error {
	klog.V(1).Infof("%q version %q", cmd.CommandPath(), version.Get())
	cliflag.PrintFlags(cmd.Flags())

	stopCh := signals.StopChannel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-stopCh
		cancel()
	}()

	return o.run(ctx, streams)
}

func NewLocalDriverOptions(_ genericclioptions.IOStreams) *LocalDriverOptions {
	return &LocalDriverOptions{
		DriverName: "local.csi.scylladb.com",
	}
}

func NewLocalDriverCommand(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewLocalDriverOptions(streams)

	cmd := &cobra.Command{
		Use:   "local-csi-driver",
		Short: "Run the Local CSI Driver",
		Long:  `Run the Local CSI Driver.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := o.Validate()
			if err != nil {
				return err
			}

			err = o.Complete()
			if err != nil {
				return err
			}

			err = o.Run(streams, cmd)
			if err != nil {
				return err
			}

			return nil
		},

		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.Flags().StringVarP(&o.DriverName, "driver-name", "", o.DriverName, "Name under which driver is registered")
	cmd.Flags().StringVarP(&o.VolumesDir, "volumes-dir", "", o.VolumesDir, "Path to directory on filesystem where driver provisions the volumes.")
	cmd.Flags().StringVarP(&o.Endpoint, "endpoint", "", o.Endpoint, "Unix endpoint where driver should listen")
	cmd.Flags().StringVarP(&o.NodeName, "node-name", "", o.NodeName, "Name of the node for which the driver is responsible of")

	cmdutil.InstallKlog(cmd)

	return cmd
}

func (o *LocalDriverOptions) run(ctx context.Context, _ genericclioptions.IOStreams) error {
	driver, err := local.NewDriver(o.DriverName, version.Get().String(), o.VolumesDir, o.NodeName)
	if err != nil {
		return fmt.Errorf("failed to create driver: %w", err)
	}

	listener, cleanup, err := o.makeListener(o.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	server := grpc.NewServer()

	csi.RegisterIdentityServer(server, driver)
	csi.RegisterControllerServer(server, driver)
	csi.RegisterNodeServer(server, driver)

	klog.Infof("Listening for connections on address: %#v", listener.Addr())

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := server.Serve(listener); err != nil && err != grpc.ErrServerStopped {
			klog.Errorf("Error occurred during server serving: %v", err)
		}

		if err := cleanup(); err != nil {
			klog.Errorf("Error occurred during listener cleanup: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		<-ctx.Done()

		server.GracefulStop()
	}()

	wg.Wait()

	return nil
}

func (o *LocalDriverOptions) makeListener(endpoint string) (net.Listener, func() error, error) {
	addr, err := o.parseUnixEndpoint(endpoint)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() error {
		return os.Remove(addr)
	}

	l, err := net.Listen("unix", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("can't listen on addr %q using unix protocol: %w", addr, err)
	}

	return l, cleanup, err
}

func (o *LocalDriverOptions) parseUnixEndpoint(endpoint string) (string, error) {
	if strings.HasPrefix(strings.ToLower(endpoint), "unix://") {
		addr := strings.TrimPrefix(endpoint, "unix://")
		if !strings.HasPrefix(addr, "/") {
			addr = "/" + addr
		}

		return addr, nil
	}

	return "", fmt.Errorf("unsupported endpoint %q, must be an absolute path or have 'unix://' prefix", endpoint)
}
