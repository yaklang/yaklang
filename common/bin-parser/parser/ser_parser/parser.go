package ser_parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/yserx"
)

func init() {
	base.RegisterParser("ser", &SerParser{})
}

type SerParser struct {
}

func (s *SerParser) Parse(data *base.BitReader, node *base.Node) error {
	res, err := yserx.ParseJavaSerializedFromReader(data)
	if err != nil {
		return err
	}
	node.Ctx.SetItem("data", res)
	return nil
}
func (s *SerParser) Generate(data any, node *base.Node) error {
	return nil
}
func (s *SerParser) OnRoot(node *base.Node) error {
	return nil
}
