package stream_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// An explicit complete-PDU entry; derived fields do not fabricate leaf spans.
// Native receivers use these same codecs after directional state selection.
func parseObservedSession(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > (1<<20)*8 {
		return fmt.Errorf("session fields: explicit bounded complete PDU required")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "Message", "raw", node.Ctx)
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
	v, err := getNodeResult(raw, true)
	if err != nil {
		return err
	}
	wire, ok := v.([]byte)
	if !ok {
		return fmt.Errorf("session fields: bytes unavailable")
	}
	var fields map[string]any
	switch profile {
	case "stp", "pvst":
		fields, err = DecodeSTPWire(wire, profile == "pvst", 4096)
	case "l2tp3-ip", "l2tp3-udp":
		fields, err = DecodeL2TPv3Wire(wire, profile == "l2tp3-udp", 4096)
	case "pim-control":
		fields, err = DecodePIMControl(wire, PIMWireContext{}, 4096)
	case "ldp-session":
		fields, err = DecodeLDPSessionPDU(wire, 4096)
	case "ldp-discovery":
		fields, err = DecodeLDPDiscovery(wire, 4096)
	case "bgp":
		fields, err = DecodeBGPMessage(wire, BGPWireContext{}, 4096)
	case "ajp", "ajp-body":
		fields, err = DecodeAJPMessage(wire, profile == "ajp-body", 4096)
	case "rtmp-handshake":
		fields, err = RTMPInitialHandshake(wire)
	case "rtmp-initial-chunk":
		fields, err = DecodeRTMPInitialChunk(wire, 1<<20)
	case "amf0":
		var values []any
		values, err = DecodeAMF0(wire, 4096)
		fields = map[string]any{"Values": values}
	case "hsrp", "glbp", "bfd":
		fields, err = DecodeRoutingControl(wire, profile, 4096)
	default:
		return fmt.Errorf("unselected observed session profile")
	}
	if err != nil {
		return err
	}
	// NodeToMap intentionally does not execute custom `out` expressions.
	// Publish the validated value explicitly so Result and ParseStructured
	// expose the same tree, while the sole byte span remains the whole PDU.
	raw.Cfg.SetItem(CfgRawResult, fields)
	node.Children = []*base.Node{raw}
	node.Cfg.SetItem("additionInfo", map[string]any{"Profile": profile, "Connection Context": "not supplied to stateless entry"})
	committed = true
	return nil
}
