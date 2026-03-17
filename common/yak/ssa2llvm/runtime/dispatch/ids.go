package dispatch

// FuncID is the stable identifier passed from LLVM-generated code into the
// runtime dispatcher.
type FuncID int64

// DispatcherSymbol is the single exported runtime entry used to invoke
// runtime-dispatched functions from LLVM-generated code.
//
// The design goal is to keep the final native binary small and harder to
// reverse by minimizing the amount of readable exported symbols.
const DispatcherSymbol = "yak_runtime_dispatch"

// NOTE: IDs must stay stable once published, otherwise old binaries or
// cached IR will call wrong functions.
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

	// runtime control
	IDWaitAllAsyncCallFinish FuncID = 12
)
