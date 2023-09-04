package utils

import "golang.org/x/exp/slices"

func Remove[T comparable](slice []T, s T) []T {
	if index := slices.Index(slice, s); index > -1 {
		return append(slice[:index], slice[index+1:]...)
	}
	return slice
}
