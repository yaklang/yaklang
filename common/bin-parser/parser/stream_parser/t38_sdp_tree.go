package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func parseT38SDPAdvertisement(node *base.Node, process func(*base.Node) (func(bool), error), h248 bool) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits%8 != 0 || bits == 0 || bits > t38SDPMaxBytes*8 {
		return fmt.Errorf("t38-sdp: explicit byte request boundary must be 1..65536 bytes")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "T38 SDP staged advertisement", "raw", node.Ctx)
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
	fields, info, err := decodeT38SDPAdvertisement(value.([]byte), h248)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	var fill func(*base.Node, []t38SDPField) error
	fill = func(parent *base.Node, fields []t38SDPField) error {
		for _, f := range fields {
			if f.Start < 0 || f.End < f.Start || uint64(f.End) > bits/8 {
				return fmt.Errorf("t38-sdp: invalid decoded span")
			}
			var child *base.Node
			if f.Type == "" {
				child = &base.Node{Name: f.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
				child.Cfg.SetItem(CfgIsList, false)
			} else {
				if f.Type != "raw" && f.Type != "string" {
					return fmt.Errorf("t38-sdp: unsupported decoded type")
				}
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, f.Name, f.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(f.Start)*8, start + uint64(f.End)*8})
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
