package parser

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// ProjectCIFSSessionSetupFields projects the successfully parsed explicit
// Unicode/extended-security/terminal-AndX profile. Native and canonical callers
// share this typed, source-bound projection; no connection state is read here.
func ProjectCIFSSessionSetupFields(node *base.Node, raw []byte) (fields map[string]any, err error) {
	defer func() {
		if v := recover(); v != nil {
			fields, err = nil, fmt.Errorf("CIFS setup projection: invalid Node: %v", v)
		}
	}()
	if node == nil || node.Cfg == nil || len(raw) > 65598 {
		return nil, fmt.Errorf("CIFS setup projection: invalid bounded source")
	}
	response := false
	switch node.Name {
	case "CIFSDirectTCPSessionSetupExtendedRequest":
	case "CIFSDirectTCPSessionSetupExtendedResponse":
		response = true
	default:
		return nil, fmt.Errorf("CIFS setup projection: unselected entry %q", node.Name)
	}
	parts, err := cifsProjectionChildren(node, "Direct TCP", "Header", "Body")
	if err != nil {
		return nil, err
	}
	p := cifsFieldProjection{raw: raw}
	fields = map[string]any{"Wire Field Ranges": map[string]any{}}
	direct := map[string]any{"Wire Field Ranges": map[string]any{}}
	dc, err := cifsProjectionChildren(parts[0], "Message Type", "Payload Length")
	if err != nil {
		return nil, err
	}
	if err = p.literal(direct, "Message Type", dc[0], "uint8", 1, false); err != nil {
		return nil, err
	}
	if err = p.literal(direct, "Payload Length", dc[1], "uint32", 3, false); err != nil {
		return nil, err
	}
	fields["Direct TCP"] = direct
	header := map[string]any{"Wire Field Ranges": map[string]any{}}
	hnames := []string{"Protocol ID", "Command", "Status", "Flags", "Flags 2", "PID High", "Security Features", "Reserved", "Tree ID", "Process ID Low", "User ID", "Multiplex ID"}
	hc, err := cifsProjectionChildren(parts[1], hnames...)
	if err != nil {
		return nil, err
	}
	for i, spec := range []struct {
		typ   string
		width int
	}{{"uint32", 4}, {"uint8", 1}, {"uint32", 4}, {"uint8", 1}, {"uint16", 2}, {"uint16", 2}, {"raw", 8}, {"uint16", 2}, {"uint16", 2}, {"uint16", 2}, {"uint16", 2}, {"uint16", 2}} {
		if err = p.literal(header, hnames[i], hc[i], spec.typ, spec.width, true); err != nil {
			return nil, err
		}
	}
	fields["Header"] = header
	type fieldSpec struct {
		name, typ string
		width     int
	}
	specs := []fieldSpec{{"Word Count", "uint8", 1}, {"AndX Command", "uint8", 1}, {"AndX Reserved", "uint8", 1}, {"AndX Offset", "uint16", 2}}
	if response {
		specs = append(specs, fieldSpec{"Action", "uint16", 2}, fieldSpec{"Security Blob Length", "uint16", 2})
	} else {
		specs = append(specs, []fieldSpec{{"Max Buffer Size", "uint16", 2}, {"Max Mpx Count", "uint16", 2}, {"VC Number", "uint16", 2}, {"Session Key", "uint32", 4}, {"Security Blob Length", "uint16", 2}, {"Reserved", "uint32", 4}, {"Capabilities", "uint32", 4}}...)
	}
	specs = append(specs, fieldSpec{"Byte Count", "uint16", 2})
	names := make([]string, 0, len(specs)+5)
	for _, spec := range specs {
		names = append(names, spec.name)
	}
	names = append(names, "Security Blob", "Alignment Pad", "Native OS Raw Bytes", "Native LAN Manager Raw Bytes", "Trailing Extension Bytes")
	bc, err := cifsProjectionChildren(parts[2], names...)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"Wire Field Ranges": map[string]any{}}
	for i, spec := range specs {
		if err = p.literal(body, spec.name, bc[i], spec.typ, spec.width, true); err != nil {
			return nil, err
		}
	}
	for i, name := range names[len(specs):] {
		width := -1
		if i == 0 {
			width = int(body["Security Blob Length"].(uint16))
		}
		if i == 1 {
			width = int(p.at % 2)
		}
		if err = p.literal(body, name, bc[len(specs)+i], "raw", width, true); err != nil {
			return nil, err
		}
		// A present, zero-width raw leaf is an observed empty buffer, not JSON null.
		if b := body[name].([]byte); len(b) == 0 {
			body[name] = []byte{}
		}
	}
	if p.at != uint64(len(raw)) {
		return nil, fmt.Errorf("CIFS setup projection: incomplete source coverage")
	}
	for _, name := range []string{"Native OS", "Native LAN Manager"} {
		key := name + " Raw Bytes"
		b := body[key].([]byte)
		if len(b) < 2 || b[len(b)-1] != 0 || b[len(b)-2] != 0 {
			return nil, fmt.Errorf("CIFS setup projection: missing Unicode terminator")
		}
		text, e := stream_parser.DecodeCIFSUTF16(b[:len(b)-2])
		if e != nil || strings.IndexByte(text, 0) >= 0 {
			return nil, fmt.Errorf("CIFS setup projection: invalid Unicode string")
		}
		body[name] = text
		span := body["Wire Field Ranges"].(map[string]any)[key].([]any)
		cifsProjectionRange(body, name, [2]uint64{span[0].(uint64), span[1].(uint64) - 2})
	}
	fields["Body"], fields["Packet Name"] = body, "SMB_COM_SESSION_SETUP_ANDX"
	fields["Response"], fields["Extended Security"] = response, true
	fields["String Encoding"], fields["AndX Offset Ignored"] = "UTF-16LE", true
	fields["Authentication Verified"], fields["Signature Verified"], fields["Session Established"] = false, false, false
	fields["Trailing Extension Status"] = "absent"
	if len(body["Trailing Extension Bytes"].([]byte)) != 0 {
		fields["Trailing Extension Status"] = "opaque: unclassified capture extension"
	}
	phase := "request"
	if response {
		phase = "success observed"
		if header["Status"].(uint32) != 0 {
			phase = "continuation required"
		}
	}
	fields["Setup Phase"] = phase
	return fields, nil
}
