// Copyright (C) 2021 ScyllaDB

package slices_test

import (
	"testing"

	"github.com/scylladb/k8s-local-volume-provisioner/pkg/util/slices"
)

func TestContains(t *testing.T) {
	tcs := []struct {
		name     string
		array    []int
		element  int
		expected bool
	}{
		{
			name:     "empty slice",
			array:    []int{},
			element:  1,
			expected: false,
		},
		{
			name:     "slice with element",
			array:    []int{0, 1, 2, 3},
			element:  1,
			expected: true,
		},
		{
			name:     "slice without element",
			array:    []int{0, 1, 2, 3},
			element:  123,
			expected: false,
		},
	}
	for i := range tcs {
		test := tcs[i]
		t.Run(test.name, func(t *testing.T) {
			got := slices.Contains(test.array, test.element)
			if test.expected != got {
				t.Errorf("expected %v got %v", test.expected, got)
			}
		})
	}
}
