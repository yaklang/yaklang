package utils

import "github.com/samber/lo"

func ContainsAny[T comparable](s []T, vals ...T) bool {
	for _, v := range vals {
		if lo.Contains(s, v) {
			return true
		}
	}
	return false
}

func ContainsAll[T comparable](s []T, vals ...T) bool {
	for _, val := range vals {
		if !lo.Contains(s, val) {
			return false
		}
	}

	return true
}
