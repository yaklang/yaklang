package stream_parser

import (
	"fmt"
	"hash/crc32"
	"net"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const cassandraFieldsMaxBytes = 1 << 20
const cassandraFieldsMaxItems = 4096
const cassandraFieldsMaxLeaves = 65536

// CQL version, direction and pre-READY v5 framing are caller-selected contexts.
// Internode Initiate is a different protocol, never a CQL opcode or port guess.
type cassandraFieldsReader struct {
	wire                   []byte
	at, end, items, leaves int
	err                    error
	fields                 []tlsCertificateField
	arena                  *fieldArena
	direct                 bool
	values                 []structuredFieldValue
}

func (r *cassandraFieldsReader) fail(s string) {
	if r.err == nil {
		r.err = fmt.Errorf("cassandra-fields: %s at byte %d", s, r.at)
	}
}
func (r *cassandraFieldsReader) take(name, typ string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > r.end-r.at {
		r.fail("incomplete " + name)
		return nil
	}
	if r.leaves >= cassandraFieldsMaxLeaves {
		r.fail("field resource limit")
		return nil
	}
	r.leaves++
	s := r.at
	r.at += n
	if r.direct {
		var value any
		if typ == "raw" {
			value = r.arena.cloneBytes(r.wire[s:r.at])
		}
		r.values = r.arena.appendValue(r.values, name, value)
	} else {
		if len(r.fields) == cap(r.fields) {
			fields := r.arena.allocate(max(8, 2*cap(r.fields)))[:len(r.fields)]
			copy(fields, r.fields)
			r.fields = fields
		}
		r.fields = append(r.fields, tlsCertificateLeaf(name, typ, s, r.at))
	}
	return r.wire[s:r.at]
}
func (r *cassandraFieldsReader) uint(name string, n int) uint64 {
	var v uint64
	for _, x := range r.take(name, "uint64", n) {
		v = v<<8 | uint64(x)
	}
	if r.direct && r.err == nil {
		r.values[len(r.values)-1].value = v
	}
	return v
}
func (r *cassandraFieldsReader) item() bool {
	if r.err != nil {
		return false
	}
	r.items++
	if r.items > cassandraFieldsMaxItems {
		r.fail("item resource limit")
		return false
	}
	return true
}
func (r *cassandraFieldsReader) group(name string, start, index int, list bool) {
	if r.direct {
		if r.err != nil {
			return
		}
		value := projectCassandraValues(r.values[index:], list)
		clear(r.values[index:])
		r.values = r.arena.appendValue(r.values[:index], name, value)
		return
	}
	children := r.arena.allocate(len(r.fields) - index)
	copy(children, r.fields[index:])
	r.fields = append(r.fields[:index], tlsCertificateField{Name: name, Start: start, End: r.at, List: list, Children: children})
}

// Direct mode collapses validated groups as they finish. The exact same reader
// enforces all wire boundaries and resource limits; no descriptor tree needs
// to be copied and traversed afterwards. These maps/slices are never pooled.
func projectCassandraValues(fields []structuredFieldValue, list bool) any {
	if list {
		values := make([]any, len(fields))
		for i := range fields {
			values[i] = fields[i].value
		}
		return values
	}
	values := make(map[string]any, len(fields))
	for i := range fields {
		values[fields[i].name] = fields[i].value
	}
	return values
}
func (r *cassandraFieldsReader) text(name, lengthName string) map[string]any {
	n := r.uint(lengthName, 2)
	s := r.at
	b := r.take(name, "string", int(n))
	if !utf8.Valid(b) {
		r.fail("invalid UTF-8 in " + name)
	}
	value := any(string(b))
	if r.direct && r.err == nil {
		r.values[len(r.values)-1].value = value
	}
	return map[string]any{"Text": value, "Bytes": r.arena.cloneBytes(b), "Byte Range": [2]int{s, r.at}}
}
func (r *cassandraFieldsReader) fieldCount() int {
	if r.direct {
		return len(r.values)
	}
	return len(r.fields)
}

func (r *cassandraFieldsReader) options(info map[string]any, multi bool) {
	count := r.uint("Option Count", 2)
	start, index := r.at, r.fieldCount()
	var options []map[string]any
	// A valid option needs two string-length fields even when both are empty.
	// Bound speculative capacity by the wire as well as the existing item limit.
	if capacity := min(int(count), cassandraFieldsMaxItems, (r.end-r.at)/4); capacity > 0 {
		options = make([]map[string]any, 0, capacity)
	}
	hasVersion := false
	for i := uint64(0); i < count && r.item(); i++ {
		s, ix := r.at, r.fieldCount()
		key := r.text("Option Key", "Option Key Length")
		if key["Text"] == "CQL_VERSION" {
			hasVersion = true
		}
		m := map[string]any{"Key": key}
		if multi {
			n := r.uint("Option Value Count", 2)
			vs, vi := r.at, r.fieldCount()
			var values []map[string]any
			if capacity := min(int(n), cassandraFieldsMaxItems-r.items, (r.end-r.at)/2); capacity > 0 {
				values = make([]map[string]any, 0, capacity)
			}
			for j := uint64(0); j < n && r.item(); j++ {
				valueStart, valueIndex := r.at, r.fieldCount()
				values = append(values, r.text("Option Value", "Option Value Length"))
				r.group("Value", valueStart, valueIndex, false)
			}
			r.group("Values", vs, vi, true)
			m["Values"] = values
		} else {
			m["Value"] = r.text("Option Value", "Option Value Length")
		}
		m["Byte Range"] = [2]int{s, r.at}
		options = append(options, m)
		r.group("Option", s, ix, false)
	}
	r.group("Options", start, index, true)
	info["Options"], info["CQL Version Key Present"] = options, hasVersion
}

func (r *cassandraFieldsReader) initiate(info map[string]any) {
	if r.uint("Protocol Magic", 4) != 0xca552dfa {
		r.fail("invalid internode magic")
	}
	flags := r.uint("Connection Flags", 4)
	requested, minimum, maximum := (flags>>8)&255, (flags>>16)&255, (flags>>24)&255
	if requested < 12 || maximum < 12 {
		r.fail("profile requires modern Initiate")
	}
	framing := ((flags >> 2) & 1) | ((flags >> 3) & 2)
	if framing == 3 {
		r.fail("unsupported internode framing id")
		return
	}
	// Preserve reserved bits; they are not an invitation to infer new fields.
	info["Requested Messaging Version"], info["Minimum Messaging Version"], info["Maximum Messaging Version"] = requested, minimum, maximum
	info["Framing ID"], info["Streaming Mode"], info["Category ID"], info["Reserved Connection Bits"] = framing, flags&8 != 0, flags&3, (flags>>5)&7
	info["Framing"] = []string{"unprotected", "lz4", "crc"}[framing]
	if flags&8 != 0 {
		info["Connection Type"] = "streaming"
	} else {
		info["Connection Type"] = []string{"legacy messages", "urgent messages", "small messages", "large messages"}[flags&3]
	}
	length := r.uint("Endpoint Length", 1)
	if length != 6 && length != 18 {
		r.fail("profile requires endpoint with explicit port")
		return
	}
	s := r.at
	address := r.take("Endpoint Address", "raw", int(length)-2)
	info["Endpoint Address"] = map[string]any{"Bytes": r.arena.cloneBytes(address), "Byte Range": [2]int{s, r.at}, "Text": net.IP(address).String()}
	info["Endpoint Port"] = r.uint("Endpoint Port", 2)
	crcStart := r.at
	want := r.uint("Message CRC32", 4)
	if r.err != nil {
		return
	}
	// Apache's CRC starts with FA 2D 55 CA before the actual wire message.
	// This four-byte prefix is not an additional on-wire field.
	crc := crc32.ChecksumIEEE([]byte{0xfa, 0x2d, 0x55, 0xca})
	crc = crc32.Update(crc, crc32.IEEETable, r.wire[:crcStart])
	if want != uint64(crc) {
		r.fail("internode CRC32 mismatch")
		return
	}
	info["CRC32 Verified"], info["CRC32 Covered Byte Range"], info["CRC32 Initial Bytes"] = true, [2]int{0, crcStart}, []byte{0xfa, 0x2d, 0x55, 0xca}
	info["CRC32 Field Byte Range"] = [2]int{crcStart, r.at}
}

func decodeCassandraFields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	return decodeCassandraFieldsWithArena(wire, profile, nil)
}

func decodeCassandraFieldsWithArena(wire []byte, profile string, arena *fieldArena) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) < 9 || len(wire) > cassandraFieldsMaxBytes {
		return nil, nil, fmt.Errorf("cassandra-fields: input boundary or resource limit")
	}
	r := &cassandraFieldsReader{wire: wire, end: len(wire), arena: arena, direct: arena != nil && arena.outputReserve > 0}
	if r.direct {
		r.values = arena.values[:0]
	}
	capacity := 15
	switch profile {
	case "options4", "options5-initial":
		capacity = 13
	case "internode-initiate-modern":
		capacity = 22
	}
	info := make(map[string]any, capacity)
	info["Layout Context"], info["Context Is Caller Supplied"], info["Session State Validated"] = profile, true, false
	info["TCP Reassembly Performed"], info["Query Executed"], info["Byte Ranges Are Message Relative"] = false, false, true
	if profile == "internode-initiate-modern" {
		info["Protocol"] = "Cassandra Internode"
		r.initiate(info)
	} else {
		var version, opcode uint64
		switch profile {
		case "options4":
			version, opcode = 4, 5
		case "startup4":
			version, opcode = 4, 1
		case "supported4":
			version, opcode = 0x84, 6
		case "options5-initial":
			version, opcode = 5, 5
		case "startup5-initial":
			version, opcode = 5, 1
		case "supported5-initial":
			version, opcode = 0x85, 6
		default:
			return nil, nil, fmt.Errorf("cassandra-fields: unsupported explicit profile %q", profile)
		}
		if r.uint("Version", 1) != version {
			r.fail("version/direction differs from explicit profile")
		}
		if r.uint("Flags", 1) != 0 {
			r.fail("flagged or compressed envelopes require another profile")
		}
		stream := r.uint("Stream", 2)
		if stream > 32767 {
			r.fail("negative stream id for initial exchange")
		}
		if r.uint("Opcode", 1) != opcode {
			r.fail("opcode differs from explicit profile")
		}
		length := r.uint("Body Length", 4)
		if length != uint64(len(wire)-9) {
			r.fail("body length differs from supplied boundary")
		}
		info["Protocol"], info["Version"], info["Response"], info["Stream ID"] = "CQL", version&127, version&128 != 0, stream
		info["Compression Applied"], info["Negotiation Validated"] = false, false
		if version&127 == 5 {
			info["Framing Context"] = "caller-supplied initial exchange, before READY or AUTHENTICATE"
		} else {
			info["Framing Context"] = "v4 envelope"
		}
		if r.err == nil {
			switch opcode {
			case 1:
				r.options(info, false)
			case 6:
				r.options(info, true)
			}
		}
	}
	if r.at != r.end {
		r.fail("unconsumed message bytes")
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	if r.direct {
		arena.structuredValue = projectCassandraValues(r.values, false)
		return nil, info, nil
	}
	return r.fields, info, nil
}

func parseCassandraFields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	arena := acquireFieldArena()
	defer arena.release()
	return parseCertificateFieldTree(node, process, func(wire []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeCassandraFieldsWithArena(wire, profile, arena)
	}, "cassandra-fields")
}
