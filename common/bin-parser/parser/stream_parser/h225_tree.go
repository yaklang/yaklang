package stream_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// parseH225Message stages one explicitly bounded RAS datagram or complete
// TPKT/Q.931 call record. Neither TCP reassembly nor protocol guessing is done.
func parseH225Message(node *base.Node, process func(*base.Node) (func(bool), error), mode string) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits%8 != 0 || bits == 0 || bits > 65535*8 {
		return fmt.Errorf("h225: explicit 1..65535 byte message boundary required")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "H225 staged message", "raw", node.Ctx)
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
		return fmt.Errorf("h225: staged message is not raw bytes")
	}
	fields, info, err := decodeH225Message(wire, mode)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "big")
	var fill func(*base.Node, []h225Field) error
	fill = func(parent *base.Node, fields []h225Field) error {
		for index, field := range fields {
			if field.Start < 0 || field.End < field.Start || uint64(field.End) > bits {
				return fmt.Errorf("h225: invalid decoded field span")
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
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(field.Start), start + uint64(field.End)})
				if field.Decoded {
					child.Cfg.SetItem("h225 decoded value", field.Value)
					child.Cfg.SetItem("out", "return node.Cfg.GetItem(\"h225 decoded value\")")
				}
			}
			child.Cfg.SetItem(CfgLength, uint64(field.End-field.Start))
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
