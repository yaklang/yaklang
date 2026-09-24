package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// parseLATMessage is a parse-only native rule operation, not an implicit
// optimization of arbitrary user-defined nodes. The single bounded read, output
// bits and field tree are all committed only after structural validation.
func parseLATMessage(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits%8 != 0 || bits < 64 || bits > 1500*8 {
		return fmt.Errorf("lat: explicit byte message boundary must be 8..1500 bytes")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "LAT staged message", "raw", node.Ctx)
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
	wire := value.([]byte)
	fields, err := decodeLATMessage(wire)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "little")
	var fill func(*base.Node, []latField) error
	fill = func(parent *base.Node, fields []latField) error {
		for index, field := range fields {
			if field.Start < 0 || field.End < field.Start || uint64(field.End) > bits {
				return fmt.Errorf("lat: invalid decoded field span")
			}
			var child *base.Node
			if field.Type == "" {
				child = &base.Node{Name: field.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
				child.Cfg.SetItem(CfgIsList, field.List)
			} else {
				switch field.Type {
				case "raw", "string", "uint8", "uint16":
				default:
					return fmt.Errorf("lat: unsupported decoded field type")
				}
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, field.Name, field.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(field.Start), start + uint64(field.End)})
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
	node.Cfg.SetItem("additionInfo", map[string]any{
		"Profile": "LAT virtual-circuit structural fields", "Maximum Message Bytes": 1500,
		"Sender Role":             map[bool]string{false: "host", true: "terminal-server"}[wire[0]&2 != 0],
		"Session State Validated": false, "Negotiated Limits Validated": false,
		"Service Data Semantics Decoded": false, "Parameter Data Semantics Decoded": false,
		"Discovery Exchange Decoded": false, "Ethernet Padding Inferred": false,
	})
	committed = true
	return nil
}
