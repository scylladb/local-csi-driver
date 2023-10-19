// Copyright (c) 2023 ScyllaDB.

package driver

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/cmdutil"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/limit"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/limit/xfs"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/volume"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/genericclioptions"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/signals"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/util/fs"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/version"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/errors"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"
)

type LocalDriverOptions struct {
	DriverName string
	Listen     string
	VolumesDir string
	NodeName   string
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

	cmd.Flags().StringVarP(&o.DriverName, "driver-name", "", o.DriverName, "Name of the driver used for registration.")
	cmd.Flags().StringVarP(&o.VolumesDir, "volumes-dir", "", o.VolumesDir, "Path to directory where driver provisions the volumes.")
	cmd.Flags().StringVarP(&o.Listen, "listen", "", o.Listen, "Path to the driver socket.")
	cmd.Flags().StringVarP(&o.NodeName, "node-name", "", o.NodeName, "Name of the node for which the driver is responsible of.")

	cmdutil.InstallKlog(cmd)

	return cmd
}

func (o *LocalDriverOptions) Validate() error {
	var errs []error

	if len(o.DriverName) == 0 {
		errs = append(errs, fmt.Errorf("driver-name cannot be empty"))
	}

	if len(o.Listen) == 0 {
		errs = append(errs, fmt.Errorf("listen cannot be empty"))
	}

	if len(o.VolumesDir) == 0 {
		errs = append(errs, fmt.Errorf("volumes-dir cannot be empty"))
	}

	_, err := os.Stat(o.VolumesDir)
	if err != nil {
		errs = append(errs, fmt.Errorf("can't stat volumes-dir: %w", err))
	}

	if len(o.NodeName) == 0 {
		errs = append(errs, fmt.Errorf("node-name cannot be empty"))
	}

	err = errors.NewAggregate(errs)
	if err != nil {
		return err
	}

	return nil
}

func (o *LocalDriverOptions) Complete() error {
	return nil
}

func (o *LocalDriverOptions) Run(streams genericclioptions.IOStreams, cmd *cobra.Command) error {
	klog.V(1).InfoS("Driver started", "command", cmd.CommandPath(), "version", version.Get())
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

func (o *LocalDriverOptions) run(ctx context.Context, _ genericclioptions.IOStreams) error {
	sm, err := volume.NewStateManager(o.VolumesDir)
	if err != nil {
		return fmt.Errorf("can't create state manager: %w", err)
	}

	volumeFsType, err := fs.GetFilesystem(o.VolumesDir)
	if err != nil {
		return fmt.Errorf("can't get filesystem of volume dir %q: %w", o.VolumesDir, err)
	}

	var limiter limit.Limiter = &limit.NoopLimiter{}

	switch volumeFsType {
	case "xfs":
		xl, err := xfs.NewXFSLimiter(o.VolumesDir, sm.GetVolumes())
		if err != nil {
			return fmt.Errorf("can't create XFS limiter: %w", err)
		}
		limiter = xl
	default:
		return fmt.Errorf("unsupported volumes dir filesystem %q", volumeFsType)
	}

	vm, err := volume.NewVolumeManager(o.VolumesDir, sm, volume.WithLimiter(limiter))
	if err != nil {
		return fmt.Errorf("can't create driver: %w", err)
	}

	if err := os.Remove(o.Listen); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("can't remove file at %q: %w", o.Listen, err)
	}

	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "unix", o.Listen)
	if err != nil {
		return fmt.Errorf("can't listen on %q using unix protocol: %w", o.Listen, err)
	}

	defer func() {
		cleanupErr := os.Remove(o.Listen)
		if cleanupErr != nil {
			klog.ErrorS(cleanupErr, "Failed to cleanup the listen socket", "socket", o.Listen)
		}
	}()

	d := driver.NewDriver(o.DriverName, version.Get().String(), o.NodeName, vm)

	server := grpc.NewServer()

	csi.RegisterIdentityServer(server, d)
	csi.RegisterControllerServer(server, d)
	csi.RegisterNodeServer(server, d)

	var eg errgroup.Group

	eg.Go(func() error {
		klog.InfoS("Listening for connections", "address", listener.Addr())
		err = server.Serve(listener)
		if err != nil && err != grpc.ErrServerStopped {
			return fmt.Errorf("can't serve: %w", err)
		}

		return nil
	})

	eg.Go(func() error {
		<-ctx.Done()

		server.GracefulStop()

		return nil
	})

	err = eg.Wait()
	if err != nil {
		return err
	}

	return nil
}
