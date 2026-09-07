package stream_parser

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// parseGIOPMessage parses one PDU. Exact entries reject trailing bytes when
// supplied an explicit input boundary; stream entries leave following PDUs to
// their caller. No TCP or GIOP fragment reassembly is inferred here. Both staged
// reads and the field tree commit only after complete wire validation.
func parseGIOPMessage(node *base.Node, process func(*base.Node) (func(bool), error), exact bool) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if bounded && (bits%8 != 0 || bits < 12*8) {
		return fmt.Errorf("giop: incomplete or non-byte message boundary")
	}
	var finishers []func(bool)
	committed := false
	defer func() {
		for i := len(finishers) - 1; i >= 0; i-- {
			finishers[i](!committed)
		}
	}()
	read := func(name string, length uint64) ([]byte, uint64, error) {
		raw, err := base.NewNodeTreeWithConfig(node.Cfg, name, "raw", node.Ctx)
		if err != nil {
			return nil, 0, err
		}
		raw.Cfg.SetItem(CfgParent, node)
		raw.Cfg.SetItem(CfgLength, length*8)
		finish, err := process(raw)
		if finish != nil {
			finishers = append(finishers, finish)
		}
		if err != nil {
			return nil, 0, err
		}
		value, err := getNodeResult(raw, true)
		if err != nil {
			return nil, 0, err
		}
		return value.([]byte), GetNodeResultPos(raw)[0], nil
	}
	header, start, err := read("GIOP staged header", 12)
	if err != nil {
		return err
	}
	if string(header[:4]) != "GIOP" || header[4] != 1 || header[5] > 3 {
		return fmt.Errorf("giop: expected GIOP version 1.0..1.3")
	}
	endian := "big"
	var order binary.ByteOrder = binary.BigEndian
	if header[6]&1 != 0 {
		endian, order = "little", binary.LittleEndian
	}
	size := uint64(order.Uint32(header[8:12]))
	if size > 1048576 || (bounded && ((12+size)*8 > bits || (exact && (12+size)*8 != bits))) {
		return fmt.Errorf("giop: declared size does not fit message boundary or 1 MiB body limit")
	}
	var body []byte
	if size > 0 {
		body, _, err = read("GIOP staged body", size)
		if err != nil {
			return err
		}
	}
	fields, err := decodeGIOPBody(body, header[4], header[5], header[6], header[7])
	if err != nil {
		return err
	}
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, endian)
	var fill func(*base.Node, []giopField, int) error
	fill = func(parent *base.Node, fields []giopField, offset int) error {
		for index, field := range fields {
			if field.Start < 0 || field.End < field.Start || field.End+offset > int(12+size) {
				return fmt.Errorf("giop: invalid decoded field span")
			}
			var child *base.Node
			if field.Type == "" {
				child = &base.Node{Name: field.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
				child.Cfg.SetItem(CfgIsList, field.List)
			} else {
				switch field.Type {
				case "raw", "string", "uint8", "uint16", "uint32":
				default:
					return fmt.Errorf("giop: unsupported decoded field type %q", field.Type)
				}
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, field.Name, field.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(offset+field.Start)*8, start + uint64(offset+field.End)*8})
			}
			child.Cfg.SetItem(CfgLength, uint64(field.End-field.Start)*8)
			child.Cfg.SetItem(CfgParent, parent)
			if parent.Cfg.GetBool(CfgIsList) {
				child.Cfg.SetItem(CfgElementIndex, index)
			}
			if err := fill(child, field.Children, offset); err != nil {
				return err
			}
			parent.Children = append(parent.Children, child)
		}
		return nil
	}
	if err = fill(staged, []giopField{
		{Name: "Magic", Type: "raw", Start: 0, End: 4},
		{Name: "Major", Type: "uint8", Start: 4, End: 5},
		{Name: "Minor", Type: "uint8", Start: 5, End: 6},
		{Name: "Flags", Type: "uint8", Start: 6, End: 7},
		{Name: "Message Type", Type: "uint8", Start: 7, End: 8},
		{Name: "Message Size", Type: "uint32", Start: 8, End: 12},
	}, 0); err != nil {
		return err
	}
	if err = fill(staged, fields, 12); err != nil {
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
		"Body Fields Validated":         header[6]&2 == 0 && header[7] != 7,
		"More Fragments":                header[5] > 0 && header[6]&2 != 0,
		"Fragment Reassembly Performed": false, "Stub Semantics Decoded": false,
	})
	committed = true
	return nil
}
