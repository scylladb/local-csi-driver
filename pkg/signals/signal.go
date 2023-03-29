// Copyright (c) 2023 ScyllaDB.

package signals

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"k8s.io/klog/v2"
)

var (
	stopChannel = make(chan struct{})
	once        sync.Once

	shutdownSignals = []os.Signal{syscall.SIGINT, syscall.SIGABRT, syscall.SIGTERM}
)

func setupStopChannel() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		s := <-c
		klog.InfoS("Received shutdown signal; shutting down...", "signal", s)
		close(stopChannel)
		<-c
		klog.InfoS("Received second shutdown signal; exiting...", "signal", s)
		// Second signal, exit directly.
		os.Exit(1)
	}()
}

func StopChannel() (stopCh <-chan struct{}) {
	once.Do(setupStopChannel)
	return stopChannel
}
