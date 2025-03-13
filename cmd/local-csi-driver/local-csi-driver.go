// Copyright (c) 2023 ScyllaDB.

package main

import (
	"flag"
	"os"

	cmd "github.com/scylladb/local-csi-driver/pkg/cmd/local-csi-driver"
	"github.com/scylladb/local-csi-driver/pkg/genericclioptions"
	"k8s.io/component-base/cli"
	"k8s.io/component-base/logs/klogflags"
)

func main() {
	klogflags.Init(flag.CommandLine)
	command := cmd.NewLocalDriverCommand(genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	})
	exitCode := cli.Run(command)
	os.Exit(exitCode)
}
