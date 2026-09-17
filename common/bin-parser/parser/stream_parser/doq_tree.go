package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func parseDoQStream(node *base.Node, mode string, response bool, process func(*base.Node) (func(bool), error)) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > 8<<20 {
		return fmt.Errorf("doq: explicit 1..1048576 byte clear stream boundary required")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "DoQ staged clear stream", "raw", node.Ctx)
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
	wire, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("doq: staged stream is not raw")
	}
	fields, info, err := decodeDoQStream(wire, mode, response)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "big")
	var fill func(*base.Node, []doqField) error
	fill = func(parent *base.Node, fields []doqField) error {
		for _, f := range fields {
			if f.Start < 0 || f.End < f.Start || uint64(f.End)*8 > bits {
				return fmt.Errorf("doq: invalid field span")
			}
			var child *base.Node
			if f.Type == "" {
				child = &base.Node{Name: f.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
			} else {
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, f.Name, f.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(f.Start)*8, start + uint64(f.End)*8})
				if f.Decoded {
					child.Cfg.SetItem("doq decoded value", f.Value)
					child.Cfg.SetItem("out", "return node.Cfg.GetItem(\"doq decoded value\")")
				}
			}
			child.Cfg.SetItem(CfgLength, uint64(f.End-f.Start)*8)
			child.Cfg.SetItem(CfgParent, parent)
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
