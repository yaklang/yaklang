package stream_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// Selected wire-version entry points require an explicit datagram boundary.
// Derived fields are an envelope value, not invented contiguous field spans:
// RDPEUDP2's on-wire byte permutation cannot be described as one field offset.
func parseRDPUDPEnvelope(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	version := uint16(1)
	if profile == "v2" {
		version = 0x101
	} else if profile != "v1" {
		return fmt.Errorf("rdpudp: unknown selected wire profile")
	}
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits < 64 || bits%8 != 0 || bits > 65507*8 {
		return fmt.Errorf("rdpudp: explicit datagram boundary required")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "Envelope", "raw", node.Ctx)
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
		return fmt.Errorf("rdpudp: staged bytes unavailable")
	}
	decoded, err := DecodeRDPUDPWire(wire, version)
	if err != nil {
		return err
	}
	raw.Cfg.SetItem("rdpudp observed envelope", decoded.Fields)
	raw.Cfg.SetItem("out", "return node.Cfg.GetItem(\"rdpudp observed envelope\")")
	node.Children = []*base.Node{raw}
	node.Cfg.SetItem("additionInfo", map[string]any{"Profile": profile, "Authentication Verified": false, "TLS Context Required": true})
	committed = true
	return nil
}
