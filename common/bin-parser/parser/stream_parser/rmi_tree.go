package stream_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// Only the historical RMI header entry can read an unbounded, self-delimiting
// seven-byte transport header. All exact record/phase entries require a bound.
func parseRMIRecord(node *base.Node, process func(*base.Node) (func(bool), error), mode string) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if mode == "legacy-header" {
		mode = "header"
		if !bounded {
			bits = 56
			bounded = true
		}
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > 8<<20 {
		return fmt.Errorf("rmi: explicit 1..1048576 byte record boundary required")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "RMI staged record", "raw", node.Ctx)
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
		return fmt.Errorf("rmi: staged record is not raw")
	}
	fields, info, err := decodeRMIRecord(wire, mode)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "big")
	var fill func(*base.Node, []rmiField) error
	fill = func(parent *base.Node, fields []rmiField) error {
		for index, field := range fields {
			if field.Start < 0 || field.End < field.Start || uint64(field.End)*8 > bits {
				return fmt.Errorf("rmi: invalid decoded field span")
			}
			var child *base.Node
			if field.Type == "" {
				child = &base.Node{Name: field.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
				child.Cfg.SetItem(CfgIsList, field.List)
			} else {
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, field.Name, field.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(field.Start)*8, start + uint64(field.End)*8})
				if field.Decoded {
					child.Cfg.SetItem("rmi decoded value", field.Value)
					child.Cfg.SetItem("out", "return node.Cfg.GetItem(\"rmi decoded value\")")
				}
			}
			child.Cfg.SetItem(CfgLength, uint64(field.End-field.Start)*8)
			child.Cfg.SetItem(CfgParent, parent)
			if parent.Cfg.GetBool(CfgIsList) {
				child.Cfg.SetItem(CfgElementIndex, index)
			}
			if err := fill(child, field.Children); err != nil {
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
