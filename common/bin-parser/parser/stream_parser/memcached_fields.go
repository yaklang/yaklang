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
	fail := func(s string) ([]tlsCertificateField, map[string]any, error) {
		return nil, nil, fmt.Errorf("memcached-fields: %s", s)
	}
	if len(wire) == 0 || len(wire) > memcachedFieldsMaxBytes {
		return fail("input boundary or resource limit")
	}
	info := map[string]any{"Protocol": "Memcached", "Layout Context": profile, "Context Is Caller Supplied": true, "TCP Reassembly Performed": false, "Session State Validated": false, "Command Executed": false, "Byte Ranges Are Message Relative": true}
	var fs []tlsCertificateField
	leaf := func(name, typ string, s, e int) { fs = append(fs, tlsCertificateLeaf(name, typ, s, e)) }
	text := func(s, e int) map[string]any {
		return map[string]any{"Text": string(wire[s:e]), "Bytes": bytes.Clone(wire[s:e]), "Byte Range": [2]int{s, e}}
	}
	switch profile {
	case "stats-request":
		if !bytes.Equal(wire, []byte("stats\r\n")) {
			return fail("profile requires stats without arguments")
		}
		leaf("Command", "string", 0, 5)
		leaf("Line Terminator", "raw", 5, 7)
		info["Command"] = "stats"
	case "stats-response":
		var stats []map[string]any
		// Reserve only a small, bounded number of observed lines. Even a
		// malformed 1 MiB input must not trigger a speculative huge allocation.
		reserve := min(64, max(0, bytes.Count(wire, []byte("\r\n"))-1))
		if reserve > 0 {
			fs = make([]tlsCertificateField, 0, reserve)
			stats = make([]map[string]any, 0, reserve)
		}
		at := 0
		for {
			if bytes.Equal(wire[at:], []byte("END\r\n")) {
				end := at
				fs = []tlsCertificateField{{Name: "Statistics", Start: 0, End: end, List: true, Children: fs}}
				leaf("Response Terminator", "string", at, at+3)
				leaf("Line Terminator", "raw", at+3, at+5)
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
			children := []tlsCertificateField{
				tlsCertificateLeaf("Statistic Prefix", "string", at, at+4),
				tlsCertificateLeaf("Name Separator", "raw", at+4, at+5),
				tlsCertificateLeaf("Statistic Name", "string", at+5, keyEnd),
				tlsCertificateLeaf("Value Separator", "raw", keyEnd, valueStart),
				tlsCertificateLeaf("Statistic Value", "string", valueStart, end),
				tlsCertificateLeaf("Line Terminator", "raw", end, end+2),
			}
			fs = append(fs, tlsCertificateField{Name: "Statistic", Start: at, End: end + 2, Children: children})
			value := text(valueStart, end)
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
				if v, e := strconv.ParseUint(value["Text"].(string), 10, 64); e == nil {
					value["Unsigned Decimal"] = v
				}
			}
			stats = append(stats, map[string]any{"Name": text(at+5, keyEnd), "Value": value, "Byte Range": [2]int{at, end + 2}})
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
		for _, f := range []struct {
			name       string
			start, end int
		}{
			{"Magic", 0, 1}, {"Opcode", 1, 2}, {"Key Length", 2, 4}, {"Extras Length", 4, 5}, {"Data Type", 5, 6}, {"VBucket ID", 6, 8}, {"Total Body Length", 8, 12}, {"Opaque", 12, 16}, {"CAS", 16, 24},
		} {
			leaf(f.name, "uint64", f.start, f.end)
		}
		leaf("Key", "raw", 24, len(wire))
		info["Command"], info["Response"], info["Key"] = "GET", false, map[string]any{"Bytes": bytes.Clone(wire[24:]), "Byte Range": [2]int{24, len(wire)}}
		info["VBucket Semantics Validated"], info["CAS Semantics Validated"] = false, false
	default:
		return fail("unsupported explicit profile")
	}
	return fs, info, nil
}

func parseMemcachedFields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	return parseCertificateFieldTree(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeMemcachedFields(w, profile)
	}, "memcached-fields")
}
