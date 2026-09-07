package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func parseTLS12ControlHandshake(node *base.Node, process func(*base.Node) (func(bool), error), messageType uint8) error {
	if messageType != 4 && messageType != 14 {
		return fmt.Errorf("tls12-control: unsupported handshake profile")
	}
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits < 32 || bits%8 != 0 || bits > tls12ControlMaxBytes*8 {
		return fmt.Errorf("tls12-control: explicit 4..65545 byte handshake boundary required")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "TLS control staged handshake", "raw", node.Ctx)
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
		return fmt.Errorf("tls12-control: staged value is not raw")
	}
	fields, info, err := decodeTLS12ControlHandshake(wire, messageType)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "big")
	for _, f := range fields {
		if f.Start < 0 || f.End < f.Start || uint64(f.End)*8 > bits {
			return fmt.Errorf("tls12-control: invalid field span")
		}
		child, err := base.NewNodeTreeWithConfig(staged.Cfg, f.Name, f.Type, node.Ctx)
		if err != nil {
			return err
		}
		child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(f.Start)*8, start + uint64(f.End)*8})
		child.Cfg.SetItem(CfgLength, uint64(f.End-f.Start)*8)
		child.Cfg.SetItem(CfgParent, staged)
		staged.Children = append(staged.Children, child)
	}
	if err := InitNode(staged); err != nil {
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
