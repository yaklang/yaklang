package rustparser

import (
	"github.com/yaklang/antlr/v4"
)

// RustParserBase is ported from grammars-v4 rust/Go/rust_parser_base.go.
// It only depends on the ANTLR Go target.
type RustParserBase struct {
	*antlr.BaseParser
}

func (p *RustParserBase) NextGT() bool {
	return p.GetInputStream().LA(1) == RustParserGT
}

func (p *RustParserBase) NextLT() bool {
	return p.GetInputStream().LA(1) == RustParserLT
}
