package utils

import "golang.org/x/exp/slices"

func RemoveSliceItem[T comparable](slice []T, s T) []T {
	if index := slices.Index(slice, s); index > -1 {
		return append(slice[:index], slice[index+1:]...)
	}
	return slice
}

func InsertSliceItem[T comparable](slices []T, e T, index int) []T {
	if index > len(slices) {
		return slices
	}
	slices = append(slices, e)
	copy(slices[index+1:], slices[index:])
	slices[index] = e
	return slices
}

func ReplaceSliceItem[T comparable](s []T, t T, to T) []T {
	if index := slices.Index(s, t); index > -1 {
		s[index] = to
	}
	return s
}

func AppendSliceItemWhenNotExists[T comparable](s []T, t T) []T {
	if slices.Contains(s, t) {
		return s
	}
	return append(s, t)
}
