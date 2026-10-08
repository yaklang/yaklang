package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// Decode into an isolated plan before publishing any output fields. The
// process transaction also composes with a held caller's outer reader backup.
func parseSMB3Negotiate(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits%8 != 0 || bits < 100*8 || bits > smb3NegotiateMaxBytes*8 {
		return fmt.Errorf("smb3 negotiate: explicit byte record boundary must be 100..65536 bytes")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "SMB3 staged negotiate", "raw", node.Ctx)
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
	fields, info, err := decodeSMB3Negotiate(value.([]byte))
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "little")
	var fill func(*base.Node, []smb3Field) error
	fill = func(parent *base.Node, fields []smb3Field) error {
		for i, f := range fields {
			if f.Start < 0 || f.End < f.Start || uint64(f.End) > bits {
				return fmt.Errorf("smb3 negotiate: invalid decoded field span")
			}
			var child *base.Node
			if f.Type == "" {
				child = &base.Node{Name: f.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
				child.Cfg.SetItem(CfgIsList, f.List)
			} else {
				switch f.Type {
				case "raw", "uint8", "uint16", "uint32", "uint64":
				default:
					return fmt.Errorf("smb3 negotiate: invalid decoded field type")
				}
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, f.Name, f.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(f.Start), start + uint64(f.End)})
			}
			child.Cfg.SetItem(CfgParent, parent)
			child.Cfg.SetItem(CfgLength, uint64(f.End-f.Start))
			if parent.Cfg.GetBool(CfgIsList) {
				child.Cfg.SetItem(CfgElementIndex, i)
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
