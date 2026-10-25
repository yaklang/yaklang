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

func pimDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	add := func(name, key string, typ DisplayFieldType, bits int, nested bool) {
		b := displayRoot("pim", name, key, typ, bits, "pdu", key)
		if nested {
			b = displayCustom("pim", name, key, typ, bits, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
				for _, m := range pimFieldRows(e.Session) {
					if strings.HasPrefix(name, "pim.source_addr.") && m["Kind"] != "source" {
						continue
					}
					if strings.HasPrefix(name, "pim.group_addr.") && m["Kind"] != "group" {
						continue
					}
					if v, ok := m[key]; ok {
						emit(v)
					}
				}
			})
		}
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
			b := displayCustom("pim", "pim."+name, path, DisplayAddress, 0, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
				for _, row := range pimFieldRows(e.Session) {
					// Wireshark uses ip/ipv6 fields for versioned Register IP
					// headers, and pim.source/group only for version-zero dummy
					// headers. Keep our useful Register addresses in their own
					// observation namespace instead of changing that contract.
					if _, register := row["Dummy Header"]; register && routingNumber(row, "IP Version") != 0 {
						continue
					}
					v := row[path]
					if path == "Group" && row["Kind"] == "group" {
						v = row["Address"]
					}
					if path == "Source" && row["Kind"] == "source" {
						v = row["Address"]
					}
					if path != "Group" {
						if a, ok := v.(map[string]any); ok {
							v = a["Address"]
						}
					}
					if s, ok := v.(string); ok && strings.Contains(s, ":") == v6 {
						emit(s)
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
			out = append(out, displayCustom("pim", "pim."+name, path, DisplayAddress, 0, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
				for _, row := range pimFieldRows(e.Session) {
					p.walk(row, []string{path, "*", "Address"}, func(v any) {
						if s, ok := v.(string); ok && strings.Contains(s, ":") == v6 {
							emit(s)
						}
					})
				}
			}))
		}
	}
	out = append(out, displayRoot("pim", "pim.observed.checksum_verified", "Checksum Verified", DisplayBoolean, 0, "context", "Checksum Verified"))
	for _, key := range []string{"Source", "Group"} {
		b := displayRoot("pim", "pim.observed.register."+strings.ToLower(key), key, DisplayAddress, 0, "pdu", "")
		b.path = []string{"Register IP Header", key}
		out = append(out, b)
	}
	return out
}
