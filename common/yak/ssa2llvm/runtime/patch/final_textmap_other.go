//go:build !linux

package patch

// SortFinalTextMap is a no-op on non-Linux hosts for now; the self-contained
// ssa2llvm link path is currently Linux-only.
func SortFinalTextMap(path string) error {
	return nil
}

// MissingRetainedModules is a no-op on non-Linux hosts for now.
func MissingRetainedModules(path string) ([]string, error) {
	return nil, nil
}
