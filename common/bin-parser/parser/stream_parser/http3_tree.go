package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// The complete wire plan and local QPACK snapshot are validated before any
// node is published. The reader transaction composes with caller-held backups.
func parseHTTP3Stream(node *base.Node, mode string, process func(*base.Node) (func(bool), error)) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits%8 != 0 || bits == 0 || bits > http3MaxBytes*8 {
		return fmt.Errorf("http3: explicit byte stream boundary must be 1..1048576 bytes")
	}
	var encoder []byte
	if value, present := node.Ctx.LookupItem("http3QPACKEncoderStream"); present {
		var ok bool
		encoder, ok = value.([]byte)
		if !ok {
			return fmt.Errorf("http3: encoder snapshot config must be []byte")
		}
	}
	var capacity uint64
	if value, present := node.Ctx.LookupItem("http3QPACKMaxTableCapacity"); present {
		switch n := value.(type) {
		case uint64:
			capacity = n
		case int:
			if n < 0 {
				return fmt.Errorf("http3: negative maximum table capacity")
			}
			capacity = uint64(n)
		default:
			return fmt.Errorf("http3: maximum table capacity config must be uint64/int")
		}
	}
	if len(encoder) > http3MaxBytes || capacity > http3MaxBytes {
		return fmt.Errorf("http3: encoder snapshot/capacity exceeds implementation profile")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "HTTP3 staged stream", "raw", node.Ctx)
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
	fields, info, err := decodeHTTP3Stream(value.([]byte), mode, encoder, capacity)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "big")
	var fill func(*base.Node, []http3Field) error
	fill = func(parent *base.Node, fields []http3Field) error {
		for _, f := range fields {
			if f.Start < 0 || f.End < f.Start || uint64(f.End) > bits {
				return fmt.Errorf("http3: invalid field span")
			}
			var child *base.Node
			if f.Type == "" {
				child = &base.Node{Name: f.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
			} else {
				switch f.Type {
				case "raw", "string", "uint8", "uint64":
				default:
					return fmt.Errorf("http3: invalid field type")
				}
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
