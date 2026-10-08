package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// An explicit parse-only rule operation. Commit the one bounded read and the
// entirely new field tree together; no parser state survives between messages.
func parseMegacoMessage(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits%8 != 0 || bits == 0 || bits > megacoMaxBytes*8 {
		return fmt.Errorf("megaco: explicit byte message boundary must be 1..65527 bytes")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "Megaco staged message", "raw", node.Ctx)
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
	fields, err := decodeMegacoMessage(value.([]byte))
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	var fill func(*base.Node, []megacoField) error
	fill = func(parent *base.Node, fields []megacoField) error {
		for _, field := range fields {
			if field.Start < 0 || field.End < field.Start || uint64(field.End)*8 > bits {
				return fmt.Errorf("megaco: invalid decoded span")
			}
			var child *base.Node
			if field.Type == "" {
				child = &base.Node{Name: field.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
				child.Cfg.SetItem(CfgIsList, false)
			} else {
				if field.Type != "raw" && field.Type != "string" {
					return fmt.Errorf("megaco: unsupported decoded field type")
				}
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, field.Name, field.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(field.Start)*8, start + uint64(field.End)*8})
			}
			child.Cfg.SetItem(CfgLength, uint64(field.End-field.Start)*8)
			child.Cfg.SetItem(CfgParent, parent)
			if field.Info != nil {
				child.Cfg.SetItem("additionInfo", field.Info)
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
	profile := "RFC 3525 v1 bounded text structural subset"
	for _, field := range fields {
		if field.Name == "Version" && field.Info["Numeric Value"] == uint64(2) {
			profile = "ITU-T H.248.1 (05/2002) v2 command-only AMMS replies"
		}
	}
	node.Children = staged.Children
	node.Cfg.SetItem("additionInfo", map[string]any{
		"Profile": profile, "Maximum Message Bytes": megacoMaxBytes, "Maximum Field Nodes": megacoMaxNodes, "Maximum Nesting": megacoMaxDepth,
		"Binary Encoding Decoded": false, "SDP Semantics Decoded": false, "Package Semantics Decoded": false, "Transaction Outcome Validated": false, "Session State Validated": false,
	})
	committed = true
	return nil
}
