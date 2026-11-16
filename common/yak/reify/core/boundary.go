package core

// BoundarySource identifies how a function boundary was discovered.
type BoundarySource string

const (
	BoundarySymbol        BoundarySource = "symbol"
	BoundaryEHFrame       BoundarySource = "eh_frame"
	BoundaryFunctionStart BoundarySource = "function_start"
	BoundaryInitArray     BoundarySource = "init_array"
	BoundaryPLT           BoundarySource = "plt"
	BoundaryEntry         BoundarySource = "entry"
	BoundaryCall          BoundarySource = "call_target"
	BoundaryScan          BoundarySource = "scan"
)

// FunctionBoundary is one recovered function range. Size is resolved after all
// sources have been merged; a zero size means the range ends at the next
// discovered boundary or the executable section end.
type FunctionBoundary struct {
	Address uint64         `json:"address"`
	Size    uint64         `json:"size,omitempty"`
	Name    string         `json:"name,omitempty"`
	Source  BoundarySource `json:"source"`
	// Mode names the instruction-set execution mode this function uses. ARM
	// carries both the ARM and the Thumb encoding in one image, so the mode is
	// a property of the function, not of the image.
	Mode string `json:"mode,omitempty"`
}

// BoundaryStats reports discovery counts. Duplicates is the number of
// addresses that were reported by more than one source.
type BoundaryStats struct {
	Symbol        int `json:"symbol"`
	EHFrame       int `json:"eh_frame"`
	FunctionStart int `json:"function_start"`
	InitArray     int `json:"init_array"`
	PLT           int `json:"plt"`
	Entry         int `json:"entry"`
	Call          int `json:"call_target"`
	Scan          int `json:"scan"`
	Duplicates    int `json:"duplicates"`
}

// BoundaryHints carries format facts that help an architecture parser find
// function boundaries. Loaders fill it; they never decode instructions.
type BoundaryHints struct {
	// ExecSections lists file-backed executable ranges in image addresses.
	ExecSections []Region

	// OpdDescriptors maps a PowerPC ELFv1 function descriptor to the code address
	// it holds. A call reaches the descriptor, and the symbol names the code.
	OpdDescriptors map[uint64]uint64
	// LocalEntryOffset is the distance from a function symbol to the entry a
	// local call targets. PowerPC ELFv2 places it eight bytes after the global
	// entry, past the code that computes the table of contents.
	LocalEntryOffset uint64
	// FunctionStarts lists exact function entry addresses from format metadata
	// (Mach-O LC_FUNCTION_STARTS, PE exports/exception data).
	FunctionStarts []uint64

	// EHFrame is the raw .eh_frame section contents and its image address.
	EHFrame     []byte
	EHFrameAddr uint64

	// InitTargets holds resolved .init_array/.fini_array function pointers.
	InitTargets []uint64

	// PLTTargets holds PLT stub addresses paired with their symbol names.
	PLTTargets []PLTEntry

	// GOTRelocs maps a GOT slot address to the address it resolves to.
	GOTRelocs map[uint64]uint64

	// ModeRegions lists address ranges whose instruction encoding the file
	// format records. ARM images mix ARM and Thumb code, and the linker writes
	// the PLT and .init sections in ARM mode even when .text is Thumb.
	ModeRegions []ModeRegion
}

// ModeRegion is one address range with a fixed execution mode.
type ModeRegion struct {
	Region
	Mode string
}

// PLTEntry is one PLT stub discovered by a loader.
type PLTEntry struct {
	Address uint64
	Name    string
}

// BoundaryResult is an architecture parser's function discovery output.
type BoundaryResult struct {
	Functions []FunctionBoundary
	Stats     BoundaryStats
}

// BoundaryDiscoverer is an optional Parser capability. Parsers that can split a
// code region into functions implement it; asm2lowir calls it before parsing.
type BoundaryDiscoverer interface {
	DiscoverBoundaries(reader *ByteReader, hints BoundaryHints, image *Image) (BoundaryResult, error)
}
