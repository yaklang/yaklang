package dispatch

// FuncID is the stable identifier passed from LLVM-generated code into the
// runtime dispatcher.
type FuncID int64

// DispatcherSymbol is the single exported runtime entry used to invoke
// yaklang standard library functions from LLVM-generated code.
//
// The design goal is to keep the final native binary small and harder to
// reverse by minimizing the amount of readable exported symbols.
const DispatcherSymbol = "yak_std_call"

// NOTE: IDs must stay stable once published, otherwise old binaries or
// cached IR will call wrong functions.
const (
	// poc
	IDPocTimeout           FuncID = 1
	IDPocGet               FuncID = 2
	IDPocGetHTTPPacketBody FuncID = 3

	// os
	IDOsGetenv FuncID = 4
)
