package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func parseTLS12CertificateAuth(node *base.Node, process func(*base.Node) (func(bool), error), messageType uint8) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > tlsCertificateAuthMaxBytes*8 {
		return fmt.Errorf("tls-certificate-auth: explicit 1..131333 byte boundary required")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "TLS certificate exchange staged handshake", "raw", node.Ctx)
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
		return fmt.Errorf("tls-certificate-auth: staged value is not raw")
	}
	fields, info, err := decodeTLS12CertificateAuth(wire, messageType)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "big")
	var fill func(*base.Node, []tlsCertificateAuthField) error
	fill = func(parent *base.Node, fields []tlsCertificateAuthField) error {
		for index, f := range fields {
			if f.Start < 0 || f.End < f.Start || uint64(f.End)*8 > bits {
				return fmt.Errorf("tls-certificate-auth: invalid field span")
			}
			var child *base.Node
			if f.Type == "" {
				child = &base.Node{Name: f.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
				// An observed empty vector still has a precise zero-width wire
				// result. Keep the list empty, not a fabricated child element.
				if f.List && f.Start == f.End && len(f.Children) == 0 {
					child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(f.Start)*8, start + uint64(f.End)*8})
				}
			} else {
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, f.Name, f.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(f.Start)*8, start + uint64(f.End)*8})
			}
			child.Cfg.SetItem(CfgParent, parent)
			child.Cfg.SetItem(CfgLength, uint64(f.End-f.Start)*8)
			child.Cfg.SetItem(CfgIsList, f.List)
			if parent.Cfg.GetBool(CfgIsList) {
				child.Cfg.SetItem(CfgElementIndex, index)
			}
			if err = fill(child, f.Children); err != nil {
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
