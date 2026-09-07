package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const tdsFieldsMaxBytes = 1 << 20
const tdsFieldsMaxItems = 4096
const tdsFieldsMaxLeaves = 65536

// Explicit MS-TDS 7.1 / 7.2 layout profiles. Version and cleartext context
// come from the caller, never a port or a payload heuristic. A complete message
// includes every TDS packet header; TCP ordering is the caller's responsibility.
type tdsFieldsPacket struct{ start, end, logicalStart, logicalEnd int }
type tdsFieldsReader struct {
	wire           []byte // logical message body, without packet headers
	at, end        int
	err            error
	fields         []tlsCertificateField
	packets        []tdsFieldsPacket
	version72      bool
	leaves, values int
}

func (r *tdsFieldsReader) fail(s string) {
	if r.err == nil {
		r.err = fmt.Errorf("tds-fields: %s at logical byte %d", s, r.at)
	}
}
func (r *tdsFieldsReader) take(name, typ string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > r.end-r.at {
		r.fail("incomplete " + name)
		return nil
	}
	if r.leaves == tdsFieldsMaxLeaves {
		r.fail("field resource limit")
		return nil
	}
	r.leaves++
	s := r.at
	r.at += n
	f := tlsCertificateLeaf(name, typ, s, r.at)
	f.Endian = "little"
	r.fields = append(r.fields, f)
	return r.wire[s:r.at]
}
func (r *tdsFieldsReader) uint(name string, n int) uint64 {
	b := r.take(name, "uint64", n)
	var v uint64
	for i := len(b) - 1; i >= 0; i-- {
		v = v<<8 | uint64(b[i])
	}
	return v
}
func (r *tdsFieldsReader) ranges(start, end int) [][2]int {
	var spans [][2]int
	index := sort.Search(len(r.packets), func(i int) bool { return r.packets[i].logicalEnd > start })
	if start == end && start == len(r.wire) {
		index = len(r.packets) - 1
	}
	for ; index < len(r.packets); index++ {
		p := r.packets[index]
		if p.logicalStart > end || start != end && p.logicalStart == end {
			break
		}
		a, b := max(start, p.logicalStart), min(end, p.logicalEnd)
		// A zero-width value belongs to the following payload, or the end of
		// the final one, never both sides of a packet header.
		if start == end {
			if start >= p.logicalStart && (start < p.logicalEnd || start == len(r.wire) && p.logicalEnd == start) {
				at := p.start + 8 + start - p.logicalStart
				return [][2]int{{at, at}}
			}
		} else if a < b {
			spans = append(spans, [2]int{p.start + 8 + a - p.logicalStart, p.start + 8 + b - p.logicalStart})
		}
	}
	return spans
}
func (r *tdsFieldsReader) span(start, end int) map[string]any {
	return map[string]any{"Logical Byte Range": [2]int{start, end}, "Wire Byte Ranges": r.ranges(start, end)}
}

// Keep exact code units when a database string contains unpaired surrogates.
// Such values have no synthesized replacement-character Text annotation.
func tdsFieldsUnicode(m map[string]any, b []byte) {
	units := make([]uint16, len(b)/2)
	valid := len(b)%2 == 0
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xd800 && u <= 0xdbff {
			if i+1 == len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				valid = false
			} else {
				i++
			}
		} else if u >= 0xdc00 && u <= 0xdfff {
			valid = false
		}
	}
	m["Unicode Code Units"], m["Unicode Text Valid"] = units, valid
	if valid {
		m["Text"] = string(utf16.Decode(units))
	}
}
func (r *tdsFieldsReader) unicode(name string, n int) map[string]any {
	s := r.at
	b := r.take(name, "raw", n)
	if n%2 != 0 {
		r.fail("odd Unicode byte length")
	}
	m := r.span(s, r.at)
	m["Bytes"] = bytes.Clone(b)
	tdsFieldsUnicode(m, b)
	return m
}
func (r *tdsFieldsReader) name(name string, width int) map[string]any {
	n := r.uint(name+" Character Count", width)
	return r.unicode(name, int(n)*2)
}
func (r *tdsFieldsReader) allHeaders() []map[string]any {
	start := r.at
	size := r.uint("All Headers Length", 4)
	if size < 4 || size > uint64(r.end-start) {
		r.fail("invalid ALL_HEADERS length")
		return nil
	}
	outer := r.end
	r.end = start + int(size)
	defer func() { r.end = outer }()
	var headers []map[string]any
	seen := map[uint64]bool{}
	for r.err == nil && r.at < r.end {
		s := r.at
		n := r.uint("Header Length", 4)
		if n < 6 || n > uint64(r.end-s) {
			r.fail("invalid header length")
			break
		}
		kind := r.uint("Header Type", 2)
		if seen[kind] {
			r.fail("duplicate header type")
			break
		}
		seen[kind] = true
		m := r.span(s, s+int(n))
		m["Type"] = kind
		switch kind {
		case 2:
			if n != 18 {
				r.fail("transaction descriptor header requires 18 bytes")
				break
			}
			m["Transaction Descriptor"] = r.uint("Transaction Descriptor", 8)
			m["Outstanding Request Count"] = r.uint("Outstanding Request Count", 4)
		default:
			// Optional header layouts require separately selected profiles.
			r.fail("unsupported ALL_HEADERS type")
		}
		headers = append(headers, m)
	}
	if !seen[2] {
		r.fail("transaction descriptor header required")
	}
	return headers
}

func decodeTDSFields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	kinds := map[string]byte{"batch71": 1, "batch72": 1, "rpc71": 3, "rpc72": 3, "response71": 4, "response72": 4}
	kind, ok := kinds[profile]
	if !ok {
		return nil, nil, fmt.Errorf("tds-fields: unknown explicit profile")
	}
	if len(wire) < 8 || len(wire) > tdsFieldsMaxBytes {
		return nil, nil, fmt.Errorf("tds-fields: complete 8..1048576 byte message required")
	}
	r := &tdsFieldsReader{version72: strings.HasSuffix(profile, "72")}
	framingError := func(s string) ([]tlsCertificateField, map[string]any, error) {
		return nil, nil, fmt.Errorf("tds-fields: %s", s)
	}
	var previousID byte
	var spid uint16
	for at := 0; at < len(wire); {
		if len(r.packets) == tdsFieldsMaxItems {
			return framingError("packet resource limit")
		}
		if len(wire)-at < 8 {
			return framingError("incomplete packet header")
		}
		h := wire[at : at+8]
		n := int(binary.BigEndian.Uint16(h[2:4]))
		if h[0] != kind || n < 8 || n > len(wire)-at {
			return framingError("packet type or length differs from explicit boundary")
		}
		if len(r.packets) != 0 && (h[6] != previousID+1 || binary.BigEndian.Uint16(h[4:6]) != spid) {
			return framingError("packet sequence or SPID changed")
		}
		if (h[1]&1 != 0) != (at+n == len(wire)) {
			return framingError("exact final EOM boundary required")
		}
		previousID, spid = h[6], binary.BigEndian.Uint16(h[4:6])
		p := tdsFieldsPacket{at, at + n, len(r.wire), len(r.wire) + n - 8}
		r.packets = append(r.packets, p)
		r.wire = append(r.wire, wire[at+8:at+n]...)
		at += n
	}
	r.end = len(r.wire)
	info := map[string]any{"Layout Context": profile, "Version Is Caller Supplied": true, "Session State Validated": false, "Query Executed": false, "TCP Reassembly Performed": false, "TDS Packet Count": len(r.packets), "Logical Body Bytes": len(r.wire), "Wire Byte Ranges Are Message Relative": true}
	if r.version72 && kind != 4 {
		info["All Headers"] = r.allHeaders()
	}
	switch kind {
	case 1:
		info["SQL Text"] = r.unicode("SQL Text", r.end-r.at)
	case 3:
		info["RPC Requests"] = r.rpc()
	case 4:
		info["Response Tokens"] = r.response()
	}
	if r.err == nil && r.at != r.end {
		r.fail("unconsumed logical body")
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	// Project flat logical leaves to each physical payload. A leaf split by a
	// TDS header is raw fragments; its decoded value and all spans live in info.
	// This never publishes a numeric field spanning a packet header.
	list := tlsCertificateField{Name: "TDS Packets", Start: 0, End: len(wire), List: true}
	fragments := make([][]tlsCertificateField, len(r.packets))
	for _, f := range r.fields {
		for _, span := range r.ranges(f.Start, f.End) {
			i := sort.Search(len(r.packets), func(i int) bool { return r.packets[i].end > span[0] })
			if i == len(r.packets) {
				i--
			}
			child := f
			child.Start, child.End = span[0], span[1]
			if child.End-child.Start != f.End-f.Start {
				child.Name += " Fragment"
				child.Type = "raw"
			}
			fragments[i] = append(fragments[i], child)
		}
	}
	projectedLeaves := 0
	for i, p := range r.packets {
		fs := []tlsCertificateField{
			tlsCertificateLeaf("Type", "uint8", p.start, p.start+1), tlsCertificateLeaf("Status", "uint8", p.start+1, p.start+2),
			tlsCertificateLeaf("Length", "uint16", p.start+2, p.start+4), tlsCertificateLeaf("SPID", "uint16", p.start+4, p.start+6),
			tlsCertificateLeaf("PacketID", "uint8", p.start+6, p.start+7), tlsCertificateLeaf("Window", "uint8", p.start+7, p.start+8),
		}
		fs = append(fs, fragments[i]...)
		projectedLeaves += len(fs)
		if projectedLeaves > tdsFieldsMaxLeaves {
			return framingError("projected field resource limit")
		}
		list.Children = append(list.Children, tlsCertificateField{Name: "TDS Packet", Start: p.start, End: p.end, Children: fs})
	}
	info["Value Count"] = r.values
	return []tlsCertificateField{list}, info, nil
}

func parseTDSFields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	return parseCertificateFieldTree(node, process, func(wire []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeTDSFields(wire, profile)
	}, "tds-fields")
}
