package parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// ProjectCIFSNegotiateResponseFields projects only the explicitly selected,
// successfully parsed WC17 extended-security response. A dialect index is a
// literal, not a dialect name or proof of a matched request/authenticated session.
func ProjectCIFSNegotiateResponseFields(node *base.Node, raw []byte) (fields map[string]any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fields, err = nil, fmt.Errorf("CIFS response projection: invalid parsed Node: %v", recovered)
		}
	}()
	if node == nil || node.Cfg == nil || len(raw) > 65608 {
		return nil, fmt.Errorf("CIFS response projection: invalid bounded source")
	}
	p := cifsFieldProjection{raw: raw}
	fields = map[string]any{"Wire Field Ranges": map[string]any{}}
	message := node
	switch node.Name {
	case "CIFSDirectTCPNegotiateExtendedResponse":
		children, e := cifsProjectionChildren(node, "Direct TCP Zero", "Direct TCP Length", "Negotiate Response")
		if e != nil {
			return nil, e
		}
		direct := map[string]any{"Wire Field Ranges": map[string]any{}}
		if e = p.literal(direct, "Message Type", children[0], "uint8", 1, false); e != nil {
			return nil, e
		}
		if e = p.literal(direct, "Payload Length", children[1], "uint32", 3, false); e != nil {
			return nil, e
		}
		fields["Direct TCP"] = direct
		message = children[2]
	case "CIFSNegotiateExtendedResponse":
	default:
		return nil, fmt.Errorf("CIFS response projection: unselected entry %q", node.Name)
	}
	children, e := cifsProjectionChildren(message, "ProtocolId", "Command", "Status", "Flags", "Flags2", "PIDHigh", "SecurityFeatures", "Reserved", "TID", "PID", "UID", "MID", "Body")
	if e != nil {
		return nil, e
	}
	header := map[string]any{"Wire Field Ranges": map[string]any{}}
	for i, spec := range []struct {
		name, typ string
		width     int
	}{
		{"Protocol ID", "uint32", 4}, {"Command", "uint8", 1}, {"Status", "uint32", 4},
		{"Flags", "uint8", 1}, {"Flags 2", "uint16", 2}, {"PID High", "uint16", 2},
		{"Security Features", "raw", 8}, {"Reserved", "uint16", 2}, {"Tree ID", "uint16", 2},
		{"Process ID Low", "uint16", 2}, {"User ID", "uint16", 2}, {"Multiplex ID", "uint16", 2},
	} {
		if e = p.literal(header, spec.name, children[i], spec.typ, spec.width, true); e != nil {
			return nil, e
		}
	}
	fields["Header"] = header
	bn := children[12]
	names := []string{"WordCount", "DialectIndex", "SecurityMode", "MaxMpxCount", "MaxNumberVcs", "MaxBufferSize", "MaxRawSize", "SessionKey", "Capabilities", "SystemTime", "ServerTimeZone", "ChallengeLength", "ByteCount", "ServerGUID", "SecurityBlob"}
	bc, e := cifsProjectionChildren(bn, names...)
	if e != nil {
		return nil, e
	}
	body := map[string]any{"Wire Field Ranges": map[string]any{}}
	for i, spec := range []struct {
		name, typ string
		width     int
	}{
		{"Word Count", "uint8", 1}, {"Dialect Index", "uint16", 2}, {"Security Mode", "uint8", 1},
		{"Max Mpx Count", "uint16", 2}, {"Max Number Vcs", "uint16", 2}, {"Max Buffer Size", "uint32", 4},
		{"Max Raw Size", "uint32", 4}, {"Session Key", "uint32", 4}, {"Capabilities", "uint32", 4},
		{"System Time", "uint64", 8}, {"Server Time Zone", "int16", 2}, {"Challenge Length", "uint8", 1},
		{"Byte Count", "uint16", 2}, {"Server GUID", "raw", 16},
	} {
		if e = p.literal(body, spec.name, bc[i], spec.typ, spec.width, true); e != nil {
			return nil, e
		}
	}
	count := body["Byte Count"].(uint16)
	if count > 16 {
		if e = p.literal(body, "Security Blob", bc[14], "raw", int(count)-16, true); e != nil {
			return nil, e
		}
	} else {
		unused := bc[14]
		if count != 16 || unused.Cfg == nil || unused.Cfg.ResultSettings().Present || unused.Cfg.ResultSettings().RawPresent || len(unused.Children) != 0 {
			return nil, fmt.Errorf("CIFS response projection: missing security blob")
		}
		body["Security Blob"] = []byte{}
		cifsProjectionRange(body, "Security Blob", [2]uint64{p.at, p.at})
	}
	if p.at != uint64(len(raw)) {
		return nil, fmt.Errorf("CIFS response projection: parsed fields do not cover bounded source")
	}
	fields["Body"] = body
	fields["Packet Name"], fields["Response"] = "SMB_COM_NEGOTIATE", true
	fields["Extended Security"] = true
	fields["Dialect Negotiated"], fields["Session Established"] = false, false
	fields["Signature Verified"], fields["Authentication Verified"] = false, false
	return fields, nil
}
