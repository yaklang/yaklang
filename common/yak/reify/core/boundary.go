package core

// BoundarySource identifies how a function boundary was discovered.
type BoundarySource string

const (
	BoundarySymbol    BoundarySource = "symbol"
	BoundaryEHFrame   BoundarySource = "eh_frame"
	BoundaryInitArray BoundarySource = "init_array"
	BoundaryPLT       BoundarySource = "plt"
	BoundaryEntry     BoundarySource = "entry"
	BoundaryCall      BoundarySource = "call_target"
	BoundaryScan      BoundarySource = "scan"
)

// FunctionBoundary is one recovered function range. Size is resolved after all
// sources have been merged; a zero size means the range ends at the next
// discovered boundary or the executable section end.
type FunctionBoundary struct {
	Address uint64         `json:"address"`
	Size    uint64         `json:"size,omitempty"`
	Name    string         `json:"name,omitempty"`
	Source  BoundarySource `json:"source"`
}

// BoundaryStats reports discovery counts. Duplicates is the number of
// addresses that were reported by more than one source.
type BoundaryStats struct {
	Symbol     int `json:"symbol"`
	EHFrame    int `json:"eh_frame"`
	InitArray  int `json:"init_array"`
	PLT        int `json:"plt"`
	Entry      int `json:"entry"`
	Call       int `json:"call_target"`
	Scan       int `json:"scan"`
	Duplicates int `json:"duplicates"`
}

// BoundaryHints carries format facts that help an architecture parser find
// function boundaries. Loaders fill it; they never decode instructions.
type BoundaryHints struct {
	// ExecSections lists file-backed executable ranges in image addresses.
	ExecSections []Region

	// EHFrame is the raw .eh_frame section contents and its image address.
	EHFrame     []byte
	EHFrameAddr uint64

	// InitTargets holds resolved .init_array/.fini_array function pointers.
	InitTargets []uint64

	// PLTTargets holds PLT stub addresses paired with their symbol names.
	PLTTargets []PLTEntry

	// GOTRelocs maps a GOT slot address to the address it resolves to.
	GOTRelocs map[uint64]uint64
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
