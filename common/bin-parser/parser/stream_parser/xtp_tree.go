package stream_parser

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// Explicit parse-only operation. The bounded read and entirely new field tree
// commit together; a failed decode does not mutate caller nodes or metadata.
func parseXTPMessage(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits%8 != 0 || bits < 32*8 || bits > xtpMaxBytes*8 {
		return fmt.Errorf("xtp: explicit byte message boundary must be 32..65535 bytes")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "XTP staged message", "raw", node.Ctx)
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
	fields, err := decodeXTPMessage(wire)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "big")
	var fill func(*base.Node, []xtpField) error
	fill = func(parent *base.Node, fields []xtpField) error {
		for _, field := range fields {
			if field.Start < 0 || field.End < field.Start || uint64(field.End) > bits {
				return fmt.Errorf("xtp: invalid decoded span")
			}
			var child *base.Node
			if field.Type == "" {
				child = &base.Node{Name: field.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
				child.Cfg.SetItem(CfgIsList, false)
			} else {
				switch field.Type {
				case "raw", "uint8", "uint16", "uint32", "uint64":
				default:
					return fmt.Errorf("xtp: unsupported decoded field type")
				}
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, field.Name, field.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(field.Start), start + uint64(field.End)})
			}
			child.Cfg.SetItem(CfgLength, uint64(field.End-field.Start))
			child.Cfg.SetItem(CfgParent, parent)
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
	checkLength := len(wire)
	if wire[8]&0x40 != 0 {
		checkLength = 32
	}
	node.Children = staged.Children
	node.Cfg.SetItem("additionInfo", map[string]any{
		"Profile":               "XTP 4.0 bounded DATA/CNTL/FIRST/TCNTL/JCNTL/DIAG structural fields",
		"Maximum Message Bytes": xtpMaxBytes, "Checksum Valid": true, "Checksum Covered Bytes": checkLength,
		"Payload Checksum Verified": wire[8]&0x40 == 0, "Returned Key": binary.BigEndian.Uint64(wire)&(uint64(1)<<63) != 0,
		"Packet Kind":               map[byte]string{0: "DATA", 1: "CNTL", 2: "FIRST", 5: "TCNTL", 7: "JCNTL", 8: "DIAG"}[wire[11]&31],
		"Reserved Values Validated": false, "Service Semantics Decoded": false, "Data Semantics Decoded": false,
		"Session State Validated": false, "Acknowledgment State Validated": false, "Multicast Membership Validated": false,
	})
	committed = true
	return nil
}
