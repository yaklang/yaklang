package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// parseSteamDiscovery stages one bounded datagram and its complete field tree.
// The callback transaction is committed only after wire/schema validation and
// tree construction succeed. All leaf spans refer to the original wire buffer;
// decoded values are not cached on nodes or shared between parser instances.
func parseSteamDiscovery(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits%8 != 0 {
		return fmt.Errorf("steam discovery: explicit byte boundary required")
	}
	if bits < 16*8 || bits > 65535*8 {
		return fmt.Errorf("steam discovery: datagram outside 16..65535 bytes")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "Steam staged wire", "raw", node.Ctx)
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
	message, err := decodeSteamDiscovery(value.([]byte))
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	leaf := func(parent *base.Node, name, kind string, from, to int) (*base.Node, error) {
		if from < 0 || to < from || to > len(value.([]byte)) {
			return nil, fmt.Errorf("steam discovery: invalid field span")
		}
		origin := "raw"
		if kind == "string" || kind == "uint32" {
			origin = kind
		}
		n, err := base.NewNodeTreeWithConfig(parent.Cfg, name, origin, node.Ctx)
		if err != nil {
			return nil, err
		}
		n.Cfg.SetItem(CfgParent, parent)
		n.Cfg.SetItem(CfgEndian, "little")
		n.Cfg.SetItem(CfgLength, uint64(to-from)*8)
		n.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(from)*8, start + uint64(to)*8})
		if kind != "raw" && kind != "string" && kind != "uint32" {
			code := steamScalarOutCode(kind)
			if code == "" {
				return nil, fmt.Errorf("steam discovery: unsupported field kind %q", kind)
			}
			n.Cfg.SetItem("out", code)
		}
		parent.Children = append(parent.Children, n)
		return n, nil
	}
	branch := func(parent *base.Node, name string) *base.Node {
		n := &base.Node{Name: name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
		n.Cfg.SetItem(CfgParent, parent)
		parent.Children = append(parent.Children, n)
		return n
	}
	var fields func(*base.Node, []steamDiscoveryField) error
	fields = func(parent *base.Node, records []steamDiscoveryField) error {
		// Protobuf fields are ordered and may repeat even when the schema says
		// optional. A struct with repeated "Field" keys would lose all but the
		// final record in NodeToMap; list identity preserves every occurrence.
		parent.Cfg.SetItem(CfgIsList, true)
		for _, field := range records {
			record := branch(parent, "Field")
			record.Cfg.SetItem("additionInfo", map[string]any{"Field Number": field.Number, "Wire Type": field.Wire, "Kind": field.Kind})
			if field.Start < field.ValueStart {
				if _, err := leaf(record, "Field Encoding", "raw", field.Start, field.ValueStart); err != nil {
					return err
				}
			}
			switch field.Kind {
			case "message", "packed", "group":
				if len(field.Children) == 0 {
					if _, err := leaf(record, field.Name, "raw", field.ValueStart, field.End); err != nil {
						return err
					}
				} else if err := fields(branch(record, field.Name), field.Children); err != nil {
					return err
				}
			default:
				if _, err := leaf(record, field.Name, field.Kind, field.ValueStart, field.End); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, f := range []struct {
		name, kind string
		from, to   int
	}{
		{"Signature", "raw", 0, 8}, {"Header Length", "uint32", 8, 12},
	} {
		if _, err := leaf(staged, f.name, f.kind, f.from, f.to); err != nil {
			return err
		}
	}
	if len(message.Header) == 0 {
		_, err = leaf(staged, "Header", "raw", message.HeaderStart, message.HeaderEnd)
	} else {
		err = fields(branch(staged, "Header"), message.Header)
	}
	if err != nil {
		return err
	}
	if _, err = leaf(staged, "Body Length", "uint32", message.HeaderEnd, message.BodyStart); err != nil {
		return err
	}
	if len(message.Body) == 0 {
		_, err = leaf(staged, "Body", "raw", message.BodyStart, message.BodyEnd)
	} else {
		err = fields(branch(staged, "Body"), message.Body)
	}
	if err != nil {
		return err
	}
	if err = InitNode(staged); err != nil {
		return err
	}
	for _, child := range staged.Children {
		child.Cfg.SetItem(CfgParent, node)
	}
	node.Children = staged.Children
	node.Cfg.SetItem("additionInfo", map[string]any{"Effective Message Type": message.MessageType, "Field Count": message.Fields, "Session State Decoded": false})
	committed = true
	return nil
}

// An exact expression fast path avoids creating a VM for every scalar result.
// The general expression library exposes the same pure decoder as its oracle.
// Reuse constant expressions rather than formatting an identical string for
// each scalar in a large repeated-field tree.
func steamScalarOutCode(kind string) string {
	switch kind {
	case "u64":
		return `return decodeSteamScalar(data.Value, "u64")`
	case "u32":
		return `return decodeSteamScalar(data.Value, "u32")`
	case "i32":
		return `return decodeSteamScalar(data.Value, "i32")`
	case "bool":
		return `return decodeSteamScalar(data.Value, "bool")`
	case "fixed64":
		return `return decodeSteamScalar(data.Value, "fixed64")`
	case "float32":
		return `return decodeSteamScalar(data.Value, "float32")`
	}
	return ""
}

func steamScalarOutKind(code string) string {
	switch code {
	case `return decodeSteamScalar(data.Value, "u64")`:
		return "u64"
	case `return decodeSteamScalar(data.Value, "u32")`:
		return "u32"
	case `return decodeSteamScalar(data.Value, "i32")`:
		return "i32"
	case `return decodeSteamScalar(data.Value, "bool")`:
		return "bool"
	case `return decodeSteamScalar(data.Value, "fixed64")`:
		return "fixed64"
	case `return decodeSteamScalar(data.Value, "float32")`:
		return "float32"
	}
	return ""
}
