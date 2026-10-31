package pcaputil

import "strings"

const (
	isisLegacyNeighborTLVs       = 1002 // shared type 2/3 metric fields, kept in wire order
	isisLegacyIPReachabilityTLVs = 1128 // shared type 128/130 metric fields
)

// Every selector stays inside its PDU family and TLV namespace. Identical leaf
// names in SNP reports, link advertisements and prefix sub-TLVs are not peers.
func isisDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	type fieldSpec struct {
		name, key string
		typ       DisplayFieldType
		bits      int
	}
	add := func(group string, tlv int, path []string, spec fieldSpec, invert bool) {
		b := displayCustom("isis", "isis."+spec.name, spec.key, spec.typ, spec.bits, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			typ := routingNumber(e.Session, "PDU Type")
			family := "hello"
			if typ == 18 || typ == 20 {
				family = "lsp"
			}
			if typ == 24 || typ == 25 {
				family = "csnp"
			}
			if typ == 26 || typ == 27 {
				family = "psnp"
			}
			if group != "" && !strings.Contains("|"+group+"|", "|"+family+"|") {
				return
			}
			rowValue := func(row map[string]any) {
				v, exists := row[spec.key]
				if !exists {
					return
				}
				if spec.name == "type.reserved" {
					if n, ok := displayUint(v); ok {
						v = n >> 5
					}
				}
				if spec.name == "hello.reserved" {
					if n, ok := displayUint(v); ok {
						v = n >> 2
					}
				}
				if invert {
					if x, ok := v.(bool); ok {
						v = !x
					}
				}
				before := len(p.snapshot.Values)
				emit(v)
				if len(p.snapshot.Values) == before+1 {
					view := ProtocolEvent{Session: row, Raw: e.Raw}
					p.snapshot.Values[before].Range = view.displayPDUFieldRange(spec.key)
				}
			}
			project := func(row map[string]any) {
				if len(path) == 0 {
					rowValue(row)
					return
				}
				p.walk(row, path, func(v any) {
					if child, ok := v.(map[string]any); ok {
						rowValue(child)
					}
				})
			}
			if tlv < 0 {
				project(e.Session)
				if spec.name == "hello.reserved" {
					if n, ok := displayUint(e.Session["Reserved Priority Bit"]); ok {
						before := len(p.snapshot.Values)
						emit(n >> 7)
						if len(p.snapshot.Values) == before+1 {
							p.snapshot.Values[before].Range = e.displayPDUFieldRange("Reserved Priority Bit")
						}
					}
				}
				return
			}
			p.maps(e.Session["TLVs"], func(row map[string]any) {
				t := routingNumber(row, "Type")
				if tlv == 0 || t == uint64(tlv) || tlv == isisLegacyNeighborTLVs && (t == 2 || t == 3) || tlv == isisLegacyIPReachabilityTLVs && (t == 128 || t == 130) {
					if tlv == isisLegacyNeighborTLVs && t == 3 {
						if shared, ok := row["Shared Metrics"].(map[string]any); ok {
							rowValue(shared)
						}
					} else {
						project(row)
					}
				}
			})
		})
		if invert {
			b.descriptor.Label = strings.TrimSuffix(spec.key, " Supported") + " Unsupported Bit"
			b.descriptor.Description = "Literal unsupported bit (true means unsupported), not a negotiated capability"
		}
		out = append(out, b)
	}
	u := func(group string, tlv int, path []string, name, key string, bits int) {
		add(group, tlv, path, fieldSpec{name, key, DisplayUnsigned, bits}, false)
	}
	s := func(group string, tlv int, path []string, name, key string) {
		add(group, tlv, path, fieldSpec{name, key, DisplayString, 0}, false)
	}
	flag := func(group string, tlv int, path []string, name, key string, invert bool) {
		add(group, tlv, path, fieldSpec{name, key, DisplayBoolean, 0}, invert)
	}
	list := func(key string) []string { return []string{key, "*"} }
	for _, f := range []struct{ name, key string }{
		{"irpd", "Protocol Discriminator"}, {"len", "Length Indicator"}, {"version", "Protocol ID Extension"}, {"sysid_len", "ID Length"},
		{"type", "PDU Type"}, {"type.reserved", "Reserved Type Bits"}, {"version2", "Version"}, {"reserved", "Reserved"}, {"max_area_adr", "Maximum Area Addresses"},
	} {
		u("", -1, nil, f.name, f.key, 8)
	}
	for _, g := range []string{"hello", "lsp", "csnp", "psnp"} {
		u(g, -1, nil, g+".pdu_length", "PDU Length", 16)
		u(g, 0, nil, g+".clv.type", "Type", 8)
		u(g, 0, nil, g+".clv.length", "Length", 8)
	}
	for _, g := range []string{"hello", "lsp"} {
		s(g, 1, list("Area Addresses"), g+".area_address", "Length-prefixed Address Octets")
		s(g, 1, list("Area Addresses"), g+".observed.area_address", "Address")
		u(g, 129, list("Protocols Supported"), g+".clv_nlpid.nlpid", "NLPID", 8)
		for _, f := range []struct {
			tlv, bits int
			name      string
		}{{132, 32, "clv_ipv4_int_addr"}, {232, 128, "clv_ipv6_int_addr"}} {
			add(g, f.tlv, list("Interface Addresses"), fieldSpec{g + "." + f.name, "Address", DisplayAddress, f.bits}, false)
		}
	}
	s("hello", -1, nil, "hello.source_id", "Source System ID")
	s("hello", -1, nil, "hello.lan_id", "LAN ID")
	u("hello", -1, nil, "hello.circuit_type", "Circuit Type", 2)
	u("hello", -1, nil, "hello.reserved", "Reserved Circuit Bits", 8)
	u("hello", -1, nil, "hello.holding_timer", "Holding Timer Seconds", 16)
	u("hello", -1, nil, "hello.local_circuit_id", "Local Circuit ID", 8)
	u("hello", -1, nil, "hello.priority", "Priority", 7)
	s("hello", 6, list("IS Neighbors"), "hello.is_neighbor", "SNPA")
	u("hello", 211, nil, "hello.clv_restart_flags", "Restart Flags", 8)
	for _, f := range []struct{ name, key string }{{"rr", "Restart Requested"}, {"ra", "Restart Acknowledged"}, {"sa", "Suppress Adjacency"}} {
		flag("hello", 211, nil, "hello.clv_restart_flags."+f.name, f.key, false)
	}
	u("hello", 240, nil, "hello.adjacency_state", "Adjacency State", 8)
	// These names explicitly describe observations rather than actual adjacency.
	u("hello", 240, nil, "hello.observed.local_extended_circuit", "Extended Local Circuit ID", 32)
	u("hello", 240, nil, "hello.observed.neighbor_extended_circuit", "Neighbor Extended Local Circuit ID", 32)
	s("hello", 240, nil, "hello.observed.neighbor_system", "Neighbor System ID")
	s("lsp", -1, nil, "lsp.lsp_id", "LSP ID")
	u("lsp", -1, nil, "lsp.remaining_life", "Remaining Lifetime Seconds", 16)
	u("lsp", -1, nil, "lsp.sequence_number", "Sequence Number", 32)
	u("lsp", -1, nil, "lsp.wire_checksum", "Wire Checksum", 16)
	flag("lsp", -1, nil, "lsp.partition_repair", "Partition Repair", false)
	flag("lsp", -1, nil, "lsp.overload", "Overload", false)
	u("lsp", -1, nil, "lsp.att", "Attached Flags", 4)
	for _, metric := range []string{"Default", "Delay", "Expense", "Error"} {
		flag("lsp", -1, nil, "lsp."+strings.ToLower(metric)+"_metric", "Attached "+metric+" Metric", false)
	}
	u("lsp", -1, nil, "lsp.is_type", "IS Type", 2)
	s("lsp", 137, nil, "lsp.hostname", "Hostname")
	flag("lsp", 2, nil, "lsp.is_virtual", "Virtual Flag", false)
	u("lsp", 2, nil, "lsp.eis_neighbors_clv_inner.reserved", "Reserved Neighbor Octet", 8)
	s("lsp", 2, list("Neighbors"), "lsp.eis_neighbors.is_neighbor", "Neighbor Node ID")
	s("lsp", 3, list("Neighbors"), "lsp.eis_neighbors.es_neighbor_id", "System ID")
	u("lsp", 3, nil, "lsp.es_neighbor_count", "ES Neighbor Count", 16)
	out[len(out)-1].descriptor.Origin = "derived"
	for _, tlv := range []int{isisLegacyNeighborTLVs, isisLegacyIPReachabilityTLVs} {
		prefix, path := "lsp.eis_neighbors.", list("Neighbors")
		if tlv == isisLegacyIPReachabilityTLVs {
			prefix, path = "lsp.ip_reachability.", list("Reachability Entries")
		}
		for _, metric := range []string{"Default", "Delay", "Expense", "Error"} {
			name := strings.ToLower(metric) + "_metric"
			u("lsp", tlv, path, prefix+name, metric+" Metric", 6)
			flag("lsp", tlv, path, prefix+name+"_ie", metric+" Metric External", false)
			if metric != "Default" {
				bitName := prefix + name + "_supported"
				if prefix == "lsp.ip_reachability." {
					bitName = prefix + name + "_support"
				}
				if prefix == "lsp.eis_neighbors." && metric == "Delay" {
					bitName = "lsp.eis_neighbors_delay_metric.supported"
				}
				flag("lsp", tlv, path, bitName, metric+" Metric Supported", true)
			}
		}
		if prefix == "lsp.ip_reachability." {
			flag("lsp", tlv, path, prefix+"distribution", "Up Down", false)
		}
	}
	add("lsp", isisLegacyIPReachabilityTLVs, list("Reachability Entries"), fieldSpec{"lsp.ip_reachability.ipv4_prefix", "IPv4 Prefix", DisplayAddress, 32}, false)
	add("lsp", isisLegacyIPReachabilityTLVs, list("Reachability Entries"), fieldSpec{"lsp.ip_reachability.observed.ipv4_mask", "IPv4 Mask", DisplayAddress, 32}, false)
	s("lsp", 22, list("Neighbors"), "lsp.ext_is_reachability.is_neighbor_id", "Neighbor Node ID")
	u("lsp", 22, list("Neighbors"), "lsp.ext_is_reachability.metric", "Metric", 24)
	u("lsp", 22, list("Neighbors"), "lsp.ext_is_reachability.subclvs_length", "Sub-TLV Length", 8)
	for _, f := range []struct {
		tlv, bits    int
		prefix, addr string
	}{{135, 32, "lsp.ext_ip_reachability.", "ipv4_prefix"}, {236, 128, "lsp.ipv6_reachability.", "ipv6_prefix"}} {
		p := list("Reachability Entries")
		u("lsp", f.tlv, p, f.prefix+"metric", "Metric", 32)
		u("lsp", f.tlv, p, f.prefix+"prefix_length", "Prefix Length", 8)
		flag("lsp", f.tlv, p, f.prefix+"distribution", "Up Down", false)
		flag("lsp", f.tlv, p, f.prefix+"subtlv", "Sub-TLVs Present", false)
		// A variable-width prefix expanded with zero octets is derived. Never
		// claim that a full 4/16-octet address exists in the captured PDU.
		add("lsp", f.tlv, p, fieldSpec{f.prefix + f.addr, "Canonical Prefix Address", DisplayAddress, f.bits}, false)
		out[len(out)-1].descriptor.Origin = "derived"
		if f.tlv == 236 {
			flag("lsp", f.tlv, p, f.prefix+"distribution_internal", "External", false)
			u("lsp", f.tlv, p, f.prefix+"reserved_bits", "Reserved Reachability Flags", 5)
		} else {
			u("lsp", f.tlv, p, f.prefix+"subclvs_length", "Sub-TLV Length", 8)
		}
	}
	for _, g := range []string{"csnp", "psnp"} {
		s(g, -1, nil, g+".source_id", "Source System ID")
		u(g, -1, nil, g+".source_circuit", "Source Circuit ID", 8)
	}
	s("csnp", -1, nil, "csnp.start_lsp_id", "Start LSP ID")
	s("csnp", -1, nil, "csnp.end_lsp_id", "End LSP ID")
	// Wireshark shares csnp.* entry names with PSNP reports.
	s("csnp|psnp", 9, list("LSP Entries"), "csnp.lsp_id", "LSP ID")
	u("csnp|psnp", 9, list("LSP Entries"), "csnp.lsp_seq_num", "Sequence Number", 32)
	u("csnp|psnp", 9, list("LSP Entries"), "csnp.lsp_remain_life", "Remaining Lifetime Seconds", 16)
	u("csnp|psnp", 9, list("LSP Entries"), "csnp.lsp_checksum", "Reported Checksum", 16)
	b := displayCustom("isis", "isis.lsp.checksum", "LSP interpreted checksum (purge has no verified checksum)", DisplayUnsigned, 16, "derived", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
		if n, ok := displayUint(e.Session["Wire Checksum"]); ok {
			if routingNumber(e.Session, "Remaining Lifetime Seconds") == 0 {
				n = 0
			}
			emit(n)
		}
	})
	out = append(out, b)
	return out
}
