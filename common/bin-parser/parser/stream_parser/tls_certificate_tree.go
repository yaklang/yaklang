package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func parseTLSCertificateHandshake(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	return parseCertificateFieldTree(node, process, decodeTLSCertificateHandshake, "tls-certificate")
}

// Both entries publish fully staged fields only after their selected grammar
// succeeds. Keeping the TLS vector grammar separate preserves opaque non-X.509
// negotiated representations and the historical Certificate DER raw field.
func parseCertificateFieldTree(node *base.Node, process func(*base.Node) (func(bool), error), decode func([]byte) ([]tlsCertificateField, map[string]any, error), profile string) error {
	return parseExactByteFieldTreeWithEndian(node, process, decode, profile, "big")
}

// The wire grammar owns field byte order, independently of the enclosing rule.
// Existing network-order callers retain their default; no caller config changes.
func parseExactByteFieldTreeWithEndian(node *base.Node, process func(*base.Node) (func(bool), error), decode func([]byte) ([]tlsCertificateField, map[string]any, error), profile, endian string) error {
	if endian != "big" && endian != "little" {
		return fmt.Errorf("%s: unsupported field byte order", profile)
	}
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > tlsCertificateMaxBytes*8 {
		return fmt.Errorf("%s: explicit 1..1048576 byte boundary required", profile)
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "TLS Certificate staged handshake", "raw", node.Ctx)
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
		return fmt.Errorf("tls-certificate: staged value is not raw")
	}
	fields, info, err := decode(wire)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	if err := buildExactByteFieldTree(node, fields, info, start, bits, profile, endian); err != nil {
		return err
	}
	committed = true
	return nil
}

// Construct all public fields before publishing children/metadata.
func buildExactByteFieldTree(node *base.Node, fields []tlsCertificateField, info map[string]any, start, bits uint64, profile, endian string) error {
	var err error
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfigWithItems(node.Cfg, base.ConfigItem{Key: CfgEndian, Value: endian}), Ctx: node.Ctx}
	var count func([]tlsCertificateField) int
	count = func(fs []tlsCertificateField) int {
		n := len(fs)
		for _, f := range fs {
			n += count(f.Children)
		}
		return n
	}
	batch := base.NewNodeBatch(count(fields))
	var fill func(*base.Node, []tlsCertificateField) error
	fill = func(parent *base.Node, fields []tlsCertificateField) error {
		if len(fields) == 0 {
			return nil // retain nil Children, not an allocated empty slice
		}
		parent.Children = make([]*base.Node, 0, len(fields))
		parentIsList := parent.Cfg.GetBool(CfgIsList)
		for index, f := range fields {
			if f.Endian != "" && f.Endian != "big" && f.Endian != "little" {
				return fmt.Errorf("%s: unsupported individual field byte order", profile)
			}
			if f.Start < 0 || f.End < f.Start || uint64(f.End)*8 > bits {
				return fmt.Errorf("tls-certificate: invalid field span")
			}
			var local [6]base.ConfigItem
			items := local[:0]
			// An empty list has an observed zero-width result; ordinary empty
			// containers do not. Preserve the original config replay order.
			if f.Type != "" || (f.List && f.Start == f.End && len(f.Children) == 0) {
				items = append(items, base.ConfigItem{Key: CfgNodeResult, Value: [2]uint64{start + uint64(f.Start)*8, start + uint64(f.End)*8}})
			}
			if f.Endian != "" {
				items = append(items, base.ConfigItem{Key: CfgEndian, Value: f.Endian})
			}
			items = append(items,
				base.ConfigItem{Key: CfgParent, Value: parent},
				base.ConfigItem{Key: CfgLength, Value: uint64(f.End-f.Start) * 8},
				base.ConfigItem{Key: CfgIsList, Value: f.List})
			if parentIsList {
				items = append(items, base.ConfigItem{Key: CfgElementIndex, Value: index})
			}
			var child *base.Node
			if f.Type == "" {
				child = batch.NewNode(f.Name, yaml.MapSlice{}, parent.Cfg, node.Ctx, items...)
			} else {
				child, err = batch.NewNodeTreeWithConfigItems(parent.Cfg, f.Name, f.Type, node.Ctx, items...)
				if err != nil {
					return err
				}
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
	return nil
}
