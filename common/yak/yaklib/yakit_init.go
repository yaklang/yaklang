//go:build !ssa2llvm_aot

// The self-contained AOT runtime prunes the optional cli module, so
// AutoInitYakit's start-up call would reach a pruned stub. The AOT runtime
// does not need the Yakit webhook client; skip the init there entirely.
package yaklib

func init() {
	AutoInitYakit()
}
