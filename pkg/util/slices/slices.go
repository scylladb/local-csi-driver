// Copyright (c) 2023 ScyllaDB.

package slices

func Contains[T comparable](arr []T, elem T) bool {
	for _, e := range arr {
		if e == elem {
			return true
		}
	}
	return false
}

func Unique[T comparable](slice []T) []T {
	m := make(map[T]struct{}, len(slice))
	for _, i := range slice {
		m[i] = struct{}{}
	}

	u := make([]T, 0, len(slice))
	for i := range m {
		u = append(u, i)
	}

	return u
}
