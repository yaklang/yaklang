package stream_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
)

var (
	ErrMLDTruncated = errors.New("MLD bytes unavailable")
	ErrMLDMalformed = errors.New("MLD wire layout invalid")
)

// MLDWireContext describes the enclosing IP extent, not listener state. A
// negative DeclaredBytes means that no valid declared extent is available.
// Without a context, the caller supplies one complete, bounded ICMPv6 PDU.
type MLDWireContext struct{ DeclaredBytes int }

type MLDWireField struct {
	Name           string
	Value          any
	Offset, Length int
	// Path names leaves in Fields; array indices are decimal components.
	Path []string
}

type MLDWireResult struct {
	Fields  map[string]any
	Values  []MLDWireField
	Version uint8
	Summary string
}

type MLDWireError struct {
	Kind, Code, Reason  string
	Available, Required int
}

func (e *MLDWireError) Error() string {
	if e.Required > e.Available {
		return fmt.Sprintf("MLD %s (%d/%d bytes)", e.Reason, e.Available, e.Required)
	}
	return "MLD " + e.Reason
}

func (e *MLDWireError) Unwrap() error {
	if e.Kind == "incomplete" {
		return ErrMLDTruncated
	}
	if e.Kind == "limited" {
		if e.Code == "MLDLengthUnconfirmed" {
			return ErrObservedWireContext
		}
		return ErrObservedWireBudget
	}
	return ErrMLDMalformed
}

// DecodeMLDWire decodes RFC2710/RFC9777 fields and bounded record vectors. It
// never establishes membership, verifies a checksum or validates the IP/Router
// Alert envelope. Incomplete inputs retain only whole available fields.
func DecodeMLDWire(w []byte, maxBytes, maxElements int, context ...MLDWireContext) (out MLDWireResult, err error) {
	defer func() { attachMLDWireRanges(&out) }()
	if maxBytes <= 0 || len(w) > maxBytes || maxElements <= 0 {
		return out, ErrObservedWireBudget
	}
	declared := len(w)
	if len(context) > 1 {
		return out, fmt.Errorf("MLD: at most one enclosing extent is allowed")
	}
	if len(context) == 1 {
		declared = context[0].DeclaredBytes
	}
	if len(w) < 4 {
		return out, &MLDWireError{"incomplete", "MLDTruncated", "ICMPv6 header unavailable", len(w), 4}
	}
	typ := w[0]
	if typ != 130 && typ != 131 && typ != 132 && typ != 143 {
		return out, fmt.Errorf("MLD: ICMPv6 type %d is outside this profile", typ)
	}
	if declared >= 0 && declared < len(w) {
		return out, &MLDWireError{"malformed", "MLDEnvelopeLength", "bytes exceed the declared enclosing extent", len(w), declared}
	}
	m := map[string]any{"Observation Scope": "captured wire fields; checksum, IP envelope and listener state unverified"}
	out.Fields = map[string]any{"Type": typ, "Code": w[1], "Checksum": binary.BigEndian.Uint16(w[2:4]), "MLD": m}
	add := func(name string, value any, at, size int, path ...string) {
		out.Values = append(out.Values, MLDWireField{name, value, at, size, append([]string(nil), path...)})
	}
	add("", typ, 0, 1, "Type")
	add("", w[1], 1, 1, "Code")
	add("icmpv6.checksum", binary.BigEndian.Uint16(w[2:4]), 2, 2, "Checksum")
	fail := func(kind, code, reason string, required int) (MLDWireResult, error) {
		m["Status"], m["Error"] = kind, reason
		m["Available ICMPv6 Bytes"] = len(w)
		if required > len(w) {
			m["Minimum Bytes Needed for Next Field"] = required
		}
		out.Summary += "; " + reason
		return out, &MLDWireError{kind, code, reason, len(w), required}
	}
	need := func(end int) bool { return end > len(w) }
	used := 0
	charge := func() bool {
		if used >= maxElements {
			return false
		}
		used++
		return true
	}
	address := func(at int) string { return netip.AddrFrom16([16]byte(w[at : at+16])).String() }
	finish := func(at int) (MLDWireResult, error) {
		if at < len(w) {
			m["Additional Data (uninterpreted)"] = bytes.Clone(w[at:])
			add("", hex.EncodeToString(w[at:]), at, len(w)-at, "MLD", "Additional Data (uninterpreted)")
		}
		if declared > len(w) {
			return fail("incomplete", "MLDEnvelopeTruncated", "declared enclosing message tail unavailable", declared)
		}
		if declared < 0 {
			return fail("limited", "MLDLengthUnconfirmed", "declared enclosing message length unavailable", 0)
		}
		return out, nil
	}
	if typ == 143 {
		out.Version = 2
		out.Summary = "MLDv2 Multicast Listener Report"
		m["Version"], m["Message"] = uint8(2), "Multicast Listener Report"
		if len(w) >= 6 {
			m["Report Flags (raw)"] = binary.BigEndian.Uint16(w[4:6])
			add("", m["Report Flags (raw)"], 4, 2, "MLD", "Report Flags (raw)")
		}
		if need(8) {
			return fail("incomplete", "MLDTruncated", "report fixed header unavailable", 8)
		}
		n := int(binary.BigEndian.Uint16(w[6:8]))
		m["Address Record Count"] = uint16(n)
		add("icmpv6.mldr.nb_mcast_records", uint16(n), 6, 2, "MLD", "Address Record Count")
		records := []map[string]any{}
		m["Address Records"] = records
		at := 8
		for i := 0; i < n; i++ {
			if need(at + 1) {
				return fail("incomplete", "MLDTruncated", "address record fixed header unavailable", at+20)
			}
			if !charge() {
				return fail("limited", "MLDCollectionBudget", "address record collection budget exceeded", 0)
			}
			rec := map[string]any{"Index": i + 1, "Record Type": w[at]}
			path := []string{"MLD", "Address Records", fmt.Sprint(i)}
			put := func(key, alias string, value any, offset, length int) {
				rec[key] = value
				add(alias, value, offset, length, append(append([]string(nil), path...), key)...)
			}
			put("Record Type", "icmpv6.mldr.mar.record_type", w[at], at, 1)
			names := [...]string{"", "MODE_IS_INCLUDE", "MODE_IS_EXCLUDE", "CHANGE_TO_INCLUDE_MODE", "CHANGE_TO_EXCLUDE_MODE", "ALLOW_NEW_SOURCES", "BLOCK_OLD_SOURCES"}
			if int(w[at]) < len(names) && w[at] != 0 {
				rec["Record Type Name"] = names[w[at]]
			} else {
				rec["Record Type Name"] = "unrecognized; record bytes observed only"
			}
			add("", rec["Record Type Name"], at, 1, append(append([]string(nil), path...), "Record Type Name")...)
			if !need(at + 2) {
				put("Auxiliary Data Words", "icmpv6.mldr.mar.aux_data_len", w[at+1], at+1, 1)
			}
			if !need(at + 4) {
				put("Source Count", "icmpv6.mldr.mar.nb_sources", binary.BigEndian.Uint16(w[at+2:at+4]), at+2, 2)
			}
			if !need(at + 20) {
				put("Multicast Address", "icmpv6.mldr.mar.multicast_address", address(at+4), at+4, 16)
			}
			records = append(records, rec)
			m["Address Records"] = records
			if need(at + 20) {
				return fail("incomplete", "MLDTruncated", "address record fixed header unavailable", at+20)
			}
			ns, aux := int(binary.BigEndian.Uint16(w[at+2:at+4])), int(w[at+1])*4
			rec["Declared Record Bytes"] = 20 + ns*16 + aux
			at += 20
			sources := []string{}
			rec["Sources"] = sources
			for j := 0; j < ns; j++ {
				if need(at + 16) {
					return fail("incomplete", "MLDTruncated", "record source address unavailable", at+16)
				}
				if !charge() {
					return fail("limited", "MLDCollectionBudget", "source address collection budget exceeded", 0)
				}
				value := address(at)
				sources = append(sources, value)
				rec["Sources"] = sources
				add("icmpv6.mldr.mar.source_address", value, at, 16, append(append([]string(nil), path...), "Sources", fmt.Sprint(j))...)
				at += 16
			}
			if need(at + aux) {
				return fail("incomplete", "MLDTruncated", "record auxiliary data unavailable", at+aux)
			}
			if aux > 0 {
				rec["Auxiliary Data (uninterpreted)"] = bytes.Clone(w[at : at+aux])
				add("icmpv6.mldr.mar.auxiliary_data", hex.EncodeToString(w[at:at+aux]), at, aux, append(append([]string(nil), path...), "Auxiliary Data (uninterpreted)")...)
			}
			at += aux
		}
		return finish(at)
	}
	out.Summary = "MLD Multicast Listener Query"
	m["Message"] = "Multicast Listener Query"
	queryLengthInvalid := false
	if typ == 131 || typ == 132 {
		out.Version = 1
		m["Message"] = "Multicast Listener Report"
		if typ == 132 {
			m["Message"] = "Multicast Listener Done"
		}
		out.Summary = "MLDv1 " + m["Message"].(string)
	} else {
		length := declared
		if length == 24 {
			out.Version = 1
		} else if length >= 28 {
			out.Version = 2
		} else if length >= 24 {
			queryLengthInvalid = true
		}
	}
	if out.Version != 0 {
		m["Version"] = out.Version
		if typ == 130 {
			out.Summary = fmt.Sprintf("MLDv%d Multicast Listener Query", out.Version)
		}
	}
	if len(w) >= 6 {
		code := binary.BigEndian.Uint16(w[4:6])
		if typ != 130 {
			m["Unused Delay Word"] = code
			add("", code, 4, 2, "MLD", "Unused Delay Word")
		} else {
			m["Response Code/Delay Word"] = code
			add("icmpv6.mld.maximum_response_code.raw", code, 4, 2, "MLD", "Response Code/Delay Word")
			if out.Version == 1 {
				m["Maximum Response Delay (ms)"] = code
				add("icmpv6.mld.maximum_response_delay", code, 4, 2, "MLD", "Maximum Response Delay (ms)")
			} else if out.Version == 2 {
				value := uint32(code)
				if code >= 0x8000 {
					value = (uint32(code&0xfff) | 0x1000) << (((code >> 12) & 7) + 3)
				}
				m["Maximum Response Delay (ms)"] = value
				add("icmpv6.mld.maximum_response_code", value, 4, 2, "MLD", "Maximum Response Delay (ms)")
			}
		}
	}
	if len(w) >= 8 {
		m["Reserved"] = binary.BigEndian.Uint16(w[6:8])
		add("", m["Reserved"], 6, 2, "MLD", "Reserved")
	}
	if !need(24) {
		m["Multicast Address"] = address(8)
		add("icmpv6.mld.multicast_address", address(8), 8, 16, "MLD", "Multicast Address")
	}
	if need(24) {
		return fail("incomplete", "MLDTruncated", "query/report/done fixed header unavailable", 24)
	}
	if queryLengthInvalid {
		return fail("malformed", "MLDQueryLength", "query length selects neither v1 (24) nor v2 (at least 28)", 0)
	}
	if out.Version == 0 {
		return fail("limited", "MLDLengthUnconfirmed", "query version unavailable without a declared complete extent", 0)
	}
	if out.Version == 1 {
		return finish(24)
	}
	if !need(25) {
		flags := w[24]
		m["Flags"], m["Suppress Router-Side Processing"], m["Querier Robustness Variable"], m["Extension Flags (raw)"] = flags, flags&8 != 0, flags&7, flags>>4
		add("icmpv6.mld.flag", flags, 24, 1, "MLD", "Flags")
		add("icmpv6.mld.flag.s", flags&8 != 0, 24, 1, "MLD", "Suppress Router-Side Processing")
		add("icmpv6.mld.flag.qrv", flags&7, 24, 1, "MLD", "Querier Robustness Variable")
		add("icmpv6.mld.flag.reserved", flags>>4, 24, 1, "MLD", "Extension Flags (raw)")
	}
	if !need(26) {
		code := w[25]
		value := uint16(code)
		if code >= 128 {
			value = (uint16(code&15) | 16) << (((code >> 4) & 7) + 3)
		}
		m["Query Interval Code"], m["Query Interval (s)"] = code, value
		add("icmpv6.mld.qqi.raw", code, 25, 1, "MLD", "Query Interval Code")
		add("icmpv6.mld.qqi", value, 25, 1, "MLD", "Query Interval (s)")
	}
	if need(28) {
		return fail("incomplete", "MLDTruncated", "v2 query fixed header unavailable", 28)
	}
	n := int(binary.BigEndian.Uint16(w[26:28]))
	m["Source Count"] = uint16(n)
	add("icmpv6.mld.nb_sources", uint16(n), 26, 2, "MLD", "Source Count")
	sources := []string{}
	m["Sources"] = sources
	at := 28
	for i := 0; i < n; i++ {
		if need(at + 16) {
			return fail("incomplete", "MLDTruncated", "query source address unavailable", at+16)
		}
		if !charge() {
			return fail("limited", "MLDCollectionBudget", "source address collection budget exceeded", 0)
		}
		value := address(at)
		sources = append(sources, value)
		m["Sources"] = sources
		add("icmpv6.mld.source_address", value, at, 16, "MLD", "Sources", fmt.Sprint(i))
		at += 16
	}
	return finish(at)
}

// Range metadata follows the same nested tree shown by the inspector. Array
// leaves retain their ordinal, so repeated source/group addresses do not all
// point at the first record. Derived indices/status have no fabricated ranges.
func attachMLDWireRanges(result *MLDWireResult) {
	for _, field := range result.Values {
		var node any = result.Fields
		for index, part := range field.Path {
			switch value := node.(type) {
			case map[string]any:
				label := part
				last := index == len(field.Path)-1
				if array, ok := value[part].([]string); ok && index == len(field.Path)-2 {
					n, err := strconv.Atoi(field.Path[index+1])
					if err != nil || n < 0 || n >= len(array) {
						break
					}
					label = part + "[" + field.Path[index+1] + "]"
					last = true
				}
				if last {
					ranges, _ := value["Wire Field Ranges"].(map[string]any)
					if ranges == nil {
						ranges = map[string]any{}
						value["Wire Field Ranges"] = ranges
					}
					ranges[label] = []int{field.Offset, field.Offset + field.Length}
					node = nil
				} else {
					node = value[part]
				}
			case []map[string]any:
				n, err := strconv.Atoi(part)
				if err == nil && n >= 0 && n < len(value) {
					node = value[n]
				} else {
					node = nil
				}
			default:
				node = nil
			}
			if node == nil {
				break
			}
		}
	}
}
