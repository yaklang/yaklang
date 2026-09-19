package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// parseTLSChangeCipherSpecRecord requires a complete plaintext record supplied
// by the caller. Its legacy record version cannot identify a negotiated version
// or distinguish a TLS 1.3 compatibility message from an earlier-version CCS.
func parseTLSChangeCipherSpecRecord(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits != 48 {
		return fmt.Errorf("tls-ccs: explicit six-byte plaintext record boundary required")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "TLS CCS staged record", "raw", node.Ctx)
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
	if !ok || len(wire) != 6 || wire[0] != 20 || wire[1] != 3 || wire[2] < 1 || wire[2] > 3 || wire[3] != 0 || wire[4] != 1 || wire[5] != 1 {
		return fmt.Errorf("tls-ccs: invalid plaintext ChangeCipherSpec record")
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "big")
	for _, f := range []struct {
		name, typ  string
		start, end uint64
	}{
		{"Content Type", "uint8", 0, 1},
		{"Legacy Record Version", "uint16", 1, 3},
		{"Record Length", "uint16", 3, 5},
		{"ChangeCipherSpec Value", "uint8", 5, 6},
	} {
		child, err := base.NewNodeTreeWithConfig(staged.Cfg, f.name, f.typ, node.Ctx)
		if err != nil {
			return err
		}
		child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + f.start*8, start + f.end*8})
		child.Cfg.SetItem(CfgLength, (f.end-f.start)*8)
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
	node.Cfg.SetItem("additionInfo", map[string]any{
		"Profile":                              "TLS plaintext ChangeCipherSpec record layout",
		"Plaintext Context Supplied By Caller": true,
		"Protocol Version Inferred":            false,
		"Handshake Phase Validated":            false,
		"Cipher State Transition Validated":    false,
		"Handshake Completion Validated":       false,
		"TCP Reassembly Performed":             false,
		"Structured Generation Supported":      false,
	})
	committed = true
	return nil
}
