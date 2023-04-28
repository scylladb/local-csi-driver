// Copyright (c) 2023 ScyllaDB.

package limit

import "math"

const (
	MaxLimits = math.MaxUint32 - 1
)

type Limiter interface {
	// NewLimit creates a new limit on provided directory path.
	NewLimit(directory string) (uint32, error)

	// SetLimit sets new limit of capacityBytes on provided limitID.
	SetLimit(limitID uint32, capacityBytes int64) error

	// RemoveLimit removes a limit having limitID.
	RemoveLimit(limitID uint32) error
}
