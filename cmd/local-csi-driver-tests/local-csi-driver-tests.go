// Copyright (c) 2023 ScyllaDB.

package main

import (
	"os"

	cmd "github.com/scylladb/local-csi-driver/pkg/cmd/tests"
	"github.com/scylladb/local-csi-driver/pkg/genericclioptions"
	"k8s.io/component-base/cli"
	_ "k8s.io/component-base/logs/testinit"
)

func main() {
	command := cmd.NewTestsCommand(genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	})
	exitCode := cli.Run(command)
	os.Exit(exitCode)
}
