package parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/ser_parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

var defaultParserRegistration base.Parser

func init() {
	base.RegisterParserFactory("default", func() base.Parser {
		return &stream_parser.DefParser{}
	})
	defaultParserRegistration = base.ParserRegistration("default")
	base.RegisterParser("ser", &ser_parser.SerParser{})
}
