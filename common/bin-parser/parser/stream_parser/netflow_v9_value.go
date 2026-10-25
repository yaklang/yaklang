package stream_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/netip"
)

// NetFlowV9Field is an observed template specifier. Scope types have their own
// namespace; they must never be interpreted as ordinary information elements.
type NetFlowV9Field struct {
	Type, Length uint16
	Scope        bool
}

// NetFlowV9Template records the actual schema source. A caller supplying
// templates is responsible for exporter/domain/generation/expiry isolation.
// Definition offsets refer to the source PDU, never the current data packet.
type NetFlowV9Template struct {
	ID                                 uint16
	Fields                             []NetFlowV9Field
	Options                            bool
	RecordLength                       int
	PDUSHA256, DefinitionSHA256        string
	DefinitionOffset, DefinitionLength int
	Sequence                           uint32
}

type NetFlowV9Result struct {
	Fields    map[string]any
	Templates map[uint16]NetFlowV9Template
}

// CloneNetFlowV9Templates gives a caller its own schema slices, independently
// of both the input and the public decoded field maps.
func CloneNetFlowV9Templates(in map[uint16]NetFlowV9Template) map[uint16]NetFlowV9Template {
	out := make(map[uint16]NetFlowV9Template, len(in))
	for id, t := range in {
		t.Fields = append([]NetFlowV9Field(nil), t.Fields...)
		out[id] = t
	}
	return out
}

type netflowV9DataSet struct {
	row        map[string]any
	id         uint16
	start, end int
	template   NetFlowV9Template
	choices    []int
}

func netflowV9Hash(b []byte) string {
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

// DecodeNetFlowV9Wire checks one UDP message, without owning a transport cache.
// Template updates are transactional: on any error Templates is nil. Same-PDU
// templates are available even after their Data FlowSet (RFC3954 section7).
// Unknown schemas and ambiguous record/padding boundaries remain explicit.
func DecodeNetFlowV9Wire(w []byte, previous map[uint16]NetFlowV9Template, limit int) (result NetFlowV9Result, err error) {
	if len(w) < 20 || len(w) > 65527 {
		return result, fmt.Errorf("netflow-v9: complete bounded header required")
	}
	if binary.BigEndian.Uint16(w) != 9 {
		return result, fmt.Errorf("%w: unselected NetFlow version", ErrObservedWireContext)
	}
	if limit <= 0 {
		return result, ErrObservedWireBudget
	}
	limit = min(limit, 4096)
	r := &controlReader{w: w, info: map[string]any{
		"Protocol Name": "NetFlow v9", "Packet Name": "Flow Export",
		"Observation Scope":       "reported flow records; no captured TCP connections, inferred delivery, absolute flow times or authenticated exporter",
		"Authentication Verified": false, "Wire Framing Complete": false, "Record Count Verified": false,
	}}
	result.Fields = r.info
	r.num("Version", 2, false)
	declared := int(r.num("Header Record Count", 2, false))
	r.num("System Uptime Milliseconds", 4, false)
	r.num("Unix Seconds", 4, false)
	seq := uint32(r.num("Sequence Number", 4, false))
	r.num("Source ID", 4, false)
	r.info["Fixed Header Complete"] = true
	r.info["Wire Field Ranges"] = routingFieldRanges(r.fields)
	if declared == 0 || len(w) == 20 {
		return result, fmt.Errorf("%w: empty NetFlow v9 export outside selected profile", ErrObservedWireContext)
	}
	if declared > limit {
		return result, ErrObservedWireBudget
	}
	contextFields := 0
	for id, t := range previous {
		contextFields += len(t.Fields) + 1
		if contextFields > limit {
			return result, ErrObservedWireBudget
		}
		width, seenNormal := 0, false
		valid := id >= 256 && t.ID == id && len(t.Fields) > 0
		for _, f := range t.Fields {
			valid = valid && (f.Length > 0 || f.Scope && f.Type == 1) && (!f.Scope || t.Options && !seenNormal)
			seenNormal = seenNormal || !f.Scope
			width += int(f.Length)
		}
		if !valid || width != t.RecordLength {
			return result, fmt.Errorf("%w: invalid caller-supplied NetFlow v9 schema", ErrObservedWireContext)
		}
	}
	working := CloneNetFlowV9Templates(previous)
	definitions := map[uint16]NetFlowV9Template{}
	conflicts := map[uint16]bool{}
	var pending []netflowV9DataSet
	var sets []map[string]any
	used, templateCount := 0, 0
	charge := func(n int) bool {
		if n < 0 || n > limit-used {
			r.err = ErrObservedWireBudget
			return false
		}
		used += n
		return true
	}
	pduHash := netflowV9Hash(w)
	for r.at < len(w) && r.err == nil {
		start := r.at
		if len(w)-start < 4 {
			r.err = fmt.Errorf("netflow-v9: partial FlowSet header")
			break
		}
		size := int(binary.BigEndian.Uint16(w[start+2:]))
		if size < 4 || size > len(w)-start || !charge(1) {
			if r.err == nil {
				r.err = fmt.Errorf("netflow-v9: FlowSet length exceeds message")
			}
			break
		}
		row := r.child("FlowSet", start+size, func(c *controlReader) {
			id := uint16(c.num("FlowSet ID", 2, false))
			c.num("FlowSet Length", 2, false)
			if id == 0 || id == 1 {
				var templates []map[string]any
				for c.at+4 <= len(c.w) && c.err == nil {
					at := c.at
					t := NetFlowV9Template{Options: id == 1, PDUSHA256: pduHash, DefinitionOffset: at, Sequence: seq}
					minimum := 4
					if t.Options {
						minimum = 6
					}
					if len(c.w)-at < minimum || !charge(1) {
						if r.err != nil {
							c.err = r.err
						} else {
							c.err = fmt.Errorf("netflow-v9: partial template header")
						}
						break
					}
					t.ID = binary.BigEndian.Uint16(w[at:])
					count, scope := int(binary.BigEndian.Uint16(w[at+2:])), 0
					if t.Options {
						options := int(binary.BigEndian.Uint16(w[at+4:]))
						if count == 0 || count%4 != 0 || options%4 != 0 {
							c.err = fmt.Errorf("netflow-v9: invalid options scope/field byte lengths")
							break
						}
						scope, count = count/4, (count+options)/4
					}
					if t.ID < 256 || count == 0 || count > (len(c.w)-at-minimum)/4 {
						c.err = fmt.Errorf("netflow-v9: invalid template ID/count/boundary")
						break
					}
					if !charge(count) {
						c.err = r.err
						break
					}
					definition := c.child("Template", at+minimum+4*count, func(d *controlReader) {
						d.num("Template ID", 2, false)
						if t.Options {
							d.num("Scope Length", 2, false)
							d.num("Options Length", 2, false)
						} else {
							d.num("Field Count", 2, false)
						}
						var fields []map[string]any
						for i := 0; i < count && d.err == nil; i++ {
							f := NetFlowV9Field{Scope: i < scope}
							field := d.child("Field Specifier", d.at+4, func(s *controlReader) {
								f.Type = uint16(s.num("Field Type", 2, false))
								f.Length = uint16(s.num("Field Length", 2, false))
								s.info["Scope"] = f.Scope
								s.need(f.Length > 0 || f.Scope && f.Type == 1, "zero-width NetFlow v9 field outside System scope")
								s.info["Wire Field Ranges"] = routingFieldRanges(s.fields)
							})
							if d.err == nil {
								t.RecordLength += int(f.Length)
								t.Fields = append(t.Fields, f)
								fields = append(fields, field)
							}
						}
						d.info["Fields"], d.info["Options Template"] = fields, t.Options
						d.info["Wire Field Ranges"] = routingFieldRanges(d.fields)
					})
					if c.err != nil {
						break
					}
					t.DefinitionLength = c.at - at
					t.DefinitionSHA256 = netflowV9Hash(w[at:c.at])
					definition["Definition SHA256"] = t.DefinitionSHA256
					if old, ok := definitions[t.ID]; ok && old.DefinitionSHA256 != t.DefinitionSHA256 {
						conflicts[t.ID] = true
					}
					definitions[t.ID] = t
					templateCount++
					templates = append(templates, definition)
				}
				c.info["Templates"] = templates
				if c.err == nil && c.at < len(c.w) {
					padding := c.rest("Padding Bytes")
					if !bytes.Equal(padding, make([]byte, len(padding))) {
						c.info["Conformance Warning"] = "nonzero padding retained; RFC3954 recommends, but does not require, zeros"
					}
				}
			} else {
				lo := c.at
				c.rest("Opaque Data")
				pending = append(pending, netflowV9DataSet{row: c.info, id: id, start: lo, end: len(c.w)})
			}
			c.info["Wire Field Ranges"] = routingFieldRanges(c.fields)
		})
		if r.err == nil {
			sets = append(sets, row)
		}
	}
	r.info["FlowSets"], r.info["Template Record Count"] = sets, uint64(templateCount)
	if r.err != nil {
		return result, r.err
	}
	r.info["Wire Framing Complete"] = true
	r.info["Data FlowSet Count"] = uint64(len(pending))
	r.info["Known FlowSet Namespace"] = true
	for id, t := range definitions {
		working[id] = t
	}
	var choices [][]int
	missing := false
	for i := range pending {
		p := &pending[i]
		if p.id < 256 {
			r.info["Known FlowSet Namespace"] = false
		}
		t, ok := working[p.id]
		if p.id < 256 || !ok || conflicts[p.id] || t.RecordLength <= 0 {
			p.row["Data Decode Unavailable"] = "template not observed, reserved FlowSet namespace or conflicting same-PDU definitions"
			missing = true
			continue
		}
		p.template = t
		for padding := 0; padding <= 3 && padding < p.end-p.start; padding++ {
			body := p.end - p.start - padding
			if body%t.RecordLength == 0 && body/t.RecordLength > 0 {
				p.choices = append(p.choices, body/t.RecordLength)
			}
		}
		if len(p.choices) == 0 {
			return result, fmt.Errorf("%w: no complete record layout with selected zero-to-three-byte padding", ErrObservedWireContext)
		}
		choices = append(choices, p.choices)
	}
	if missing || len(conflicts) > 0 {
		r.info["Data Decode Unavailable"] = "complete record count requires unambiguous observed templates for every Data FlowSet"
		return result, fmt.Errorf("%w: NetFlow v9 template context unavailable", ErrObservedWireContext)
	}
	counts, countErr := netflowV9RecordCounts(choices, declared-templateCount, limit)
	if countErr != nil {
		return result, countErr
	}
	total := 0
	for i := range pending {
		p := &pending[i]
		t := p.template
		if !charge(counts[i] * (len(t.Fields) + 1)) {
			return result, r.err
		}
		var records []map[string]any
		at := p.start
		for index := 0; index < counts[i]; index++ {
			fields := []map[string]any{}
			for _, f := range t.Fields {
				end := at + int(f.Length)
				value, kind, name := netflowV9Value(f, w[at:end])
				ranges := map[string]any{}
				if end > at {
					ranges["Value"] = []any{uint64(at), uint64(end)}
				}
				fields = append(fields, map[string]any{"Field Type": uint64(f.Type), "Field Length": uint64(f.Length), "Scope": f.Scope,
					"Field Name": name, "Value Kind": kind, "Value": value, "Wire Field Ranges": ranges})
				if kind == "opaque" {
					r.info["Unselected Field Semantics"] = true
				}
				at = end
			}
			records = append(records, map[string]any{"Index": uint64(index + 1), "Fields": fields})
		}
		delete(p.row, "Opaque Data")
		p.row["Records"], p.row["Record Count"] = records, uint64(counts[i])
		p.row["Template Definition SHA256"], p.row["Template PDU SHA256"] = t.DefinitionSHA256, t.PDUSHA256
		p.row["Template Definition Offset"], p.row["Template Definition Length"] = uint64(t.DefinitionOffset), uint64(t.DefinitionLength)
		p.row["Template Sequence Number"] = uint64(t.Sequence)
		p.row["Template Scope"] = "observed caller-supplied context"
		if _, ok := definitions[p.id]; ok {
			p.row["Template Scope"] = "same PDU"
		}
		if at < p.end {
			p.row["Padding Bytes"] = bytes.Clone(w[at:p.end])
			p.row["Wire Field Ranges"].(map[string]any)["Padding Bytes"] = []any{uint64(at), uint64(p.end)}
			if !bytes.Equal(w[at:p.end], make([]byte, p.end-at)) {
				p.row["Conformance Warning"] = "nonzero padding retained; RFC3954 recommends, but does not require, zeros"
			}
		}
		total += counts[i]
	}
	r.info["Data Record Count"], r.info["Record Count Verified"] = uint64(total), true
	result.Templates = working
	return result, nil
}

// The header's total record count may resolve a short-record/padding ambiguity.
// If two assignments fit, neither interpretation may be reported as certain.
func netflowV9RecordCounts(choices [][]int, target, limit int) ([]int, error) {
	if target < 0 {
		return nil, fmt.Errorf("netflow-v9: header record count smaller than template count")
	}
	out := make([]int, len(choices))
	var ambiguous []int
	for i, c := range choices {
		if len(c) == 1 {
			out[i], target = c[0], target-c[0]
		} else {
			ambiguous = append(ambiguous, i)
		}
	}
	if target < 0 {
		return nil, fmt.Errorf("%w: header record count does not fit selected record/padding layout", ErrObservedWireContext)
	}
	type path struct{ ways, prev, count int }
	trace := []map[int]path{{0: {ways: 1}}}
	states := 1
	for _, index := range ambiguous {
		next := map[int]path{}
		for sum, p := range trace[len(trace)-1] {
			for _, n := range choices[index] {
				if sum+n > target {
					continue
				}
				v := next[sum+n]
				next[sum+n] = path{ways: min(2, v.ways+p.ways), prev: sum, count: n}
			}
		}
		states += len(next)
		if states > limit*8 {
			return nil, ErrObservedWireBudget
		}
		trace = append(trace, next)
	}
	answer := trace[len(trace)-1][target]
	if answer.ways == 0 {
		return nil, fmt.Errorf("%w: header record count does not fit selected record/padding layout", ErrObservedWireContext)
	}
	if answer.ways != 1 {
		return nil, fmt.Errorf("%w: ambiguous NetFlow v9 record/padding counts", ErrObservedWireContext)
	}
	for i := len(ambiguous) - 1; i >= 0; i-- {
		p := trace[i+1][target]
		out[ambiguous[i]], target = p.count, p.prev
	}
	return out, nil
}

type netflowV9ValueSpec struct {
	name, kind string
	width      int // zero denotes a variable-width unsigned counter, up to64 bits
}

var netflowV9Values = map[uint16]netflowV9ValueSpec{
	1: {"Reported Octets", "unsigned", 0}, 2: {"Reported Packets", "unsigned", 0}, 3: {"Reported Aggregated Flows", "unsigned", 0},
	4: {"Reported IP Protocol", "unsigned", 1}, 5: {"Reported ToS", "unsigned", 1}, 6: {"Reported TCP Flags OR", "unsigned", 1},
	7: {"Reported Source Port", "unsigned", 2}, 8: {"Reported Source IPv4", "IPv4", 4}, 9: {"Reported Source Mask", "unsigned", 1},
	10: {"Reported Input Interface", "unsigned", 0}, 11: {"Reported Destination Port", "unsigned", 2}, 12: {"Reported Destination IPv4", "IPv4", 4},
	13: {"Reported Destination Mask", "unsigned", 1}, 14: {"Reported Output Interface", "unsigned", 0}, 15: {"Reported Next Hop IPv4", "IPv4", 4},
	16: {"Reported Source AS", "AS", 0}, 17: {"Reported Destination AS", "AS", 0}, 18: {"Reported BGP Next Hop IPv4", "IPv4", 4},
	19: {"Reported Multicast Packets", "unsigned", 0}, 20: {"Reported Multicast Octets", "unsigned", 0},
	21: {"Reported Last Uptime Milliseconds", "unsigned", 4}, 22: {"Reported First Uptime Milliseconds", "unsigned", 4},
	23: {"Reported Output Octets", "unsigned", 0}, 24: {"Reported Output Packets", "unsigned", 0},
	27: {"Reported Source IPv6", "IPv6", 16}, 28: {"Reported Destination IPv6", "IPv6", 16},
	29: {"Reported Source IPv6 Mask", "unsigned", 1}, 30: {"Reported Destination IPv6 Mask", "unsigned", 1},
	31: {"Reported IPv6 Flow Label", "unsigned", 3}, 32: {"Reported ICMP Type And Code", "unsigned", 2},
	34: {"Reported Sampling Interval", "unsigned", 4}, 35: {"Reported Sampling Algorithm", "unsigned", 1},
	36: {"Reported Active Timeout Seconds", "unsigned", 2}, 37: {"Reported Inactive Timeout Seconds", "unsigned", 2},
	38: {"Reported Engine Type", "unsigned", 1}, 39: {"Reported Engine ID", "unsigned", 1},
}

func netflowV9Value(f NetFlowV9Field, b []byte) (any, string, string) {
	if f.Scope && f.Type == 1 && len(b) == 0 {
		// Receiver-compatible System scope with no value octets. The meaning
		// comes from its template, so it must not acquire a data-byte range.
		return "entire observed exporter (no value octets)", "scope", "Reported System Scope"
	}
	s, ok := netflowV9Values[f.Type]
	if !ok || f.Scope || s.width != 0 && s.width != len(b) || s.width == 0 && (len(b) == 0 || len(b) > 8) || s.kind == "AS" && len(b) != 2 && len(b) != 4 {
		return bytes.Clone(b), "opaque", fmt.Sprintf("Reported Field %d (scope=%t)", f.Type, f.Scope)
	}
	if s.kind == "IPv4" {
		return netip.AddrFrom4([4]byte(b)).String(), s.kind, s.name
	}
	if s.kind == "IPv6" {
		return netip.AddrFrom16([16]byte(b)).String(), s.kind, s.name
	}
	var n uint64
	for _, v := range b {
		n = n<<8 | uint64(v)
	}
	return n, "unsigned", s.name
}
