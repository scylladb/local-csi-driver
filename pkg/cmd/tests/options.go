package tests

import (
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	kubeframework "k8s.io/kubernetes/test/e2e/framework"
	kubeframeworkconfig "k8s.io/kubernetes/test/e2e/framework/config"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
)

type KubeFrameworkOptions struct {
	ReportDir string
}

func NewTestFrameworkOptions() KubeFrameworkOptions {
	return KubeFrameworkOptions{}
}

func (o *KubeFrameworkOptions) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&kubeframework.TestContext.ReportDir, "report-dir", "", kubeframework.TestContext.ReportDir, "A directory for storing test reports. No data is collected until set.")
	cmd.PersistentFlags().BoolVarP(&kubeframework.TestContext.DeleteNamespaceOnFailure, "delete-namespace-on-failure", "", kubeframework.TestContext.DeleteNamespaceOnFailure, "Controls if test namespace is deleted when test fails")
	cmd.PersistentFlags().StringVarP(&kubeframework.TestContext.KubeConfig, "kubeconfig", "", kubeframework.TestContext.KubeConfig, "Path to the kubeconfig file.")

	// k8s.io/kubernetes/test/e2e/framework requires env KUBECONFIG to be set
	// it does not fall back to defaults
	if os.Getenv(clientcmd.RecommendedConfigPathEnvVar) == "" {
		os.Setenv(clientcmd.RecommendedConfigPathEnvVar, filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	}

	kubeframeworkconfig.CopyFlags(kubeframeworkconfig.Flags, flag.CommandLine)
	kubeframework.RegisterCommonFlags(kubeframeworkconfig.Flags)
	kubeframework.RegisterClusterFlags(kubeframeworkconfig.Flags)
	flag.Parse()

	kubeframework.AfterReadingAllFlags(&kubeframework.TestContext)
}

func (o *KubeFrameworkOptions) Validate() error {
	return nil
}

func (o *KubeFrameworkOptions) Complete() error {
	// Trim spaces so we can reason later if the dir is set or not
	kubeframework.TestContext.ReportDir = strings.TrimSpace(kubeframework.TestContext.ReportDir)

	testfiles.AddFileSource(testfiles.RootFileSource{Root: "vendor/k8s.io/kubernetes"})

	o.ReportDir = kubeframework.TestContext.ReportDir
	return nil
}
