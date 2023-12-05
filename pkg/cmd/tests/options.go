// Copyright (c) 2023 ScyllaDB.

package tests

import (
	"flag"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/util/errors"
	kubeframework "k8s.io/kubernetes/test/e2e/framework"
	kubeframeworkconfig "k8s.io/kubernetes/test/e2e/framework/config"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
)

type DeleteTestingNSPolicyType string

var (
	DeleteTestingNSPolicyAlways    DeleteTestingNSPolicyType = "Always"
	DeleteTestingNSPolicyOnSuccess DeleteTestingNSPolicyType = "OnSuccess"
	DeleteTestingNSPolicyNever     DeleteTestingNSPolicyType = "Never"
)

type KubeFrameworkOptions struct {
	ArtifactsDir                 string
	DeleteTestingNSPolicyUntyped string
	DeleteTestingNSPolicy        DeleteTestingNSPolicyType
}

func NewTestFrameworkOptions() KubeFrameworkOptions {
	return KubeFrameworkOptions{
		ArtifactsDir:                 "",
		DeleteTestingNSPolicyUntyped: string(DeleteTestingNSPolicyAlways),
	}
}

func (o *KubeFrameworkOptions) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&o.ArtifactsDir, "artifacts-dir", "", o.ArtifactsDir, "A directory for storing test artifacts. No data is collected until set.")
	cmd.PersistentFlags().StringVarP(&o.DeleteTestingNSPolicyUntyped, "delete-namespace-policy", "", o.DeleteTestingNSPolicyUntyped, fmt.Sprintf("Namespace deletion policy. Allowed values are [%s].", strings.Join(
		[]string{
			string(DeleteTestingNSPolicyAlways),
			string(DeleteTestingNSPolicyNever),
			string(DeleteTestingNSPolicyOnSuccess),
		},
		", ",
	)))

	cmd.PersistentFlags().StringVarP(&kubeframework.TestContext.KubeConfig, "kubeconfig", "", kubeframework.TestContext.KubeConfig, "Path to the kubeconfig file.")

	kubeframeworkconfig.CopyFlags(kubeframeworkconfig.Flags, flag.CommandLine)
	kubeframework.RegisterCommonFlags(kubeframeworkconfig.Flags)
	kubeframework.RegisterClusterFlags(kubeframeworkconfig.Flags)
	flag.Parse()

	kubeframework.AfterReadingAllFlags(&kubeframework.TestContext)
}

func (o *KubeFrameworkOptions) Validate() error {
	var errors []error

	switch p := DeleteTestingNSPolicyType(o.DeleteTestingNSPolicyUntyped); p {
	case DeleteTestingNSPolicyAlways,
		DeleteTestingNSPolicyOnSuccess,
		DeleteTestingNSPolicyNever:
	default:
		errors = append(errors, fmt.Errorf("invalid DeleteTestingNSPolicy: %q", p))
	}

	return apierrors.NewAggregate(errors)
}

func (o *KubeFrameworkOptions) Complete() error {
	o.DeleteTestingNSPolicy = DeleteTestingNSPolicyType(o.DeleteTestingNSPolicyUntyped)

	kubeframework.TestContext.ReportDir = strings.TrimSpace(o.ArtifactsDir)

	switch o.DeleteTestingNSPolicy {
	case DeleteTestingNSPolicyNever:
		kubeframework.TestContext.DeleteNamespace = false
		kubeframework.TestContext.DeleteNamespaceOnFailure = false
	case DeleteTestingNSPolicyAlways:
		kubeframework.TestContext.DeleteNamespace = true
		kubeframework.TestContext.DeleteNamespaceOnFailure = true
	case DeleteTestingNSPolicyOnSuccess:
		kubeframework.TestContext.DeleteNamespace = true
		kubeframework.TestContext.DeleteNamespaceOnFailure = false
	}

	testfiles.AddFileSource(testfiles.RootFileSource{Root: "vendor/k8s.io/kubernetes"})

	return nil
}
