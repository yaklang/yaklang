package pcaputil

func ospfv2Rows(value any) []map[string]any {
	switch v := value.(type) {
	case map[string]any:
		return []map[string]any{v}
	case []map[string]any:
		return v
	case []any:
		var rows []map[string]any
		for _, item := range v {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
		return rows
	}
	return nil
}

func ospfv2DisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	add := func(name, key, scope string, typ DisplayFieldType, bits int, derived bool) {
		collect := func(e *ProtocolEvent, emit func(any, *DisplayFieldRange)) {
			rows := pimFieldRows(e.Session)
			switch scope {
			case "header":
				rows = ospfv2Rows(e.Session["Header"])
			case "auth":
				rows = ospfv2Rows(e.Session["Authentication"])
			case "root":
				rows = []map[string]any{e.Session}
			case "requests":
				rows = ospfv2Rows(e.Session["Requests"])
			case "lls":
				rows = ospfv2Rows(e.Session["LLS"])
			case "lls-tlv":
				lls, _ := e.Session["LLS"].(map[string]any)
				rows = ospfv2Rows(lls["TLVs"])
			}
			for _, row := range rows {
				if scope == "lsa" {
					if _, present := row["LS Age"]; !present {
						continue
					}
				}
				if value, present := row[key]; present {
					var span *DisplayFieldRange
					if !derived {
						span = pimOccurrenceRange(row, key, len(e.Raw))
						if span == nil {
							continue // Derived selectors are separate from literal bytes.
						}
					}
					emit(value, span)
				}
			}
		}
		origin := "pdu"
		if derived {
			origin = "derived"
		}
		b := displayCustom("ospf", name, key+" (observed OSPFv2)", typ, bits, origin, func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
			if n := e.nativeNetworkFields; n != nil && n.frozen && n.protocol == "ospf" {
				for _, field := range n.ospfValues[name] {
					emit(field.value)
				}
			}
		})
		b.collectWireFields = collect
		b.rangeFor = func(e *ProtocolEvent, ordinal int) *DisplayFieldRange {
			n := e.nativeNetworkFields
			if n == nil || !n.frozen || ordinal < 0 || ordinal >= len(n.ospfValues[name]) {
				return nil
			}
			span := n.ospfValues[name][ordinal].span
			if span.Length <= 0 {
				return nil
			}
			return &span
		}
		b.descriptor.Description = "Private observed OSPFv2 occurrence; exact PDU spans only for literal fields. Selectors, authentication metadata and advertisements do not certify topology, SA, replay protection, adjacency, LSDB or installed routes."
		out = append(out, b)
	}
	for _, s := range []struct {
		name, key, scope string
		bits             int
	}{
		{"ospfv2.instance_id", "Instance ID", "header", 8}, {"ospfv2.auth.type", "Authentication Type", "header", 8},
		{"ospfv2.auth.key_id", "Key ID", "auth", 8}, {"ospfv2.auth.data_length", "Authentication Data Length", "auth", 8},
		{"ospfv2.auth.sequence", "Sequence", "auth", 32},
		{"ospfv2.options", "Options", "all", 8}, {"ospfv2.router_priority", "Router Priority", "all", 8},
		{"ospfv2.hello_interval", "Hello Interval", "all", 16}, {"ospfv2.dead_interval", "Router Dead Interval", "all", 32},
		{"ospfv2.interface_mtu", "Interface MTU", "all", 16}, {"ospfv2.dd_sequence", "DD Sequence", "all", 32},
		{"ospfv2.flags", "Flags", "all", 8}, {"ospfv2.lsa_count", "LSA Count", "root", 32},
		{"ospfv2.lsa.age", "LS Age", "lsa", 16}, {"ospfv2.lsa.type", "LS Type", "lsa", 8},
		{"ospfv2.lsa.sequence", "Sequence", "lsa", 32}, {"ospfv2.lsa.checksum", "Checksum", "lsa", 16},
		{"ospfv2.lsa.length", "Length", "lsa", 16}, {"ospfv2.request.ls_type", "LS Type", "requests", 32},
		{"ospfv2.link_count", "Link Count", "all", 16}, {"ospfv2.link.type", "Link Type", "all", 8},
		{"ospfv2.link.selector_count", "TOS/MT-ID Count", "all", 8}, {"ospfv2.selector", "TOS/MT-ID Selector", "all", 8},
		{"ospfv2.metric", "Metric", "all", 32}, {"ospfv2.external.flags", "Flags and TOS/MT-ID", "all", 8},
		{"ospfv2.external.tag", "External Route Tag", "all", 32},
		{"ospfv2.lls.checksum", "Checksum", "lls", 16}, {"ospfv2.lls.length_words", "Length Words", "lls", 16},
		{"ospfv2.lls.tlv.type", "TLV Type", "lls-tlv", 16}, {"ospfv2.lls.tlv.length", "Length", "lls-tlv", 16},
		{"ospfv2.lls.extended_options", "Extended Options and Flags", "lls-tlv", 32}, {"ospfv2.lls.auth.sequence", "Sequence", "lls-tlv", 32},
	} {
		add(s.name, s.key, s.scope, DisplayUnsigned, s.bits, false)
	}
	for _, s := range []struct{ name, key, scope string }{
		{"ospfv2.network_mask", "Network Mask", "all"}, {"ospfv2.designated_router", "Designated Router", "all"},
		{"ospfv2.backup_designated_router", "Backup Designated Router", "all"}, {"ospfv2.neighbor", "Neighbor Router ID", "all"},
		{"ospfv2.lsa.id", "Link State ID", "lsa"}, {"ospfv2.lsa.advertising_router", "Advertising Router", "lsa"},
		{"ospfv2.request.id", "Link State ID", "requests"}, {"ospfv2.request.advertising_router", "Advertising Router", "requests"},
		{"ospfv2.link.id", "Link ID", "all"}, {"ospfv2.link.data", "Link Data", "all"},
		{"ospfv2.attached_router", "Attached Router", "all"}, {"ospfv2.forwarding_address", "Forwarding Address", "all"},
	} {
		add(s.name, s.key, s.scope, DisplayAddress, 0, false)
	}
	add("ospfv2.checksum_verified", "Checksum Verified", "root", DisplayBoolean, 0, true)
	add("ospfv2.lsa.checksum_verified", "Checksum Verified", "lsa", DisplayBoolean, 0, true)
	add("ospfv2.lls.checksum_verified", "Checksum Verified", "lls", DisplayBoolean, 0, true)
	add("ospfv2.lls.auth.sequence_matches_header", "Sequence Matches OSPF Header", "lls-tlv", DisplayBoolean, 0, true)
	add("ospfv2.lls.auth.length_matches_header", "Authentication Data Length Matches Header", "lls-tlv", DisplayBoolean, 0, true)
	add("ospfv2.external.metric_type", "External Metric Type", "all", DisplayUnsigned, 8, true)
	return out
}

func (e *ProtocolEvent) retainNativeOSPFv2Fields(wire []byte) {
	e.retainNativeOSPFFields(wire, 2)
}
