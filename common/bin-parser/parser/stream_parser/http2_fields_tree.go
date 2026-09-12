package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func parseHTTP2Fields(node *base.Node, process func(*base.Node) (func(bool), error), mode string) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits < 72 || bits%8 != 0 || bits > http2FieldsMaxBytes*8 {
		return fmt.Errorf("http2-fields: explicit 9..1048576 byte boundary required")
	}
	if mode != "frame" && mode != "client" && mode != "server" {
		return fmt.Errorf("http2-fields: unknown profile")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "HTTP2 staged wire", "raw", node.Ctx)
	if err != nil {
		return err
	}
	raw.Cfg.SetItem(CfgParent, node)
	raw.Cfg.SetItem(CfgLength, bits)
	finish, err := process(raw)
	committed := false
	if finish != nil {
		defer func() { finish(!committed) }()
	}
	if err != nil {
		return err
	}
	value, err := getNodeResult(raw, true)
	if err != nil {
		return err
	}
	fields, info, err := decodeHTTP2Fields(value.([]byte), mode)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "big")
	var fill func(*base.Node, []http2WireField) error
	fill = func(parent *base.Node, fields []http2WireField) error {
		for _, f := range fields {
			if f.Start < 0 || f.End < f.Start || uint64(f.End) > bits {
				return fmt.Errorf("http2-fields: invalid field span")
			}
			var child *base.Node
			if f.Type == "" {
				child = &base.Node{Name: f.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
			} else {
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, f.Name, f.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(f.Start), start + uint64(f.End)})
			}
			child.Cfg.SetItem(CfgParent, parent)
			child.Cfg.SetItem(CfgLength, uint64(f.End-f.Start))
			if f.Info != nil {
				child.Cfg.SetItem("additionInfo", f.Info)
			}
			if err := fill(child, f.Children); err != nil {
				return err
			}
			parent.Children = append(parent.Children, child)
		}
		return nil
	}
	if err = fill(staged, fields); err != nil {
		return err
	}
	if err = InitNode(staged); err != nil {
		return err
	}
	for _, child := range staged.Children {
		child.Cfg.SetItem(CfgParent, node)
	}
	node.Children = staged.Children
	node.Cfg.SetItem("additionInfo", info)
	committed = true
	return nil
}
