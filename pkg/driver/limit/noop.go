// Copyright (c) 2023 ScyllaDB.

package limit

type NoopLimiter struct {
}

var _ Limiter = &NoopLimiter{}

func (l *NoopLimiter) NewLimit(directory string) (uint32, error) {
	return 0, nil
}

func (l *NoopLimiter) SetLimit(limitID uint32, capacityBytes int64) error {
	return nil
}

func (l *NoopLimiter) RemoveLimit(limitID uint32) error {
	return nil
}
