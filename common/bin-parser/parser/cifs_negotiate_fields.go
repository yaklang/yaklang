package parser

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// ErrCIFSProjectionBudget identifies the caller's dialect collection limit.
// The canonical rule independently limits a request to 1024 dialects.
var ErrCIFSProjectionBudget = errors.New("CIFS field projection collection budget exceeded")

const cifsProjectionMaxDialects = 1024

// ProjectCIFSNegotiateFields projects a successfully parsed, explicitly bounded
// CIFSDirectTCP or CIFSNegotiateRequest Node. It does not parse wire grammar or
// run output expressions. Ordered Node children, rather than NodeToMap's named
// children, preserve repeated dialects. The returned values own their storage.
//
// raw must be precisely the standalone PDU parsed into node, with bit origin
// zero. Each terminal's existing span and value are checked against that source;
// a different source, shifted tree or partial/mutated Node is rejected. Ranges
// are absolute, half-open byte offsets within raw. A dialect's result excludes
// its consumed NUL; that delimiter receives its own range. OEM codepages remain
// unselected: raw name bytes survive JSON and escaped display is not wire text.
// maxElements counts dialects only; zero allows no dialects.
func ProjectCIFSNegotiateFields(node *base.Node, raw []byte, maxElements int) (fields map[string]any, err error) {
	// Nodes and their public context are mutable. Invalid caller replacements
	// must be an error, including malformed internal result-position types.
	defer func() {
		if recovered := recover(); recovered != nil {
			fields, err = nil, fmt.Errorf("CIFS field projection: invalid parsed Node: %v", recovered)
		}
	}()
	if maxElements < 0 {
		return nil, ErrCIFSProjectionBudget
	}
	if node == nil || node.Cfg == nil || len(raw) > 65574 {
		return nil, fmt.Errorf("CIFS field projection: invalid bounded source")
	}
	p := cifsFieldProjection{raw: raw}
	fields = map[string]any{"Wire Field Ranges": map[string]any{}}
	request := node
	switch node.Name {
	case "CIFSDirectTCP":
		children, childErr := cifsProjectionChildren(node, "Direct TCP Zero", "Direct TCP Length", "Negotiate Request")
		if childErr != nil {
			return nil, childErr
		}
		direct := map[string]any{"Wire Field Ranges": map[string]any{}}
		if err = p.literal(direct, "Message Type", children[0], "uint8", 1, false); err != nil {
			return nil, err
		}
		if err = p.literal(direct, "Payload Length", children[1], "uint32", 3, false); err != nil {
			return nil, err
		}
		fields["Direct TCP"] = direct
		request = children[2]
	case "CIFSNegotiateRequest":
	default:
		return nil, fmt.Errorf("CIFS field projection: unselected entry %q", node.Name)
	}
	children, err := cifsProjectionChildren(request,
		"ProtocolId", "Command", "Status", "Flags", "Flags2", "PIDHigh", "SecurityFeatures", "Reserved", "TID", "PID", "UID", "MID", "WordCount", "ByteCount", "Dialects")
	if err != nil {
		return nil, err
	}
	dialects := children[14]
	if dialects.Cfg == nil || dialects.Cfg.ResultSettings().Present || len(dialects.Children) > cifsProjectionMaxDialects {
		return nil, fmt.Errorf("CIFS field projection: invalid dialect collection")
	}
	if len(dialects.Children) > maxElements {
		return nil, ErrCIFSProjectionBudget
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
		if err = p.literal(header, spec.name, children[i], spec.typ, spec.width, true); err != nil {
			return nil, err
		}
	}
	fields["Header"] = header
	if err = p.literal(fields, "Word Count", children[12], "uint8", 1, true); err != nil {
		return nil, err
	}
	if err = p.literal(fields, "Byte Count", children[13], "uint16", 2, true); err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(dialects.Children))
	upgrade := false
	for _, dialect := range dialects.Children {
		if dialect == nil || dialect.Name != "CIFSDialect" {
			return nil, fmt.Errorf("CIFS field projection: invalid dialect child")
		}
		parts, partErr := cifsProjectionChildren(dialect, "BufferFormat", "Dialect Name")
		if partErr != nil {
			return nil, partErr
		}
		row := map[string]any{"Wire Field Ranges": map[string]any{}}
		if err = p.literal(row, "Buffer Format", parts[0], "uint8", 1, true); err != nil {
			return nil, err
		}
		value, span, nameErr := p.terminal(parts[1], "string", -1)
		if nameErr != nil {
			return nil, nameErr
		}
		name, ok := value.(string)
		if !ok || !bytes.Equal([]byte(name), raw[span[0]:span[1]]) || stream_parser.CalcNodeConsumedLength(parts[1]) != (span[1]-span[0]+1)*8 || p.at >= uint64(len(raw)) || raw[p.at] != 0 {
			return nil, fmt.Errorf("CIFS field projection: dialect value or consumed delimiter does not match source")
		}
		nameBytes := make([]byte, span[1]-span[0])
		copy(nameBytes, raw[span[0]:span[1]])
		row["Raw Name Bytes"] = nameBytes
		cifsProjectionRange(row, "Raw Name Bytes", span)
		if cifsASCIIName(nameBytes) {
			row["Name"] = name
			row["Name Representation"] = "ASCII bytes; OEM codepage not inferred"
			cifsProjectionRange(row, "Name", span)
		} else {
			row["Name"] = cifsEscapedName(nameBytes)
			row["Name Representation"] = "escaped OEM bytes; codepage not inferred"
		}
		row["NUL Terminator"] = uint8(raw[p.at])
		cifsProjectionRange(row, "NUL Terminator", [2]uint64{p.at, p.at + 1})
		p.at++
		rows = append(rows, row)
		upgrade = upgrade || name == "SMB 2.002" || name == "SMB 2.???"
	}
	if p.at != uint64(len(raw)) {
		return nil, fmt.Errorf("CIFS field projection: parsed fields do not cover bounded source")
	}
	fields["Dialects"] = rows
	fields["Packet Name"] = "SMB_COM_NEGOTIATE"
	fields["SMB2 Upgrade Offered"] = upgrade
	fields["OEM Codepage Inferred"] = false
	fields["Dialect Negotiated"] = false
	fields["Session Established"] = false
	fields["Signature Verified"] = false
	return fields, nil
}

type cifsFieldProjection struct {
	raw []byte
	at  uint64
}

func cifsProjectionChildren(node *base.Node, names ...string) ([]*base.Node, error) {
	if node == nil || node.Cfg == nil || node.Cfg.ResultSettings().Present || len(node.Children) != len(names) {
		return nil, fmt.Errorf("CIFS field projection: unexpected composite shape")
	}
	for i, name := range names {
		if node.Children[i] == nil || node.Children[i].Name != name {
			return nil, fmt.Errorf("CIFS field projection: expected ordered child %q", name)
		}
	}
	return node.Children, nil
}

func (p *cifsFieldProjection) terminal(node *base.Node, typ string, width int) (any, [2]uint64, error) {
	var zero [2]uint64
	if node == nil || node.Cfg == nil || node.Ctx == nil || len(node.Children) != 0 {
		return nil, zero, fmt.Errorf("CIFS field projection: invalid terminal")
	}
	settings := node.Cfg.ResultSettings()
	bits, ok := settings.Position.([2]uint64)
	if !settings.Present || settings.RawPresent || !ok || settings.Type != typ || bits[0]%8 != 0 || bits[1]%8 != 0 || bits[0] > bits[1] || bits[0]/8 != p.at || bits[1]/8 > uint64(len(p.raw)) {
		return nil, zero, fmt.Errorf("CIFS field projection: invalid %q result span/type", node.Name)
	}
	span := [2]uint64{bits[0] / 8, bits[1] / 8}
	if width >= 0 && span[1]-span[0] != uint64(width) {
		return nil, zero, fmt.Errorf("CIFS field projection: incorrect %q result width", node.Name)
	}
	value := stream_parser.GetNodeResult(node)
	p.at = span[1]
	return value, span, nil
}

func (p *cifsFieldProjection) literal(owner map[string]any, key string, node *base.Node, typ string, width int, little bool) error {
	value, span, err := p.terminal(node, typ, width)
	if err != nil {
		return err
	}
	wire := p.raw[span[0]:span[1]]
	if typ == "raw" {
		b, ok := value.([]byte)
		if !ok || !bytes.Equal(b, wire) {
			return fmt.Errorf("CIFS field projection: %q bytes differ from source", key)
		}
		value = bytes.Clone(b)
	} else {
		var expected uint64
		for i := range wire {
			at := i
			if little {
				at = len(wire) - 1 - i
			}
			expected = expected<<8 | uint64(wire[at])
		}
		var actual uint64
		switch v := value.(type) {
		case uint8:
			if typ != "uint8" {
				return fmt.Errorf("CIFS field projection: %q scalar type differs", key)
			}
			actual = uint64(v)
		case uint16:
			if typ != "uint16" {
				return fmt.Errorf("CIFS field projection: %q scalar type differs", key)
			}
			actual = uint64(v)
		case uint32:
			if typ != "uint32" {
				return fmt.Errorf("CIFS field projection: %q scalar type differs", key)
			}
			actual = uint64(v)
		case uint64:
			if typ != "uint64" {
				return fmt.Errorf("CIFS field projection: %q scalar type differs", key)
			}
			actual = v
		case int16:
			if typ != "int16" || width != 2 {
				return fmt.Errorf("CIFS field projection: %q scalar type differs", key)
			}
			actual = uint64(uint16(v))
		default:
			return fmt.Errorf("CIFS field projection: %q is not a typed scalar", key)
		}
		if actual != expected {
			return fmt.Errorf("CIFS field projection: %q value differs from source", key)
		}
	}
	owner[key] = value
	cifsProjectionRange(owner, key, span)
	return nil
}

func cifsProjectionRange(owner map[string]any, key string, span [2]uint64) {
	owner["Wire Field Ranges"].(map[string]any)[key] = []any{span[0], span[1]}
}

func cifsASCIIName(name []byte) bool {
	for _, b := range name {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}

func cifsEscapedName(name []byte) string {
	const hex = "0123456789abcdef"
	var out strings.Builder
	out.Grow(len(name) * 4)
	for _, b := range name {
		out.WriteString("\\x")
		out.WriteByte(hex[b>>4])
		out.WriteByte(hex[b&15])
	}
	return out.String()
}

// Only the explicit, successful CIFS profiles use this projection. Carrier
// fallback, old SMB rules and custom parser registrations retain NodeToMap.
func cifsStructuredProjection(node *base.Node, raw []byte, rule string, keys []string) (map[string]any, bool, error) {
	if rule != "application-layer.cifs" || len(keys) != 1 || base.ParserRegistration("default") != defaultParserRegistration {
		return nil, false, nil
	}
	switch keys[0] {
	case "CIFSDirectTCPSessionSetupExtendedRequest", "CIFSDirectTCPSessionSetupExtendedResponse":
		fields, err := ProjectCIFSSessionSetupFields(node, raw)
		return fields, true, err
	case "CIFSDirectTCP", "CIFSNegotiateRequest":
		fields, err := ProjectCIFSNegotiateFields(node, raw, cifsProjectionMaxDialects)
		return fields, true, err
	case "CIFSDirectTCPNegotiateExtendedResponse", "CIFSNegotiateExtendedResponse":
		fields, err := ProjectCIFSNegotiateResponseFields(node, raw)
		return fields, true, err
	default:
		return nil, false, nil
	}
}
