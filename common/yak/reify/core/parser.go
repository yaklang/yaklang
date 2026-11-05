package core

import "context"

type ParserInfo struct {
	Kind      Architecture `json:"kind"`
	Name      string       `json:"name"`
	Available bool         `json:"available"`
}

// Parser translates one instruction set from machine code into LowIR. Parse
// does not return assembly text; a parser may also implement Disassembler.
// Kind aligns a parser with an Architecture, independent of the file format.
// Available reports whether Parse can decode instructions.
type Parser interface {
	Kind() Architecture
	Name() string
	Available() bool
	Check(Target) error
	Parse(context.Context, ParseRequest) (*ParseResult, error)
}

// ParseRequest describes one code region. Address is the image address of
// Reader offset zero. Reader is a borrowed, region-bounded view; parsers must
// use its relative Bytes/ReadAt methods and must not close it. The reader owns
// byte-order handling, so parsers do not interpret Target.ByteOrder.
type ParseRequest struct {
	Target  Target
	Address uint64
	Reader  *ByteReader
}

type ParseResult struct {
	IR          *LowIR
	Diagnostics []Diagnostic
}

// Disassembly is the textual form of one decoded code region. Text contains
// one instruction per line; callers add their own headers. Instructions is the
// number of decoded instructions.
type Disassembly struct {
	Text         string       `json:"text"`
	Instructions int          `json:"instructions"`
	Diagnostics  []Diagnostic `json:"diagnostics,omitempty"`
}

// Disassembler is an optional Parser capability. Parsers without a textual
// disassembly return ErrNoDisassembler through Artifact.
type Disassembler interface {
	Disassemble(context.Context, ParseRequest) (*Disassembly, error)
}
