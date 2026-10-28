package pcaputil

import (
	"sort"
	"strings"
)

// Nested observations are ordered by their absolute PDU spans, rather than
// map iteration. Address families and repeated join/prune groups remain in
// the same order as the original wire and Wireshark fields.
func pimFieldRows(root map[string]any) []map[string]any {
	var rows []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if _, ok := x["Wire Field Ranges"]; ok {
				rows = append(rows, x)
			}
			for key, child := range x {
				if key != "Wire Field Ranges" {
					walk(child)
				}
			}
		case []map[string]any:
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(root)
	first := func(m map[string]any) uint64 {
		at := ^uint64(0)
		if spans, ok := m["Wire Field Ranges"].(map[string]any); ok {
			for _, span := range spans {
				if s, ok := span.([]any); ok && len(s) == 2 {
					if n, ok := s[0].(uint64); ok && n < at {
						at = n
					}
				}
			}
		}
		return at
	}
	sort.SliceStable(rows, func(i, j int) bool { return first(rows[i]) < first(rows[j]) })
	return rows
}

func pimOccurrenceRange(row map[string]any, key string, available int) *DisplayFieldRange {
	ranges, _ := row["Wire Field Ranges"].(map[string]any)
	span, _ := ranges[key].([]any)
	if len(span) != 2 {
		return nil
	}
	start, a := displayUint(span[0])
	end, b := displayUint(span[1])
	if !a || !b || end <= start || end > uint64(available) {
		return nil
	}
	return &DisplayFieldRange{Offset: int(start), Length: int(end - start)}
}

func pimObservedBinding(name, label string, typ DisplayFieldType, bits int, origin string, collect func(*ProtocolEvent, func(any, *DisplayFieldRange))) displayFieldBinding {
	b := displayCustom("pim", name, label, typ, bits, origin, func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
		if n := e.nativeNetworkFields; n != nil && n.frozen && n.protocol == "pim" {
			for _, field := range n.pimValues[name] {
				emit(field.value)
			}
			return
		}
		collect(e, func(value any, _ *DisplayFieldRange) { emit(value) })
	})
	b.collectWireFields = collect
	b.rangeFor = func(e *ProtocolEvent, ordinal int) *DisplayFieldRange {
		if n := e.nativeNetworkFields; n != nil && n.frozen && n.protocol == "pim" {
			values := n.pimValues[name]
			if ordinal < 0 || ordinal >= len(values) || values[ordinal].span.Length <= 0 {
				return nil
			}
			span := values[ordinal].span
			return &span
		}
		// Standalone caller-created events have no ingress cache. Their own
		// row/range map still locates occurrences without searching Raw bytes.
		var selected *DisplayFieldRange
		index := 0
		collect(e, func(_ any, span *DisplayFieldRange) {
			if index == ordinal && span != nil {
				owned := *span
				selected = &owned
			}
			index++
		})
		return selected
	}
	b.descriptor.Description = "Observed PIM field; native ingress freezes each occurrence and its exact logical-PDU span. No raw-byte search, installed route, neighbor verification or authentication claim."
	return b
}

func (e *ProtocolEvent) retainNativePIMFields(wire []byte) {
	n := &nativeNetworkObservation{protocol: "pim", values: map[string]nativeNetworkField{}, pimValues: map[string][]nativeNetworkField{}}
	owned := *e
	owned.Session, owned.Raw = e.semanticSession, wire
	// Collect values and spans together once. Repeated snapshot queries use
	// the private arrays, rather than rewalking every record for every range.
	for _, binding := range displayFieldBindings {
		if binding.protocols != "|pim|" || binding.collectWireFields == nil {
			continue
		}
		name := binding.descriptor.Name
		binding.collectWireFields(&owned, func(value any, span *DisplayFieldRange) {
			field := nativeNetworkField{value: value}
			if span != nil {
				field.span = *span
			}
			n.pimValues[name] = append(n.pimValues[name], field)
		})
	}
	e.nativeNetworkFields = n
}

func pimAddressRows(value any) []map[string]any {
	switch rows := value.(type) {
	case []map[string]any:
		return rows
	case []any:
		var out []map[string]any
		for _, item := range rows {
			if row, ok := item.(map[string]any); ok {
				out = append(out, row)
			}
		}
		return out
	}
	return nil
}

func pimDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	add := func(name, key string, typ DisplayFieldType, bits int, nested bool) {
		b := pimObservedBinding(name, key, typ, bits, "pdu", func(e *ProtocolEvent, emit func(any, *DisplayFieldRange)) {
			rows := []map[string]any{e.Session}
			if nested {
				rows = pimFieldRows(e.Session)
			}
			for _, m := range rows {
				if strings.HasPrefix(name, "pim.source_addr.") && m["Kind"] != "source" {
					continue
				}
				if strings.HasPrefix(name, "pim.group_addr.") && m["Kind"] != "group" {
					continue
				}
				if v, ok := m[key]; ok {
					emit(v, pimOccurrenceRange(m, key, len(e.Raw)))
				}
			}
		})
		out = append(out, b)
	}
	for _, f := range []struct {
		name, key string
		bits      int
		nested    bool
	}{
		{"version", "Version", 8, false}, {"type", "Type", 8, false}, {"cksum", "Checksum", 16, false},
		{"optiontype", "Option Type", 16, true}, {"optionlength", "Option Length", 16, true},
		{"holdtime", "Hold Time", 16, true}, {"propagation_delay", "Propagation Delay", 16, true}, {"override_interval", "Override Interval", 16, true},
		{"dr_priority", "DR Priority", 32, true}, {"generation_id", "Generation ID", 32, true},
		{"state_refresh_version", "State Refresh Version", 8, true}, {"state_refresh_interval", "State Refresh Interval", 8, true}, {"state_refresh_reserved", "State Refresh Reserved", 16, true},
		{"register_flag", "Register Flags", 32, false}, {"metric_pref", "Metric Preference", 32, false}, {"metric", "Metric", 32, false},
		{"numgroups", "Group Count", 16, false}, {"numjoins", "Join Count", 16, true}, {"numprunes", "Prune Count", 16, true},
		{"fragment_tag", "Fragment Tag", 16, false}, {"hash_mask_len", "Hash Mask Length", 8, false}, {"bsr_priority", "BSR Priority", 8, false},
		{"rp_count", "RP Count", 8, true}, {"frp_count", "Fragment RP Count", 8, true}, {"priority", "Priority", 8, true}, {"prefix_count", "Prefix Count", 8, false},
		{"mask_len", "Mask Length", 8, true}, {"addr_address_family", "Address Family", 8, true}, {"addr_encoding_type", "Encoding", 8, true},
		{"ttl", "TTL", 8, false}, {"interval", "Interval", 8, false},
		{"ip_version", "IP Version", 8, true},
	} {
		add("pim."+f.name, f.key, DisplayUnsigned, f.bits, f.nested)
	}
	for _, f := range []struct {
		name, key string
		nested    bool
	}{
		{"t", "Tracking Support", true}, {"register_flag.border", "Border", false}, {"register_flag.null_register", "Null Register", false},
		{"rpt", "RPT", false}, {"prune_indicator", "Prune Indicator", false}, {"prune_now", "Prune Now", false}, {"assert_override", "Assert Override", false},
		{"group_addr.flags.b", "Bidirectional", true}, {"group_addr.flags.z", "Admin Scope Zone", true}, {"source_addr.flags.s", "Sparse", true}, {"source_addr.flags.w", "Wildcard", true}, {"source_addr.flags.r", "RPT", true},
	} {
		add("pim."+f.name, f.key, DisplayBoolean, 0, f.nested)
	}
	for _, f := range []struct{ name, path string }{
		{"upstream_neighbor", "Upstream Neighbor"}, {"source", "Source"}, {"group", "Group"}, {"originator", "Originator"}, {"bsr", "BSR"}, {"rp", "RP"},
	} {
		for _, v6 := range []bool{false, true} {
			name := f.name
			if v6 {
				name += "_ip6"
			}
			path := f.path
			b := pimObservedBinding("pim."+name, path, DisplayAddress, 0, "pdu", func(e *ProtocolEvent, emit func(any, *DisplayFieldRange)) {
				for _, row := range pimFieldRows(e.Session) {
					// Wireshark uses ip/ipv6 fields for versioned Register IP
					// headers, and pim.source/group only for version-zero dummy
					// headers. Keep our useful Register addresses in their own
					// observation namespace instead of changing that contract.
					if _, register := row["Dummy Header"]; register && routingNumber(row, "IP Version") != 0 {
						continue
					}
					owner, key := row, path
					v := row[key]
					if path == "Group" && row["Kind"] == "group" {
						key, v = "Address", row["Address"]
					}
					if path == "Source" && row["Kind"] == "source" {
						key, v = "Address", row["Address"]
					}
					if path != "Group" {
						if a, ok := v.(map[string]any); ok {
							owner, key, v = a, "Address", a["Address"]
						}
					}
					if s, ok := v.(string); ok && strings.Contains(s, ":") == v6 {
						emit(s, pimOccurrenceRange(owner, key, len(e.Raw)))
					}
				}
			})
			out = append(out, b)
		}
	}
	for _, f := range []struct{ name, path string }{{"join_ip", "Joined Sources"}, {"prune_ip", "Pruned Sources"}, {"address_list", "Addresses"}} {
		for _, v6 := range []bool{false, true} {
			name := f.name
			if v6 {
				if name == "address_list" {
					name += "_ip6"
				} else {
					name += "6"
				}
			}
			path := f.path
			out = append(out, pimObservedBinding("pim."+name, path, DisplayAddress, 0, "pdu", func(e *ProtocolEvent, emit func(any, *DisplayFieldRange)) {
				for _, row := range pimFieldRows(e.Session) {
					for _, address := range pimAddressRows(row[path]) {
						if s, ok := address["Address"].(string); ok && strings.Contains(s, ":") == v6 {
							emit(s, pimOccurrenceRange(address, "Address", len(e.Raw)))
						}
					}
				}
			}))
		}
	}
	out = append(out, pimObservedBinding("pim.observed.checksum_verified", "Checksum Verified", DisplayBoolean, 0, "context", func(e *ProtocolEvent, emit func(any, *DisplayFieldRange)) {
		if value, present := e.Session["Checksum Verified"]; present {
			emit(value, nil)
		}
	}))
	for _, key := range []string{"Source", "Group"} {
		key := key
		b := pimObservedBinding("pim.observed.register."+strings.ToLower(key), key, DisplayAddress, 0, "pdu", func(e *ProtocolEvent, emit func(any, *DisplayFieldRange)) {
			if header, ok := e.Session["Register IP Header"].(map[string]any); ok {
				if value, present := header[key]; present {
					emit(value, pimOccurrenceRange(header, key, len(e.Raw)))
				}
			}
		})
		out = append(out, b)
	}
	return out
}
