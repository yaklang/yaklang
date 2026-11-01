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
	ErrIGMPTruncated = errors.New("IGMP bytes unavailable")
	ErrIGMPMalformed = errors.New("IGMP membership wire layout invalid")
)

// DeclaredBytes is the enclosing IPv4 payload extent, excluding IP padding.
// A negative value denotes an unconfirmed legacy extent, never a complete PDU.
type IGMPWireContext struct{ DeclaredBytes int }

type IGMPWireField struct {
	Name           string
	Value          any
	Offset, Length int
	Path           []string
}

type IGMPWireResult struct {
	Fields  map[string]any
	Values  []IGMPWireField
	Version uint8
	Summary string
}

type IGMPWireError struct {
	Kind, Code, Reason  string
	Available, Required int
}

func (e *IGMPWireError) Error() string {
	if e.Required > e.Available {
		return fmt.Sprintf("IGMP %s (%d/%d bytes)", e.Reason, e.Available, e.Required)
	}
	return "IGMP " + e.Reason
}

func (e *IGMPWireError) Unwrap() error {
	if e.Kind == "incomplete" {
		return ErrIGMPTruncated
	}
	if e.Kind == "limited" {
		if e.Code == "IGMPCollectionBudget" {
			return ErrObservedWireBudget
		}
		return ErrObservedWireContext
	}
	return ErrIGMPMalformed
}

// IGMPMembershipType deliberately excludes DVMRP(0x13), PIMv1(0x14), mtrace
// and unknown IP-protocol2 uses. Their own observers must retain ownership.
func IGMPMembershipType(typ uint8) bool {
	switch typ {
	case 0x11, 0x12, 0x16, 0x17, 0x22:
		return true
	}
	return false
}

func igmpFloatingCode(code uint8) uint16 {
	if code < 128 {
		return uint16(code)
	}
	return (uint16(code&15) | 16) << (((code >> 4) & 7) + 3)
}

// DecodeIGMPWire implements RFC1112/2236 and RFC9776 (which obsoletes3376)
// membership layouts. It verifies captured bounds, not the checksum, Router
// Alert, TTL, address validity, host identity, membership or router state.
// Each repeated wire value retains its own ordinal and exact byte range.
func DecodeIGMPWire(w []byte, maxBytes, maxElements int, context ...IGMPWireContext) (out IGMPWireResult, err error) {
	defer func() { attachIGMPWireRanges(&out) }()
	if maxBytes <= 0 || len(w) > maxBytes || maxElements <= 0 {
		return out, ErrObservedWireBudget
	}
	if len(context) > 1 {
		return out, fmt.Errorf("IGMP: at most one enclosing extent is allowed")
	}
	declared := len(w)
	if len(context) == 1 {
		declared = context[0].DeclaredBytes
	}
	m := map[string]any{"Observation Scope": "captured membership wire fields; checksum, IPv4 envelope and membership state unverified"}
	out.Fields = map[string]any{"IGMP": m}
	out.Summary = "IPv4 protocol2; IGMP membership header unavailable"
	add := func(name string, value any, at, size int, path ...string) {
		out.Values = append(out.Values, IGMPWireField{name, value, at, size, append([]string(nil), path...)})
	}
	put := func(key, alias string, value any, at, size int) {
		m[key] = value
		add(alias, value, at, size, "IGMP", key)
	}
	fail := func(kind, code, reason string, required int) (IGMPWireResult, error) {
		m["Status"], m["Error"] = kind, reason
		m["Available IGMP Bytes"] = len(w)
		if required > len(w) {
			m["Minimum Bytes Needed for Next Field"] = required
		}
		out.Summary += "; " + reason
		return out, &IGMPWireError{kind, code, reason, len(w), required}
	}
	if len(w) == 0 {
		return fail("incomplete", "IGMPTruncated", "type octet unavailable", 1)
	}
	typ := w[0]
	out.Fields["Type"] = typ
	add("igmp.type", typ, 0, 1, "Type")
	if !IGMPMembershipType(typ) {
		return fail("limited", "IGMPTypeUnselected", "type outside selected membership profile", 0)
	}
	if declared >= 0 && declared < len(w) {
		return fail("malformed", "IGMPEnvelopeLength", "captured bytes exceed enclosing declared extent", 0)
	}
	if len(w) >= 4 {
		out.Fields["Checksum"] = binary.BigEndian.Uint16(w[2:4])
		add("igmp.checksum", out.Fields["Checksum"], 2, 2, "Checksum")
	}
	address := func(at int) string { return netip.AddrFrom4([4]byte(w[at : at+4])).String() }
	used := 0
	charge := func() bool {
		if used >= maxElements {
			return false
		}
		used++
		return true
	}
	finish := func(at int) (IGMPWireResult, error) {
		if at < len(w) {
			put("Additional Data (uninterpreted)", "", bytes.Clone(w[at:]), at, len(w)-at)
		}
		if declared > len(w) {
			return fail("incomplete", "IGMPEnvelopeTruncated", "declared enclosing message tail unavailable", declared)
		}
		if declared < 0 {
			return fail("limited", "IGMPLengthUnconfirmed", "declared enclosing message extent unavailable", 0)
		}
		return out, nil
	}
	if typ == 0x22 {
		out.Version, out.Summary = 3, "IGMPv3 Membership Report"
		m["Version"], m["Message"] = uint8(3), "Membership Report"
		if len(w) >= 2 {
			put("Reserved Octet", "", w[1], 1, 1)
		}
		if len(w) >= 6 {
			put("Report Flags (raw)", "", binary.BigEndian.Uint16(w[4:6]), 4, 2)
		}
		if len(w) < 8 {
			return fail("incomplete", "IGMPTruncated", "v3 report fixed header unavailable", 8)
		}
		count := int(binary.BigEndian.Uint16(w[6:8]))
		put("Group Record Count", "igmp.num_grp_recs", uint16(count), 6, 2)
		records := []map[string]any{}
		m["Group Records"] = records
		at := 8
		for i := 0; i < count; i++ {
			if at >= len(w) {
				return fail("incomplete", "IGMPTruncated", "group record fixed header unavailable", at+8)
			}
			if !charge() {
				return fail("limited", "IGMPCollectionBudget", "group record collection budget exceeded", 0)
			}
			rec := map[string]any{"Index": i + 1}
			path := []string{"IGMP", "Group Records", fmt.Sprint(i)}
			set := func(key, alias string, value any, offset, length int) {
				rec[key] = value
				add(alias, value, offset, length, append(append([]string(nil), path...), key)...)
			}
			set("Record Type", "igmp.record_type", w[at], at, 1)
			names := [...]string{"", "MODE_IS_INCLUDE", "MODE_IS_EXCLUDE", "CHANGE_TO_INCLUDE_MODE", "CHANGE_TO_EXCLUDE_MODE", "ALLOW_NEW_SOURCES", "BLOCK_OLD_SOURCES"}
			if w[at] > 0 && int(w[at]) < len(names) {
				rec["Record Type Name"] = names[w[at]]
			} else {
				rec["Record Type Name"] = "unrecognized; wire record only"
			}
			if at+2 <= len(w) {
				set("Auxiliary Data Words", "igmp.aux_data_len", w[at+1], at+1, 1)
			}
			if at+4 <= len(w) {
				set("Source Count", "igmp.num_src", binary.BigEndian.Uint16(w[at+2:at+4]), at+2, 2)
			}
			if at+8 <= len(w) {
				set("Multicast Address", "igmp.maddr", address(at+4), at+4, 4)
			}
			records = append(records, rec)
			m["Group Records"] = records
			if at+8 > len(w) {
				return fail("incomplete", "IGMPTruncated", "group record fixed header unavailable", at+8)
			}
			ns, aux := int(binary.BigEndian.Uint16(w[at+2:at+4])), int(w[at+1])*4
			rec["Declared Record Bytes"] = 8 + ns*4 + aux
			at += 8
			sources := []string{}
			rec["Sources"] = sources
			for j := 0; j < ns; j++ {
				if at+4 > len(w) {
					return fail("incomplete", "IGMPTruncated", "record source address unavailable", at+4)
				}
				if !charge() {
					return fail("limited", "IGMPCollectionBudget", "source address collection budget exceeded", 0)
				}
				v := address(at)
				sources = append(sources, v)
				rec["Sources"] = sources
				add("igmp.saddr", v, at, 4, append(append([]string(nil), path...), "Sources", fmt.Sprint(j))...)
				at += 4
			}
			if at+aux > len(w) {
				return fail("incomplete", "IGMPTruncated", "record auxiliary data unavailable", at+aux)
			}
			if aux > 0 {
				set("Auxiliary Data (uninterpreted)", "igmp.aux_data", bytes.Clone(w[at:at+aux]), at, aux)
				// The typed byte-string value is hex; the semantic tree owns bytes.
				out.Values[len(out.Values)-1].Value = hex.EncodeToString(w[at : at+aux])
			}
			at += aux
		}
		return finish(at)
	}
	message := "Membership Query"
	if typ == 0x12 {
		out.Version, message = 1, "Membership Report"
	}
	if typ == 0x16 {
		out.Version, message = 2, "Membership Report"
	}
	if typ == 0x17 {
		out.Version, message = 2, "Leave Group"
	}
	queryLengthInvalid := false
	if typ == 0x11 && len(w) >= 2 {
		length := declared
		if length == 8 {
			out.Version = 2
			if w[1] == 0 {
				out.Version = 1
			}
		} else if length >= 12 {
			out.Version = 3
		} else if length >= 8 {
			queryLengthInvalid = true
		}
		put("Max Response Code (raw)", "igmp.max_resp", w[1], 1, 1)
		if out.Version != 0 {
			value := uint32(w[1]) * 100
			if out.Version == 1 {
				value = 10000
			}
			if out.Version == 3 {
				value = uint32(igmpFloatingCode(w[1])) * 100
			}
			put("Maximum Response Time (ms)", "igmp.max_resp.decoded_ms", value, 1, 1)
		}
	} else if typ != 0x11 && len(w) >= 2 {
		put("Reserved Response Octet", "", w[1], 1, 1)
	}
	m["Message"] = message
	out.Summary = "IGMP " + message
	if out.Version != 0 {
		m["Version"] = out.Version
		out.Summary = fmt.Sprintf("IGMPv%d %s", out.Version, message)
	}
	if len(w) >= 8 {
		put("Group Address", "igmp.maddr", address(4), 4, 4)
	}
	if len(w) < 8 {
		return fail("incomplete", "IGMPTruncated", "query/report fixed header unavailable", 8)
	}
	if queryLengthInvalid {
		return fail("malformed", "IGMPQueryLength", "query selects neither 8-byte v1/v2 nor at-least-12-byte v3 layout", 0)
	}
	if typ == 0x11 && out.Version == 0 {
		return fail("limited", "IGMPLengthUnconfirmed", "query version unavailable without declared enclosing length", 0)
	}
	if typ != 0x11 || out.Version != 3 {
		return finish(8)
	}
	if len(w) >= 9 {
		flags := w[8]
		put("Query Flags", "igmp.query.flags", flags, 8, 1)
		put("Suppress Router-Side Processing", "igmp.s", flags&8 != 0, 8, 1)
		put("Querier Robustness Variable", "igmp.qrv", flags&7, 8, 1)
		put("Extension Flags (raw)", "igmp.query.reserved", flags>>4, 8, 1)
	}
	if len(w) >= 10 {
		put("Query Interval Code (raw)", "igmp.qqic", w[9], 9, 1)
		put("Query Interval (seconds)", "igmp.qqic.decoded_seconds", igmpFloatingCode(w[9]), 9, 1)
	}
	if len(w) < 12 {
		return fail("incomplete", "IGMPTruncated", "v3 query fixed header unavailable", 12)
	}
	ns := int(binary.BigEndian.Uint16(w[10:12]))
	put("Source Count", "igmp.num_src", uint16(ns), 10, 2)
	sources := []string{}
	m["Sources"] = sources
	at := 12
	for i := 0; i < ns; i++ {
		if at+4 > len(w) {
			return fail("incomplete", "IGMPTruncated", "query source address unavailable", at+4)
		}
		if !charge() {
			return fail("limited", "IGMPCollectionBudget", "source address collection budget exceeded", 0)
		}
		v := address(at)
		sources = append(sources, v)
		m["Sources"] = sources
		add("igmp.saddr", v, at, 4, "IGMP", "Sources", fmt.Sprint(i))
		at += 4
	}
	return finish(at)
}

// Leaf paths include array ordinals, so repeated addresses do not share the
// first record's span. Status, version and derived record indices have no span.
func attachIGMPWireRanges(r *IGMPWireResult) {
	for _, f := range r.Values {
		var node any = r.Fields
		for i, part := range f.Path {
			switch v := node.(type) {
			case map[string]any:
				key, last := part, i == len(f.Path)-1
				if a, ok := v[part].([]string); ok && i == len(f.Path)-2 {
					n, e := strconv.Atoi(f.Path[i+1])
					if e != nil || n < 0 || n >= len(a) {
						node = nil
						break
					}
					key, last = part+"["+f.Path[i+1]+"]", true
				}
				if last {
					ranges, _ := v["Wire Field Ranges"].(map[string]any)
					if ranges == nil {
						ranges = map[string]any{}
						v["Wire Field Ranges"] = ranges
					}
					ranges[key] = []int{f.Offset, f.Offset + f.Length}
					node = nil
				} else {
					node = v[part]
				}
			case []map[string]any:
				n, e := strconv.Atoi(part)
				if e != nil || n < 0 || n >= len(v) {
					node = nil
				} else {
					node = v[n]
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
