package core

import "context"

type ParserInfo struct {
	Kind      Architecture `json:"kind"`
	Name      string       `json:"name"`
	Available bool         `json:"available"`
}

// Parser translates one instruction set from machine code into LowIR. It does
// not return assembly text. Kind aligns a parser with an Architecture,
// independent of the file format. Available reports whether Parse can decode
// instructions.
type Parser interface {
	Kind() Architecture
	Name() string
	Available() bool
	Check(Target) error
	Parse(context.Context, ParseRequest) (*ParseResult, error)
}

// ParseRequest describes a contiguous code region. Address is the image address
// of Code[0]. Code is borrowed read-only for Parse; the result must own any
// bytes it retains.
type ParseRequest struct {
	Target  Target
	Address uint64
	Code    []byte
}

type ParseResult struct {
	IR          *LowIR
	Diagnostics []Diagnostic
}
