package tests

import (
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/cmdutil"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/genericclioptions"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"
)

const (
	EnvVarPrefix = "LOCAL_CSI_DRIVER_TESTS_"
)

func NewTestsCommand(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use: "local-csi-driver-tests",
		Long: templates.LongDesc(`
		CSI Driver tests

		This command verifies behavior of an CSI Driver by running remote tests on a Kubernetes cluster.
		`),

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.ReadFlagsFromEnv(EnvVarPrefix, cmd)
		},

		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(NewRunCommand(streams))

	cmdutil.InstallKlog(cmd)

	return cmd
}
