package ser_parser

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/yserx"
)

func init() {
	base.RegisterParser("ser", &SerParser{})
}

type SerParser struct {
	base.BaseParser
}

func (s *SerParser) Parse(data *base.BitReader, node *base.Node) error {
	res, err := yserx.ParseSingleJavaSerializedFromReader(data)
	if err != nil {
		return err
	}
	if len(res) == 0 {
		node.Cfg.SetItem("ser_res", nil)
	} else {
		node.Cfg.SetItem("ser_res", res[0])
	}
	return nil
}
func (s *SerParser) Generate(data any, node *base.Node) error {
	v, ok := base.GetValueByNode(data, node)
	if !ok {
		return fmt.Errorf("get node %s value error", node.Name)
	}
	var byts []byte
	if v == nil {
		byts = yserx.MarshalJavaObjects()
	} else {
		d, ok := v.(yserx.JavaSerializable)
		if !ok {
			return errors.New("data is not JavaSerializable")
		}
		byts = yserx.MarshalJavaObjects(d)
	}

	length := uint64(len(byts) * 8)
	write := node.Ctx.GetItem("def_writer").(func(bytes []byte, u uint64) ([2]uint64, error))
	_, err := write(byts, length)
	return err
}
func (s *SerParser) Result(node *base.Node) (*base.NodeValue, error) {
	return &base.NodeValue{
		Name:  node.Name,
		Value: node.Cfg.GetItem("ser_res"),
	}, nil
}
func (s *SerParser) OnRoot(node *base.Node) error {
	return nil
}
