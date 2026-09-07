package stream_parser

import (
	"fmt"
	"strconv"
	"strings"
)

// RFC 9114 §§4,6,7. Input is an explicit, contiguous decrypted stream slice
// starting at offset zero, NOT a QUIC packet. Resource values are this
// implementation's bounds, not negotiated or universal protocol maxima.
const http3MaxBytes = 1048576
const http3MaxItems = 4096
const http3MaxString = 65536
const http3MaxInteger = uint64(1<<62 - 1)

type http3Field struct {
	Name, Type string
	Start, End int // bits in the input stream, not encrypted packet positions
	Info       map[string]any
	Children   []http3Field
}

func http3Leaf(name, typ string, start, end int) http3Field {
	return http3Field{Name: name, Type: typ, Start: start, End: end}
}

func http3Group(name string, start, end int, fields ...http3Field) http3Field {
	return http3Field{Name: name, Start: start, End: end, Children: fields}
}

type http3Cursor struct {
	wire    []byte
	at, end int
}

func (c *http3Cursor) integer(name string) (uint64, http3Field, error) {
	start := c.at
	if start >= c.end {
		return 0, http3Field{}, fmt.Errorf("http3: truncated %s", name)
	}
	width := 1 << (c.wire[start] >> 6)
	if width > c.end-start {
		return 0, http3Field{}, fmt.Errorf("http3: truncated %s integer", name)
	}
	v := uint64(c.wire[start] & 63)
	for _, b := range c.wire[start+1 : start+width] {
		v = v<<8 | uint64(b)
	}
	c.at += width
	f := http3Group(name, start*8, c.at*8, http3Leaf("Length Prefix", "uint8", start*8, start*8+2), http3Leaf("Value", "uint64", start*8+2, c.at*8))
	f.Info = map[string]any{"Decoded Value": v, "Encoded Bytes": width}
	return v, f, nil
}

func http3Token(s string) bool {
	if s == "" {
		return false
	}
	for _, b := range []byte(s) {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b))) {
			return false
		}
	}
	return true
}

func http3RequestFields(headers []map[string]any, trailer bool) (map[string]any, error) {
	info := map[string]any{}
	pseudo := map[string]string{}
	ordinary := false
	contentLength := ""
	for _, h := range headers {
		name, value := h["Name"].(string), h["Value"].(string)
		if strings.ContainsAny(value, "\x00\r\n") || strings.Trim(value, " \t") != value {
			return nil, fmt.Errorf("http3: invalid HTTP field value")
		}
		for _, b := range []byte(value) {
			if b < 0x20 && b != '\t' || b == 0x7f {
				return nil, fmt.Errorf("http3: prohibited control octet in HTTP field value")
			}
		}
		if strings.HasPrefix(name, ":") {
			if trailer || ordinary {
				return nil, fmt.Errorf("http3: pseudo-header after ordinary fields or in trailers")
			}
			if _, present := pseudo[name]; present {
				return nil, fmt.Errorf("http3: repeated pseudo-header")
			}
			if name != ":method" && name != ":scheme" && name != ":authority" && name != ":path" {
				return nil, fmt.Errorf("http3: unsupported request pseudo-header")
			}
			pseudo[name] = value
		} else {
			ordinary = true
			if !http3Token(name) || name != strings.ToLower(name) {
				return nil, fmt.Errorf("http3: HTTP field names must be lowercase tokens")
			}
			switch name {
			case "connection", "proxy-connection", "keep-alive", "transfer-encoding", "upgrade":
				return nil, fmt.Errorf("http3: prohibited connection-specific field")
			case "te":
				if !strings.EqualFold(value, "trailers") {
					return nil, fmt.Errorf("http3: TE requires trailers")
				}
			case "content-length":
				if trailer || contentLength != "" {
					return nil, fmt.Errorf("http3: Content-Length duplicate/trailer outside single-value profile")
				}
				if value == "" || strings.Trim(value, "0123456789") != "" {
					return nil, fmt.Errorf("http3: invalid Content-Length")
				}
				contentLength = value
			}
		}
	}
	if !trailer {
		if !http3Token(pseudo[":method"]) || pseudo[":method"] == "CONNECT" {
			return nil, fmt.Errorf("http3: requires ordinary request method; CONNECT outside profile")
		}
		if !strings.EqualFold(pseudo[":scheme"], "http") && !strings.EqualFold(pseudo[":scheme"], "https") {
			return nil, fmt.Errorf("http3: http/https scheme profile required")
		}
		if pseudo[":authority"] == "" || strings.ContainsAny(pseudo[":authority"], "/?#@\\") {
			return nil, fmt.Errorf("http3: :authority required by request profile")
		}
		path := pseudo[":path"]
		if path == "*" && pseudo[":method"] == "OPTIONS" {
			if _, err := decodeHTTPServiceURL(pseudo[":scheme"] + "://" + pseudo[":authority"] + "/"); err != nil {
				return nil, fmt.Errorf("http3: invalid request authority")
			}
		} else {
			if !strings.HasPrefix(path, "/") {
				return nil, fmt.Errorf("http3: nonempty origin path required")
			}
			if _, err := decodeHTTPServiceURL(pseudo[":scheme"] + "://" + pseudo[":authority"] + path); err != nil {
				return nil, fmt.Errorf("http3: request authority/path outside URI profile: %w", err)
			}
		}
		for k, v := range pseudo {
			info[k] = v
		}
	}
	if contentLength != "" {
		v, err := strconv.ParseUint(contentLength, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("http3: Content-Length exceeds uint64 profile")
		}
		info["Content Length"] = v
	}
	return info, nil
}

func decodeHTTP3Stream(wire []byte, mode string, encoder []byte, maxCapacity uint64) ([]http3Field, map[string]any, error) {
	if len(wire) == 0 || len(wire) > http3MaxBytes || len(encoder) > http3MaxBytes || maxCapacity > http3MaxBytes {
		return nil, nil, fmt.Errorf("http3: input/table exceeds bounded implementation profile")
	}
	info := map[string]any{"Profile": "HTTP/3 decrypted contiguous stream fields", "Input Coordinate System": "decrypted-stream-relative-bits", "QUIC Decryption Performed": false, "QUIC Reassembly Performed": false, "Connection State Validated": false, "Endpoint Identity Proven": false, "Transport FIN Observed": false, "Maximum Stream Bytes": http3MaxBytes, "Maximum Items": http3MaxItems, "QPACK Maximum Capacity": maxCapacity}
	c := http3Cursor{wire: wire, end: len(wire)}
	fields := []http3Field{}
	control := false
	if mode == "uni" {
		kind, f, err := c.integer("Stream Type")
		if err != nil {
			return nil, nil, err
		}
		fields = append(fields, f)
		info["Stream Type"] = kind
		switch kind {
		case 0:
			control = true
			info["Stream Role"] = "control"
		case 2:
			table, decoded, err := http3QPACKEncoder(wire, c.at, maxCapacity)
			if err != nil {
				return nil, nil, err
			}
			fields = append(fields, decoded...)
			info["Stream Role"] = "QPACK encoder"
			info["QPACK Insert Count"] = table.insertCount
			info["QPACK Current Capacity"] = table.capacity
			info["QPACK Current Size"] = table.size
			info["Encoder Reference Lifecycle Validated"] = false
			return fields, info, nil
		case 3:
			decoded, err := http3QPACKDecoder(wire, c.at)
			if err != nil {
				return nil, nil, err
			}
			fields = append(fields, decoded...)
			info["Stream Role"] = "QPACK decoder"
			info["Acknowledgment State Validated"] = false
			return fields, info, nil
		case 1:
			return nil, nil, fmt.Errorf("http3: push stream response profile unsupported")
		default:
			fields = append(fields, http3Leaf("Unknown Stream Data", "raw", c.at*8, c.end*8))
			info["Stream Role"] = "unknown unidirectional type"
			info["Unknown Stream Semantics Decoded"] = false
			return fields, info, nil
		}
	} else if mode != "request" {
		return nil, nil, fmt.Errorf("http3: explicit request/uni stream mode required")
	} else {
		info["Stream Role"] = "client request"
	}
	var table *http3QPACKTable
	if !control {
		var err error
		table, err = http3QPACKSnapshot(encoder, maxCapacity)
		if err != nil {
			return nil, nil, err
		}
		info["QPACK Encoder Snapshot Supplied"] = len(encoder) != 0
		info["QPACK Insert Count"] = table.insertCount
	}
	settings := map[uint64]uint64{}
	frames, sections, dataBytes := 0, 0, uint64(0)
	var initial map[string]any
	for c.at < c.end {
		if frames == http3MaxItems {
			return nil, nil, fmt.Errorf("http3: frame count exceeds implementation profile")
		}
		start := c.at
		kind, kf, err := c.integer("Frame Type")
		if err != nil {
			return nil, nil, err
		}
		length, lf, err := c.integer("Frame Length")
		if err != nil {
			return nil, nil, err
		}
		if length > uint64(c.end-c.at) {
			return nil, nil, fmt.Errorf("http3: declared frame payload exceeds stream boundary")
		}
		payload, end := c.at, c.at+int(length)
		f := http3Group(fmt.Sprintf("Frame %d", frames), start*8, end*8, kf, lf)
		f.Info = map[string]any{"Frame Type": kind, "Payload Bytes": length}
		if kind == 2 || kind == 6 || kind == 8 || kind == 9 {
			return nil, nil, fmt.Errorf("http3: prohibited HTTP/2-reserved frame type")
		}
		if control {
			if frames == 0 && kind != 4 {
				return nil, nil, fmt.Errorf("http3: control stream must begin with SETTINGS")
			}
			switch kind {
			case 0, 1, 5:
				return nil, nil, fmt.Errorf("http3: request frame prohibited on control stream")
			case 4:
				if frames != 0 {
					return nil, nil, fmt.Errorf("http3: duplicate SETTINGS frame")
				}
				p := http3Cursor{wire: wire, at: payload, end: end}
				for p.at < p.end {
					if len(settings) == http3MaxItems {
						return nil, nil, fmt.Errorf("http3: settings count exceeds profile")
					}
					s := p.at
					id, idf, err := p.integer("Setting Identifier")
					if err != nil {
						return nil, nil, err
					}
					v, vf, err := p.integer("Setting Value")
					if err != nil {
						return nil, nil, err
					}
					if _, present := settings[id]; present {
						return nil, nil, fmt.Errorf("http3: duplicate setting identifier")
					}
					if id >= 2 && id <= 5 {
						return nil, nil, fmt.Errorf("http3: prohibited HTTP/2-reserved setting")
					}
					if (id == 8 || id == 0x33) && v > 1 {
						return nil, nil, fmt.Errorf("http3: boolean extension setting exceeds one")
					}
					settings[id] = v
					sf := http3Group(fmt.Sprintf("Setting %d", len(settings)-1), s*8, p.at*8, idf, vf)
					sf.Info = map[string]any{"Identifier": id, "Value": v}
					f.Children = append(f.Children, sf)
				}
			case 3, 7, 13:
				p := http3Cursor{wire: wire, at: payload, end: end}
				_, id, err := p.integer("Identifier")
				if err != nil {
					return nil, nil, err
				}
				if p.at != end {
					return nil, nil, fmt.Errorf("http3: extra bytes after control identifier")
				}
				f.Children = append(f.Children, id)
				f.Info["Cross Stream Identifier Validated"] = false
			case 0xf0700, 0xf0701:
				p := http3Cursor{wire: wire, at: payload, end: end}
				element, id, err := p.integer("Prioritized Element ID")
				if err != nil {
					return nil, nil, err
				}
				if kind == 0xf0700 && element%4 != 0 {
					return nil, nil, fmt.Errorf("http3: prioritized request ID must identify a client bidirectional stream")
				}
				for _, b := range wire[p.at:end] {
					if b < 0x20 || b > 0x7e {
						return nil, nil, fmt.Errorf("http3: priority field value requires printable ASCII")
					}
				}
				f.Children = append(f.Children, id, http3Leaf("Priority Field Value", "string", p.at*8, end*8))
				f.Info["Priority Structured Fields Decoded"] = false
				f.Info["Sender Role Validated"] = false
			default:
				f.Children = append(f.Children, http3Leaf("Unknown Frame Data", "raw", payload*8, end*8))
				f.Info["Unknown Frame Semantics Decoded"] = false
			}
		} else {
			switch kind {
			case 3, 4, 5, 7, 13, 0xf0700, 0xf0701:
				return nil, nil, fmt.Errorf("http3: control/server frame prohibited in client request")
			case 1:
				if sections == 2 {
					return nil, nil, fmt.Errorf("http3: too many request field sections")
				}
				block, headers, err := http3QPACKSection(wire, payload, end, table)
				if err != nil {
					return nil, nil, err
				}
				semantics, err := http3RequestFields(headers, sections != 0)
				if err != nil {
					return nil, nil, err
				}
				if sections == 0 {
					initial = semantics
					info["Request Headers"] = headers
				} else {
					info["Trailer Headers"] = headers
				}
				f.Children = append(f.Children, block)
				f.Info["Header Count"] = len(headers)
				sections++
			case 0:
				if sections != 1 {
					return nil, nil, fmt.Errorf("http3: DATA before initial headers or after trailers")
				}
				dataBytes += length
				f.Children = append(f.Children, http3Leaf("Data", "raw", payload*8, end*8))
			default:
				f.Children = append(f.Children, http3Leaf("Unknown Frame Data", "raw", payload*8, end*8))
				f.Info["Unknown Frame Semantics Decoded"] = false
			}
		}
		fields = append(fields, f)
		frames++
		c.at = end
	}
	if control {
		if frames == 0 {
			return nil, nil, fmt.Errorf("http3: missing initial SETTINGS")
		}
		info["Settings"] = settings
	} else {
		if sections == 0 {
			return nil, nil, fmt.Errorf("http3: missing request HEADERS")
		}
		if declared, ok := initial["Content Length"]; ok && declared.(uint64) != dataBytes {
			return nil, nil, fmt.Errorf("http3: Content-Length differs from DATA byte total")
		}
		for k, v := range initial {
			info[k] = v
		}
		info["Data Bytes"] = dataBytes
		info["Field Section Count"] = sections
		info["Complete Message Boundary Required"] = true
	}
	info["Frame Count"] = frames
	return fields, info, nil
}
