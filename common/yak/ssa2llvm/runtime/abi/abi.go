package abi

// InvokeContext is the single calling-convention object shared between
// LLVM-generated code and the runtime.
//
// Layout (all i64 words, little-endian):
//   [0]  Magic
//   [1]  Version
//   [2]  Kind
//   [3]  Flags
//   [4]  Target (fn ptr or dispatch id)
//   [5]  Argc
//   [6]  Ret
//   [7]  Panic
//   [8]  Reserved0
//   [9]  Reserved1
//   [10...] Args (argc words)
//   [..] Roots (argc words, untagged pointers)
//
// Roots exist because Yak values are represented as i64 in LLVM; for some calls
// we tag pointer-like values (e.g. print/println). The runtime uses Roots as a
// Boehm-GC-visible, untagged pointer list to keep shadow objects alive.

const (
	Magic   uint64 = 0x59414B494E564B31 // "YAKINVK1"
	Version uint64 = 1
)

const (
	KindCallable uint64 = 1
	KindDispatch uint64 = 2
)

const (
	WordMagic    = 0
	WordVersion  = 1
	WordKind     = 2
	WordFlags    = 3
	WordTarget   = 4
	WordArgc     = 5
	WordRet      = 6
	WordPanic    = 7
	WordReserved = 8

	HeaderWords = 10
)
