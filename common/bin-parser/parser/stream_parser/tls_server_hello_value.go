package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// RFC 5246 7.4.1.3 and RFC 8446 4.1.3. This is an explicitly selected,
// complete, plaintext TLS 1.2/1.3 ServerHello layout, not a TLS state machine.
// All spans address input bytes. Unknown extensions and cryptographic values
// remain opaque. The special HelloRetryRequest layout is outside this profile.
const (
	tlsServerHelloMaxHandshake = 65611 // 4 + 2 + 32 + 1 + 32 + 2 + 1 + 2 + 65535
	tlsServerHelloMaxRecord    = 16389 // TLSPlaintext header + 2^14 fragment
	tlsServerHelloMaxEntries   = 1024
)

type tlsServerHelloField struct {
	Name, Type string
	Start, End int
	Children   []tlsServerHelloField
	List       bool
}

func tlsServerHelloLeaf(name, typ string, start, end int) tlsServerHelloField {
	return tlsServerHelloField{Name: name, Type: typ, Start: start, End: end}
}

type tlsServerHelloCursor struct {
	wire []byte
	at   int
	end  int
}

func (c *tlsServerHelloCursor) take(out *[]tlsServerHelloField, name, typ string, n int) ([]byte, error) {
	if n < 0 || n > c.end-c.at {
		return nil, fmt.Errorf("tls-server-hello: truncated %s at byte %d", name, c.at)
	}
	start := c.at
	c.at += n
	*out = append(*out, tlsServerHelloLeaf(name, typ, start, c.at))
	return c.wire[start:c.at], nil
}

func (c *tlsServerHelloCursor) length(out *[]tlsServerHelloField, name string, width int) (int, error) {
	b, err := c.take(out, name, fmt.Sprintf("uint%d", width*8), width)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, v := range b {
		n = n<<8 | int(v)
	}
	return n, nil
}

func decodeTLSServerHello(wire []byte, record bool) ([]tlsServerHelloField, map[string]any, error) {
	fail := func(s string) ([]tlsServerHelloField, map[string]any, error) {
		return nil, nil, fmt.Errorf("tls-server-hello: %s", s)
	}
	max := tlsServerHelloMaxHandshake
	if record {
		max = tlsServerHelloMaxRecord
	}
	if len(wire) == 0 || len(wire) > max {
		return fail("explicit complete message size outside profile")
	}
	var fields []tlsServerHelloField
	start := 0
	if record {
		if len(wire) < 9 {
			return fail("truncated record or handshake header")
		}
		if wire[0] != 22 {
			return fail("record is not plaintext handshake content")
		}
		v := binary.BigEndian.Uint16(wire[1:3])
		if v < 0x0301 || v > 0x0303 {
			return fail("record version outside profile")
		}
		if int(binary.BigEndian.Uint16(wire[3:5])) != len(wire)-5 {
			return fail("record length differs from exact boundary")
		}
		fields = append(fields, tlsServerHelloLeaf("Content Type", "uint8", 0, 1), tlsServerHelloLeaf("Record Version", "uint16", 1, 3), tlsServerHelloLeaf("Record Length", "uint16", 3, 5))
		start = 5
	}
	if len(wire)-start < 4 || wire[start] != 2 {
		return fail("complete ServerHello handshake header required")
	}
	size := int(wire[start+1])<<16 | int(wire[start+2])<<8 | int(wire[start+3])
	end := start + 4 + size
	if end > len(wire) || (!record && end != len(wire)) {
		return fail("handshake length differs from exact boundary or first complete message")
	}
	hello := tlsServerHelloField{Name: "ServerHello", Start: start, End: end}
	hello.Children = append(hello.Children, tlsServerHelloLeaf("Handshake Type", "uint8", start, start+1), tlsServerHelloLeaf("Handshake Length", "uint32", start+1, start+4))
	c := tlsServerHelloCursor{wire: wire, at: start + 4, end: end}
	version, err := c.length(&hello.Children, "Hello Version", 2)
	if err != nil {
		return nil, nil, err
	}
	if version != 0x0303 {
		return fail("only TLS 1.2 and TLS 1.3 legacy_version 0x0303 are in profile")
	}
	random, err := c.take(&hello.Children, "Random", "raw", 32)
	if err != nil {
		return nil, nil, err
	}
	hrr := []byte{0xcf, 0x21, 0xad, 0x74, 0xe5, 0x9a, 0x61, 0x11, 0xbe, 0x1d, 0x8c, 0x02, 0x1e, 0x65, 0xb8, 0x91, 0xc2, 0xa2, 0x11, 0x16, 0x7a, 0xbb, 0x8c, 0x5e, 0x07, 0x9e, 0x09, 0xe2, 0xc8, 0xa8, 0x33, 0x9c}
	if bytes.Equal(random, hrr) {
		return fail("HelloRetryRequest is outside ordinary ServerHello profile")
	}
	sid, err := c.length(&hello.Children, "Session ID Length", 1)
	if err != nil {
		return nil, nil, err
	}
	if sid > 32 {
		return fail("Session ID length exceeds 32")
	}
	if _, err = c.take(&hello.Children, "Session ID", "raw", sid); err != nil {
		return nil, nil, err
	}
	if _, err = c.take(&hello.Children, "Cipher Suite", "uint16", 2); err != nil {
		return nil, nil, err
	}
	compression, err := c.length(&hello.Children, "Compression Method", 1)
	if err != nil {
		return nil, nil, err
	}
	types := make(map[int]bool)
	var unknown []uint16
	selectedVersion := 0
	if c.at != c.end {
		n, err := c.length(&hello.Children, "Extensions Length", 2)
		if err != nil {
			return nil, nil, err
		}
		if n != c.end-c.at {
			return fail("extensions vector length differs from handshake boundary")
		}
		extensions := tlsServerHelloField{Name: "Extensions", Start: c.at, End: c.end, List: true}
		for c.at < c.end {
			if len(extensions.Children) >= tlsServerHelloMaxEntries {
				return fail("extension count exceeds 1024")
			}
			ext := tlsServerHelloField{Name: "Extension", Start: c.at}
			typ, err := c.length(&ext.Children, "Extension Type", 2)
			if err != nil {
				return nil, nil, err
			}
			if types[typ] {
				return fail("duplicate extension type")
			}
			types[typ] = true
			n, err := c.length(&ext.Children, "Extension Length", 2)
			if err != nil {
				return nil, nil, err
			}
			if n > c.end-c.at {
				return fail("extension exceeds enclosing vector")
			}
			ext.End = c.at + n
			sub := tlsServerHelloCursor{wire: wire, at: c.at, end: ext.End}
			known, err := tlsServerHelloExtension(&sub, &ext.Children, typ)
			if err != nil {
				return nil, nil, err
			}
			if !known {
				unknown = append(unknown, uint16(typ))
			}
			if sub.at != sub.end {
				return fail("extension contains trailing bytes")
			}
			if typ == 43 {
				selectedVersion = int(binary.BigEndian.Uint16(wire[c.at:ext.End]))
			}
			c.at = ext.End
			extensions.Children = append(extensions.Children, ext)
		}
		hello.Children = append(hello.Children, extensions)
	}
	tls13 := selectedVersion == 0x0304
	if types[43] && !tls13 {
		return fail("supported_versions value outside TLS 1.3 profile")
	}
	if tls13 {
		if compression != 0 {
			return fail("TLS 1.3 legacy compression must be zero")
		}
		if record && binary.BigEndian.Uint16(wire[1:3]) != 0x0303 {
			return fail("TLS 1.3 record legacy version must be 0x0303")
		}
		if !types[51] && !types[41] {
			return fail("TLS 1.3 ServerHello requires key_share or pre_shared_key")
		}
		for _, typ := range []int{0, 11, 15, 16, 18, 23, 35, 65281} {
			if types[typ] {
				return fail("known extension is not permitted in TLS 1.3 ServerHello")
			}
		}
	} else if types[51] || types[41] {
		return fail("TLS 1.3 extension requires supported_versions 0x0304")
	}
	fields = append(fields, hello)
	if end < len(wire) {
		fields = append(fields, tlsServerHelloLeaf("Following Handshake Bytes", "raw", end, len(wire)))
	}
	profile := "TLS 1.2 ServerHello layout"
	if tls13 {
		profile = "TLS 1.3 ServerHello layout"
	}
	info := map[string]any{
		"Profile": profile, "Explicit Caller Selected Profile": true,
		"Record Wrapped": record, "ParsedFirstHandshakeOnly": record,
		"Following Handshake Byte Count": len(wire) - end, "Unknown Extension Types": unknown,
		"HelloRetryRequest": false, "Negotiation Validated": false, "Handshake Completion Validated": false,
		"Peer Identity Validated": false, "Key Exchange Validated": false, "Certificate Trust Validated": false,
		"Cipher Suite Semantics Validated": false, "Extension Offer Correlation Validated": false,
		"Extension Sender Conformance Validated": false,
		"Unknown Extension Semantics Validated":  false, "TCP Reassembly Performed": false,
	}
	if tls13 {
		info["Supported Versions Wire Value"] = uint16(selectedVersion)
	}
	return fields, info, nil
}

// Extension encodings: RFCs 6066, 4492/8422, 6520, 7301, 6962,
// 7627, 5077, 8446 and 5746. No peer state or signatures are validated.
func tlsServerHelloExtension(c *tlsServerHelloCursor, out *[]tlsServerHelloField, typ int) (bool, error) {
	fail := func(s string) (bool, error) {
		return true, fmt.Errorf("tls-server-hello: extension %d %s at byte %d", typ, s, c.at)
	}
	switch typ {
	case 0, 23, 35:
		if c.at != c.end {
			return fail("must be empty in ServerHello")
		}
		_, err := c.take(out, "Empty Extension Data", "raw", 0)
		return true, err
	case 11, 65281:
		name, data := "Point Formats Length", "Point Formats"
		if typ == 65281 {
			name, data = "Renegotiated Connection Length", "Renegotiated Connection"
		}
		n, err := c.length(out, name, 1)
		if err != nil {
			return true, err
		}
		if n != c.end-c.at || (typ == 11 && n == 0) {
			return fail("invalid one-byte vector length")
		}
		if typ == 65281 {
			_, err = c.take(out, data, "raw", n)
			return true, err
		}
		list := tlsServerHelloField{Name: data, Start: c.at, End: c.end, List: true}
		for c.at < c.end {
			if _, err = c.take(&list.Children, "Point Format", "uint8", 1); err != nil {
				return true, err
			}
		}
		*out = append(*out, list)
		return true, nil
	case 15:
		mode, err := c.length(out, "Heartbeat Mode", 1)
		if err != nil {
			return true, err
		}
		if mode != 1 && mode != 2 {
			return fail("invalid heartbeat mode")
		}
		return true, nil
	case 16:
		n, err := c.length(out, "ALPN List Length", 2)
		if err != nil {
			return true, err
		}
		if n < 2 || n != c.end-c.at {
			return fail("invalid ALPN list length")
		}
		n, err = c.length(out, "ALPN Protocol Length", 1)
		if err != nil {
			return true, err
		}
		if n == 0 || n != c.end-c.at {
			return fail("ServerHello ALPN requires exactly one nonempty protocol")
		}
		_, err = c.take(out, "ALPN Protocol", "raw", n)
		return true, err
	case 18:
		n, err := c.length(out, "SCT List Length", 2)
		if err != nil {
			return true, err
		}
		if n == 0 || n != c.end-c.at {
			return fail("invalid SCT list length")
		}
		list := tlsServerHelloField{Name: "Signed Certificate Timestamps", Start: c.at, End: c.end, List: true}
		for c.at < c.end {
			if len(list.Children) >= tlsServerHelloMaxEntries {
				return fail("SCT count exceeds 1024")
			}
			sct := tlsServerHelloField{Name: "Signed Certificate Timestamp", Start: c.at}
			n, err := c.length(&sct.Children, "SCT Length", 2)
			if err != nil {
				return true, err
			}
			if n == 0 || n > c.end-c.at {
				return fail("invalid SCT entry length")
			}
			sct.End = c.at + n
			sub := tlsServerHelloCursor{wire: c.wire, at: c.at, end: sct.End}
			version, err := sub.length(&sct.Children, "SCT Version", 1)
			if err != nil {
				return true, err
			}
			if version != 0 {
				_, err = sub.take(&sct.Children, "Unknown SCT Version Data", "raw", sub.end-sub.at)
			} else {
				if _, err = sub.take(&sct.Children, "SCT Log ID", "raw", 32); err != nil {
					return true, err
				}
				if _, err = sub.take(&sct.Children, "SCT Timestamp", "uint64", 8); err != nil {
					return true, err
				}
				length, err := sub.length(&sct.Children, "SCT Extensions Length", 2)
				if err != nil {
					return true, err
				}
				if _, err = sub.take(&sct.Children, "SCT Extensions", "raw", length); err != nil {
					return true, err
				}
				if _, err = sub.take(&sct.Children, "SCT Hash Algorithm", "uint8", 1); err != nil {
					return true, err
				}
				if _, err = sub.take(&sct.Children, "SCT Signature Algorithm", "uint8", 1); err != nil {
					return true, err
				}
				length, err = sub.length(&sct.Children, "SCT Signature Length", 2)
				if err != nil {
					return true, err
				}
				_, err = sub.take(&sct.Children, "SCT Signature", "raw", length)
			}
			if err != nil {
				return true, err
			}
			if sub.at != sub.end {
				return fail("SCT contains trailing bytes")
			}
			c.at = sct.End
			list.Children = append(list.Children, sct)
		}
		*out = append(*out, list)
		return true, nil
	case 41:
		_, err := c.take(out, "Selected PSK Identity", "uint16", 2)
		return true, err
	case 43:
		_, err := c.take(out, "Supported Version", "uint16", 2)
		return true, err
	case 51:
		if _, err := c.take(out, "Key Share Group", "uint16", 2); err != nil {
			return true, err
		}
		n, err := c.length(out, "Key Exchange Length", 2)
		if err != nil {
			return true, err
		}
		if n == 0 || n != c.end-c.at {
			return fail("invalid key_exchange vector length")
		}
		_, err = c.take(out, "Key Exchange", "raw", n)
		return true, err
	default:
		_, err := c.take(out, "Unknown Extension Data", "raw", c.end-c.at)
		return false, err
	}
}
