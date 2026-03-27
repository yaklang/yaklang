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
	Magic   uint64 = 0x59414B4354585631 // "YAKCTXV1"
	Version uint64 = 1
)

const (
	KindCallable uint64 = 1
	KindDispatch uint64 = 2
)

const (
	InvokeSymbol    = "yak_runtime_invoke"
	MakeSliceSymbol = "yak_runtime_make_slice"
)

const (
	FlagAsync              uint64 = 1 << 0
	FlagPanicTaggedPointer uint64 = 1 << 1
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

// FuncID is the stable identifier passed from LLVM-generated code into the
// runtime builtin dispatcher.
type FuncID int64

// NOTE: IDs must stay stable once published, otherwise old binaries or cached
// IR will call wrong functions.
const (
	// poc
	IDPocTimeout           FuncID = 1
	IDPocGet               FuncID = 2
	IDPocGetHTTPPacketBody FuncID = 3

	// os
	IDOsGetenv FuncID = 4

	// builtin printing
	IDPrint   FuncID = 5
	IDPrintf  FuncID = 6
	IDPrintln FuncID = 7

	// yakit logging (minimal)
	IDYakitInfo  FuncID = 8
	IDYakitWarn  FuncID = 9
	IDYakitDebug FuncID = 10
	IDYakitError FuncID = 11

	// sync constructors
	IDSyncNewWaitGroup      FuncID = 12
	IDSyncNewSizedWaitGroup FuncID = 13
	IDSyncNewLock           FuncID = 14
	IDSyncNewMutex          FuncID = 15
	IDSyncNewRWMutex        FuncID = 16

	// runtime shadow-object method dispatch
	IDRuntimeShadowMethod FuncID = 17

	// builtin slice helpers
	IDAppend FuncID = 18

	// additional sync constructors
	IDSyncNewMap  FuncID = 19
	IDSyncNewOnce FuncID = 20
	IDSyncNewPool FuncID = 21
	IDSyncNewCond FuncID = 22
)

type SliceElemKind int64

const (
	SliceElemAny SliceElemKind = iota
	SliceElemInt64
	SliceElemString
	SliceElemByte
	SliceElemBool
)
