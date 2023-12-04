// Copyright (c) 2023 ScyllaDB.

//go:build tools

// Force go mod to download and vendor code that isn't depended upon.
package dependencymagnet

import (
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	_ "k8s.io/kubernetes/cmd/kubectl"
)
