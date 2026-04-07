// Package llvminterop provides the integration layer between ssa2llvm's
// compilation pipeline and external LLVM tools (opt, plugins, adapters).
// It does not belong in obfuscation/; it is a "compiler chain extension
// mechanism" that can invoke arbitrary LLVM passes on the generated IR.
package llvminterop
