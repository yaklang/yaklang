package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func parseTLS12KeyExchange(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > tls12KeyExchangeMaxBytes*8 {
		return fmt.Errorf("tls12-key-exchange: explicit 1..262154 byte boundary required")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "TLS key exchange staged handshake", "raw", node.Ctx)
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
		return fmt.Errorf("tls12-key-exchange: staged value is not raw")
	}
	fields, info, err := decodeTLS12KeyExchange(wire, profile)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "big")
	for _, f := range fields {
		if f.Start < 0 || f.End < f.Start || uint64(f.End)*8 > bits {
			return fmt.Errorf("tls12-key-exchange: invalid decoded field span")
		}
		child, err := base.NewNodeTreeWithConfig(staged.Cfg, f.Name, f.Type, node.Ctx)
		if err != nil {
			return err
		}
		child.Cfg.SetItem(CfgParent, staged)
		child.Cfg.SetItem(CfgLength, uint64(f.End-f.Start)*8)
		child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(f.Start)*8, start + uint64(f.End)*8})
		staged.Children = append(staged.Children, child)
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
