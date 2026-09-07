package stream_parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RFC 2743 section 3.1; RFC 4178 sections 3.1, 4.2 and Appendix A.
// This is a bounded SPNEGO structural profile, not every GSS mechanism.
// Mechanism tokens and MICs are opaque: neither is interpreted or verified.
// The resource limits below are implementation limits, not protocol maxima.
const gssapiMaxBytes = 65535
const gssapiMaxMechanisms = 64
const gssapiSPNEGOOID = "1.3.6.1.5.5.2"

type gssapiField struct {
	Name, Type string
	Start, End int // Bytes relative to this decoded token, never HTTP offsets.
	Info       map[string]any
	Children   []gssapiField
}

type gssapiTLV struct{ start, body, end int }
type gssapiDecoder struct{ wire []byte }

func (d gssapiDecoder) tlv(at, limit int, tag byte) (gssapiTLV, error) {
	if at < 0 || limit > len(d.wire) || at > limit || limit-at < 2 {
		return gssapiTLV{}, fmt.Errorf("gssapi: truncated DER header")
	}
	if d.wire[at] != tag {
		return gssapiTLV{}, fmt.Errorf("gssapi: expected DER tag 0x%02x at byte %d", tag, at)
	}
	body, length := at+2, int(d.wire[at+1])
	if length >= 128 {
		n := length & 127
		if n == 0 {
			return gssapiTLV{}, fmt.Errorf("gssapi: indefinite DER length")
		}
		// The whole bounded input is at most 65535 bytes. More than two
		// length octets cannot be a minimal encoding within this profile.
		if n > 2 || limit-body < n || d.wire[body] == 0 {
			return gssapiTLV{}, fmt.Errorf("gssapi: nonminimal or oversized DER length")
		}
		length = 0
		for _, b := range d.wire[body : body+n] {
			length = length*256 + int(b)
		}
		body += n
		if length < 128 {
			return gssapiTLV{}, fmt.Errorf("gssapi: nonminimal DER length")
		}
	}
	if length > limit-body {
		return gssapiTLV{}, fmt.Errorf("gssapi: DER value exceeds enclosing boundary")
	}
	return gssapiTLV{at, body, body + length}, nil
}

func (d gssapiDecoder) group(name string, t gssapiTLV, children ...gssapiField) gssapiField {
	f := gssapiField{Name: name, Start: t.start, End: t.end}
	f.Children = append(f.Children,
		gssapiField{Name: "Tag", Type: "uint8", Start: t.start, End: t.start + 1},
		gssapiField{Name: "DER Length", Type: "raw", Start: t.start + 1, End: t.body, Info: map[string]any{"Value Length": t.end - t.body}},
	)
	f.Children = append(f.Children, children...)
	return f
}

func gssapiOID(value []byte) (string, error) {
	if len(value) == 0 || len(value) > 128 {
		return "", fmt.Errorf("gssapi: OID must contain 1..128 bytes in this profile")
	}
	arcs := []string{}
	for at := 0; at < len(value); {
		if value[at] == 0x80 {
			return "", fmt.Errorf("gssapi: nonminimal OID component")
		}
		var v uint64
		for {
			if at >= len(value) {
				return "", fmt.Errorf("gssapi: unterminated OID component")
			}
			b := value[at]
			at++
			if v > (math.MaxUint64-uint64(b&127))/128 {
				return "", fmt.Errorf("gssapi: OID component exceeds uint64 profile")
			}
			v = v*128 + uint64(b&127)
			if b&128 == 0 {
				break
			}
		}
		if len(arcs) == 0 {
			first := uint64(2)
			if v < 80 {
				first = v / 40
			}
			arcs = append(arcs, strconv.FormatUint(first, 10), strconv.FormatUint(v-first*40, 10))
		} else {
			arcs = append(arcs, strconv.FormatUint(v, 10))
		}
		if len(arcs) > 64 {
			return "", fmt.Errorf("gssapi: OID exceeds 64-component profile")
		}
	}
	return strings.Join(arcs, "."), nil
}

func (d gssapiDecoder) oid(name string, at, limit int) (gssapiField, string, error) {
	t, err := d.tlv(at, limit, 6)
	if err != nil {
		return gssapiField{}, "", err
	}
	value, err := gssapiOID(d.wire[t.body:t.end])
	if err != nil {
		return gssapiField{}, "", err
	}
	f := d.group(name, t, gssapiField{Name: "OID Bytes", Type: "raw", Start: t.body, End: t.end, Info: map[string]any{"OID": value}})
	f.Info = map[string]any{"OID": value}
	return f, value, nil
}

func (d gssapiDecoder) explicitValue(name, valueName string, t gssapiTLV, tag byte) (gssapiField, gssapiTLV, error) {
	inner, err := d.tlv(t.body, t.end, tag)
	if err != nil {
		return gssapiField{}, inner, err
	}
	if inner.end != t.end {
		return gssapiField{}, inner, fmt.Errorf("gssapi: trailing bytes inside explicit field")
	}
	f := d.group(name, t, d.group(valueName, inner, gssapiField{Name: "Value", Type: "raw", Start: inner.body, End: inner.end}))
	return f, inner, nil
}

func (d gssapiDecoder) negotiation(at, limit int, initial bool) (gssapiField, map[string]any, error) {
	tag, kind := byte(0xa1), "NegTokenResp"
	if initial {
		tag, kind = 0xa0, "NegTokenInit"
	}
	choice, err := d.tlv(at, limit, tag)
	if err != nil {
		return gssapiField{}, nil, err
	}
	if choice.end != limit {
		return gssapiField{}, nil, fmt.Errorf("gssapi: trailing bytes after negotiation token")
	}
	seq, err := d.tlv(choice.body, choice.end, 0x30)
	if err != nil {
		return gssapiField{}, nil, err
	}
	if seq.end != choice.end {
		return gssapiField{}, nil, fmt.Errorf("gssapi: trailing bytes after negotiation sequence")
	}
	info := map[string]any{"Token Kind": kind}
	fields := []gssapiField{}
	previous := -1
	for at = seq.body; at < seq.end; {
		index := int(d.wire[at]) - 0xa0
		if index < 0 || index > 3 {
			return gssapiField{}, nil, fmt.Errorf("gssapi: unsupported negotiation extension")
		}
		if index <= previous {
			return gssapiField{}, nil, fmt.Errorf("gssapi: duplicate or out-of-order negotiation field")
		}
		previous = index
		t, err := d.tlv(at, seq.end, byte(0xa0+index))
		if err != nil {
			return gssapiField{}, nil, err
		}
		var field gssapiField
		switch {
		case initial && index == 0:
			list, err := d.tlv(t.body, t.end, 0x30)
			if err != nil {
				return gssapiField{}, nil, err
			}
			if list.end != t.end || list.body == list.end {
				return gssapiField{}, nil, fmt.Errorf("gssapi: mechanism list must be nonempty and complete")
			}
			oids, names := []gssapiField{}, []string{}
			for pos := list.body; pos < list.end; {
				if len(oids) == gssapiMaxMechanisms {
					return gssapiField{}, nil, fmt.Errorf("gssapi: mechanism list exceeds 64-entry profile")
				}
				oid, name, err := d.oid(fmt.Sprintf("Mechanism Type %d", len(oids)), pos, list.end)
				if err != nil {
					return gssapiField{}, nil, err
				}
				if name == gssapiSPNEGOOID {
					return gssapiField{}, nil, fmt.Errorf("gssapi: SPNEGO cannot offer itself")
				}
				oids, names, pos = append(oids, oid), append(names, name), oid.End
			}
			field = d.group("Mechanism Types", t, d.group("Mechanism List", list, oids...))
			info["Offered Mechanisms"] = names
		case initial && index == 1:
			var value gssapiTLV
			field, value, err = d.explicitValue("Request Flags", "Bit String", t, 3)
			if err != nil {
				return gssapiField{}, nil, err
			}
			b := d.wire[value.body:value.end]
			if len(b) == 0 || b[0] > 7 || (len(b) == 1 && b[0] != 0) || (len(b)-1)*8-int(b[0]) > 32 {
				return gssapiField{}, nil, fmt.Errorf("gssapi: invalid ContextFlags bit string")
			}
			if len(b) > 1 {
				// Named-bit DER omits all trailing zero bits, even with SIZE(32).
				mask := byte(1<<b[0]) - 1
				if b[len(b)-1]&mask != 0 || b[len(b)-1]&(1<<b[0]) == 0 {
					return gssapiField{}, nil, fmt.Errorf("gssapi: nonminimal ContextFlags or nonzero unused bits")
				}
			}
			flags := map[string]any{"Meaningful Bits": (len(b)-1)*8 - int(b[0])}
			for bit, name := range []string{"Delegation", "Mutual", "Replay", "Sequence", "Anonymous", "Confidentiality", "Integrity"} {
				flags[name] = len(b) > 1 && b[1]&(0x80>>bit) != 0
			}
			field.Info, info["Request Flags"] = flags, flags
		case !initial && index == 0:
			var value gssapiTLV
			field, value, err = d.explicitValue("Negotiation State", "Enumerated", t, 10)
			if err != nil {
				return gssapiField{}, nil, err
			}
			if value.end-value.body != 1 || d.wire[value.body] > 3 {
				return gssapiField{}, nil, fmt.Errorf("gssapi: unsupported negotiation state")
			}
			state := d.wire[value.body]
			field.Children[2].Children[2].Type = "uint8"
			field.Info = map[string]any{"Declared State": int(state), "State Name": []string{"accept-completed", "accept-incomplete", "reject", "request-mic"}[state]}
			info["Declared State"] = int(state)
		case !initial && index == 1:
			oid, name, err := d.oid("Selected Mechanism OID", t.body, t.end)
			if err != nil {
				return gssapiField{}, nil, err
			}
			if oid.End != t.end || name == gssapiSPNEGOOID {
				return gssapiField{}, nil, fmt.Errorf("gssapi: invalid selected mechanism")
			}
			field = d.group("Supported Mechanism", t, oid)
			info["Selected Mechanism"] = name
		default:
			name := "Mechanism List MIC"
			if index == 2 {
				name = "Response Mechanism Token"
				if initial {
					name = "Optimistic Mechanism Token"
				}
			}
			field, _, err = d.explicitValue(name, "Octet String", t, 4)
			if err != nil {
				return gssapiField{}, nil, err
			}
			field.Info = map[string]any{"Semantics Decoded": false, "Verified": false}
		}
		fields, at = append(fields, field), t.end
	}
	if initial {
		if _, ok := info["Offered Mechanisms"]; !ok {
			return gssapiField{}, nil, fmt.Errorf("gssapi: NegTokenInit requires mechanism list")
		}
	}
	return d.group(kind, choice, d.group("Negotiation Sequence", seq, fields...)), info, nil
}

func decodeGSSAPIToken(wire []byte) ([]gssapiField, map[string]any, error) {
	if len(wire) < 4 || len(wire) > gssapiMaxBytes {
		return nil, nil, fmt.Errorf("gssapi: token must contain 4..65535 bytes in this profile")
	}
	d := gssapiDecoder{wire: wire}
	var fields []gssapiField
	var info map[string]any
	if wire[0] == 0x60 {
		outer, err := d.tlv(0, len(wire), 0x60)
		if err != nil {
			return nil, nil, err
		}
		if outer.end != len(wire) {
			return nil, nil, fmt.Errorf("gssapi: trailing bytes after initial context token")
		}
		oid, mechanism, err := d.oid("Mechanism OID", outer.body, outer.end)
		if err != nil {
			return nil, nil, err
		}
		if mechanism != gssapiSPNEGOOID {
			return nil, nil, fmt.Errorf("gssapi: unsupported outer mechanism OID %s", mechanism)
		}
		if oid.End == outer.end {
			return nil, nil, fmt.Errorf("gssapi: missing SPNEGO negotiation token after mechanism OID")
		}
		var negotiation gssapiField
		negotiation, info, err = d.negotiation(oid.End, outer.end, true)
		if err != nil {
			return nil, nil, err
		}
		fields = []gssapiField{d.group("Initial Context Token", outer, oid, negotiation)}
	} else {
		negotiation, parsed, err := d.negotiation(0, len(wire), false)
		if err != nil {
			return nil, nil, err
		}
		fields, info = []gssapiField{negotiation}, parsed
	}
	info["Profile"] = "GSS-API SPNEGO bounded DER structural fields"
	info["Mechanism OID"] = gssapiSPNEGOOID
	info["Outer Mechanism OID Present"] = wire[0] == 0x60
	info["Mechanism OID Source"] = "explicit SPNEGO entry profile"
	if wire[0] == 0x60 {
		info["Mechanism OID Source"] = "initial token wire OID"
	}
	info["Maximum Token Bytes"] = gssapiMaxBytes
	info["Mechanism Semantics Decoded"] = false
	info["MIC Verified"] = false
	info["Context State Validated"] = false
	info["Negotiation Outcome Validated"] = false
	return fields, info, nil
}

// RFC 4559 HTTP tokens are a different coordinate system from decoded DER.
// Return metadata, not base.Nodes: attaching decoded leaves to HTTP wire spans
// would manufacture offsets and break both Result and NodeToBytes.
func inspectGSSAPIBase64(encoded string) (map[string]any, error) {
	if len(encoded) == 0 || len(encoded) > base64.StdEncoding.EncodedLen(gssapiMaxBytes) {
		return nil, fmt.Errorf("gssapi: Base64 token exceeds bounded profile or is empty")
	}
	for _, b := range []byte(encoded) {
		if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '/' || b == '=') {
			return nil, fmt.Errorf("gssapi: invalid Base64 wire character")
		}
	}
	wire, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(wire) != encoded {
		return nil, fmt.Errorf("gssapi: invalid or noncanonical padded Base64")
	}
	fields, info, err := decodeGSSAPIToken(wire)
	if err != nil {
		return nil, err
	}
	var convert func([]gssapiField) []any
	convert = func(fields []gssapiField) []any {
		out := make([]any, 0, len(fields))
		for _, f := range fields {
			v := map[string]any{"Name": f.Name, "Type": f.Type, "Start Bit": uint64(f.Start) * 8, "End Bit": uint64(f.End) * 8}
			if f.Type == "raw" {
				v["Value"] = bytes.Clone(wire[f.Start:f.End])
			} else if f.Type == "uint8" {
				v["Value"] = uint64(wire[f.Start])
			} else {
				v["Children"] = convert(f.Children)
			}
			if f.Info != nil {
				v["Info"] = f.Info
			}
			out = append(out, v)
		}
		return out
	}
	info["Source Encoding"] = "base64"
	info["Span Coordinate System"] = "decoded-token-relative-bits"
	info["Decoded Bytes"] = wire
	info["Decoded Fields"] = convert(fields)
	return info, nil
}
