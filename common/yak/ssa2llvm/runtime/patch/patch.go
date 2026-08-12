// Package patch provides compile-time post-processing of the embedded libyak.a
// so lld --gc-sections can drop yaklib modules a script does not use.
//
// At ssa2llvm build time (scripts/build_yaklib.sh), elfsplit moves each
// module's full chain (_cgoexp wrapper, register function, init, and module
// code) into a dedicated .modtext.<module> ELF section and removes the
// inittask -> module.init relocation.
//
// At ssa2llvm compile time, for modules the script does NOT use, we:
//  1. zero every relocation (in .rela.*) that references a .modtext.<module>
//     symbol, so no kept section points into the module's code;
//  2. set the module's ..inittask.state to 2 (already initialized), so Go's
//     runtime doInit1 returns without dereferencing the now-orphaned init
//     function pointer.
//
// lld then runs with --gc-sections and drops the unreferenced .modtext
// sections, shrinking the final static binary.
//
// The interface is platform-neutral; the concrete implementation is selected
// by build tags (linux.go currently, more platforms later).
package patch

import "fmt"

// Request describes one compile-time patch of the embedded libyak archive.
type Request struct {
	// ArchivePath is the path to the released libyak.a to patch in place.
	ArchivePath string
	// UsedModules lists yaklib modules the current script actually uses.
	// All other registered modules are candidates for removal.
	UsedModules []string
	// AllModules lists every yaklib module registered in the archive. When
	// empty, the implementation derives it from the archive's .modtext sections.
	AllModules []string
}

// Patch patches the libyak archive so unused modules can be collected.
// It is safe to call even when nothing needs patching (no-op).
func Patch(req Request) error {
	if req.ArchivePath == "" {
		return fmt.Errorf("patch: empty archive path")
	}
	impl, err := newImplementation()
	if err != nil {
		return err
	}
	return impl.patch(req)
}
