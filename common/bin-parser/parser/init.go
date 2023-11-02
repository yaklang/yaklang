package parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/ser_parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func init() {
	base.RegisterParser("default", &stream_parser.DefParser{})
	base.RegisterParser("ser", &ser_parser.SerParser{})
}
