package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const memcachedFieldsMaxBytes = 1 << 20
const memcachedFieldsMaxStats = 4096

// Profiles decode one explicitly bounded message, never an inferred TCP stream.
// Numeric text annotations describe lexical representations, not metric units.
func decodeMemcachedFields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	return decodeMemcachedFieldsWithArena(wire, profile, nil)
}

func decodeMemcachedFieldsWithArena(wire []byte, profile string, arena *fieldArena) ([]tlsCertificateField, map[string]any, error) {
	fail := func(s string) ([]tlsCertificateField, map[string]any, error) {
		return nil, nil, fmt.Errorf("memcached-fields: %s", s)
	}
	if len(wire) == 0 || len(wire) > memcachedFieldsMaxBytes {
		return fail("input boundary or resource limit")
	}
	capacity := 8
	switch profile {
	case "stats-response":
		capacity = 11
	case "binary-get-request":
		capacity = 12
	}
	info := make(map[string]any, capacity)
	info["Protocol"], info["Layout Context"], info["Context Is Caller Supplied"] = "Memcached", profile, true
	info["TCP Reassembly Performed"], info["Session State Validated"], info["Command Executed"] = false, false, false
	info["Byte Ranges Are Message Relative"] = true
	var fs []tlsCertificateField
	leaf := func(name, typ string, s, e int) { fs = append(fs, tlsCertificateLeaf(name, typ, s, e)) }
	text := func(s, e int) (map[string]any, any) {
		value := any(string(wire[s:e]))
		return map[string]any{"Text": value, "Bytes": arena.cloneBytes(wire[s:e]), "Byte Range": [2]int{s, e}}, value
	}
	switch profile {
	case "stats-request":
		if !bytes.Equal(wire, []byte("stats\r\n")) {
			return fail("profile requires stats without arguments")
		}
		if arena != nil && arena.outputReserve > 0 {
			arena.structuredValue = map[string]any{"Command": "stats", "Line Terminator": arena.cloneBytes(wire[5:7])}
		} else {
			leaf("Command", "string", 0, 5)
			leaf("Line Terminator", "raw", 5, 7)
		}
		info["Command"] = "stats"
	case "stats-response":
		var stats []map[string]any
		direct := arena != nil && arena.outputReserve > 0
		var structuredStats []any
		// Reserve only a small, bounded number of observed lines. Even a
		// malformed 1 MiB input must not trigger a speculative huge allocation.
		reserve := min(64, max(0, bytes.Count(wire, []byte("\r\n"))-1))
		if reserve > 0 {
			if direct {
				structuredStats = make([]any, 0, reserve)
			} else {
				fs = arena.allocate(reserve)[:0]
			}
			stats = make([]map[string]any, 0, reserve)
		}
		at := 0
		for {
			if bytes.Equal(wire[at:], []byte("END\r\n")) {
				end := at
				if direct {
					if structuredStats == nil {
						structuredStats = []any{}
					}
					arena.structuredValue = map[string]any{"Statistics": structuredStats, "Response Terminator": "END", "Line Terminator": arena.cloneBytes(wire[at+3 : at+5])}
				} else {
					fs = []tlsCertificateField{{Name: "Statistics", Start: 0, End: end, List: true, Children: fs}}
					leaf("Response Terminator", "string", at, at+3)
					leaf("Line Terminator", "raw", at+3, at+5)
				}
				break
			}
			if len(stats) >= memcachedFieldsMaxStats {
				return fail("statistic count resource limit")
			}
			n := bytes.Index(wire[at:], []byte("\r\n"))
			if n < 0 {
				return fail("missing CRLF or END terminator")
			}
			line := wire[at : at+n]
			if !bytes.HasPrefix(line, []byte("STAT ")) {
				return fail("expected STAT line or final END")
			}
			sep := bytes.IndexByte(line[5:], ' ')
			if sep <= 0 || 5+sep+1 >= len(line) {
				return fail("statistic requires name and value")
			}
			for _, x := range line {
				if x < 32 || x > 126 {
					return fail("profile requires printable ASCII statistic line")
				}
			}
			keyEnd, valueStart, end := at+5+sep, at+6+sep, at+n
			name, nameText := text(at+5, keyEnd)
			value, valueText := text(valueStart, end)
			if direct {
				// The same validated ranges feed both exports. Construct this
				// repeated fixed shape without an intermediate descriptor tree.
				structuredStats = append(structuredStats, map[string]any{
					"Statistic Prefix": "STAT", "Name Separator": arena.cloneBytes(wire[at+4 : at+5]),
					"Statistic Name": nameText, "Value Separator": arena.cloneBytes(wire[keyEnd:valueStart]),
					"Statistic Value": valueText, "Line Terminator": arena.cloneBytes(wire[end : end+2]),
				})
			} else {
				children := arena.allocate(6)
				copy(children, []tlsCertificateField{
					tlsCertificateLeaf("Statistic Prefix", "string", at, at+4),
					tlsCertificateLeaf("Name Separator", "raw", at+4, at+5),
					tlsCertificateLeaf("Statistic Name", "string", at+5, keyEnd),
					tlsCertificateLeaf("Value Separator", "raw", keyEnd, valueStart),
					tlsCertificateLeaf("Statistic Value", "string", valueStart, end),
					tlsCertificateLeaf("Line Terminator", "raw", end, end+2),
				})
				fs = append(fs, tlsCertificateField{Name: "Statistic", Start: at, End: end + 2, Children: children})
			}
			// Do not coerce unknown names or versions into metrics. This is optional,
			// exact lexical evidence; overflow and non-numeric text remain unchanged.
			digits := true
			for _, c := range wire[valueStart:end] {
				if c < '0' || c > '9' {
					digits = false
					break
				}
			}
			if digits {
				if v, e := strconv.ParseUint(valueText.(string), 10, 64); e == nil {
					value["Unsigned Decimal"] = v
				}
			}
			stats = append(stats, map[string]any{"Name": name, "Value": value, "Byte Range": [2]int{at, end + 2}})
			at = end + 2
		}
		info["Statistics"], info["Statistic Count"], info["Response Complete"] = stats, uint64(len(stats)), true
		info["Metric Units Inferred"] = false
	case "binary-get-request":
		if len(wire) < 24 {
			return fail("incomplete binary header")
		}
		keyLen := int(binary.BigEndian.Uint16(wire[2:4]))
		bodyLen := uint64(binary.BigEndian.Uint32(wire[8:12]))
		if wire[0] != 0x80 || wire[1] != 0 {
			return fail("profile requires binary GET request")
		}
		if wire[4] != 0 || wire[5] != 0 || keyLen == 0 || keyLen > 250 || bodyLen != uint64(keyLen) || bodyLen != uint64(len(wire)-24) {
			return fail("GET layout requires raw datatype, 1..250 byte key, no extras/value, exact body boundary")
		}
		var values map[string]any
		if arena != nil && arena.outputReserve > 0 {
			values = make(map[string]any, 10)
		}
		for _, f := range []struct {
			name       string
			start, end int
		}{
			{"Magic", 0, 1}, {"Opcode", 1, 2}, {"Key Length", 2, 4}, {"Extras Length", 4, 5}, {"Data Type", 5, 6}, {"VBucket ID", 6, 8}, {"Total Body Length", 8, 12}, {"Opaque", 12, 16}, {"CAS", 16, 24},
		} {
			if values != nil {
				var value uint64
				for _, x := range wire[f.start:f.end] {
					value = value<<8 | uint64(x)
				}
				values[f.name] = value
			} else {
				leaf(f.name, "uint64", f.start, f.end)
			}
		}
		if values != nil {
			values["Key"] = arena.cloneBytes(wire[24:])
			arena.structuredValue = values
		} else {
			leaf("Key", "raw", 24, len(wire))
		}
		info["Command"], info["Response"], info["Key"] = "GET", false, map[string]any{"Bytes": arena.cloneBytes(wire[24:]), "Byte Range": [2]int{24, len(wire)}}
		info["VBucket Semantics Validated"], info["CAS Semantics Validated"] = false, false
	default:
		return fail("unsupported explicit profile")
	}
	return fs, info, nil
}

func parseMemcachedFields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	arena := acquireFieldArena()
	defer arena.release()
	return parseCertificateFieldTree(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeMemcachedFieldsWithArena(w, profile, arena)
	}, "memcached-fields")
}
