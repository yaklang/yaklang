package ssautil

import (
	"fmt"
	"sort"
)

func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedVersionedKeys[T versionedValue, V any](m map[VersionedIF[T]]V) []VersionedIF[T] {
	keys := make([]VersionedIF[T], 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return versionedLess(keys[i], keys[j])
	})
	return keys
}

func versionedLess[T versionedValue](left, right VersionedIF[T]) bool {
	if left == right {
		return false
	}
	if left == nil {
		return true
	}
	if right == nil {
		return false
	}
	if left.GetName() != right.GetName() {
		return left.GetName() < right.GetName()
	}
	if left.GetGlobalIndex() != right.GetGlobalIndex() {
		return left.GetGlobalIndex() < right.GetGlobalIndex()
	}
	if left.GetVersion() != right.GetVersion() {
		return left.GetVersion() < right.GetVersion()
	}
	if left.GetId() != right.GetId() {
		return left.GetId() < right.GetId()
	}
	leftCaptured := left.GetCaptured()
	rightCaptured := right.GetCaptured()
	if leftCaptured != nil && rightCaptured != nil {
		if leftCaptured.GetGlobalIndex() != rightCaptured.GetGlobalIndex() {
			return leftCaptured.GetGlobalIndex() < rightCaptured.GetGlobalIndex()
		}
		if leftCaptured.GetVersion() != rightCaptured.GetVersion() {
			return leftCaptured.GetVersion() < rightCaptured.GetVersion()
		}
	}
	return fmt.Sprintf("%p", left) < fmt.Sprintf("%p", right)
}
