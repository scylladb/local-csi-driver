// Copyright (c) 2023 ScyllaDB.

package ginkgo

import (
	g "github.com/onsi/ginkgo/v2"
)

// LocalCSIDriverLabelName names the label marking specs that test/e2e opts into
// running. Suites filter on it to skip the unrelated specs that upstream
// Kubernetes e2e packages register into our suite.
const LocalCSIDriverLabelName = "local-csi-driver"

// LocalCSIDriverLabel is the LocalCSIDriverLabelName decorator. Every top level
// container in test/e2e has to carry it, otherwise its specs won't run.
var LocalCSIDriverLabel = g.Label(LocalCSIDriverLabelName)
