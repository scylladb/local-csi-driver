// Copyright (c) 2023 ScyllaDB.

package main

import (
	"flag"
	"fmt"
	"os"

	cmd "github.com/scylladb/local-csi-driver/pkg/cmd/local-csi-driver"
	"github.com/scylladb/local-csi-driver/pkg/genericclioptions"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(flag.CommandLine)
	err := flag.Set("logtostderr", "true")
	if err != nil {
		panic(err)
	}
	defer klog.Flush()

	streams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	command := cmd.NewLocalDriverCommand(streams)
	err = command.Execute()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
