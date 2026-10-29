package pcaputil

// OSPF IDs are advertised values, never trusted adjacency/session keys.
// Repeated LSAs/prefixes use their own source spans, even for equal values.
func ospfv3DisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	add := func(name, key string, typ DisplayFieldType, bits int, scope string) {
		collect := func(e *ProtocolEvent, emit func(any, *DisplayFieldRange)) {
			rows := pimFieldRows(e.Session)
			if scope == "header" {
				header, _ := e.Session["Header"].(map[string]any)
				rows = []map[string]any{header}
			}
			for _, row := range rows {
				if scope == "lsa" {
					if _, ok := row["LS Age"]; !ok {
						continue
					}
				}
				if value, ok := row[key]; ok {
					emit(value, pimOccurrenceRange(row, key, len(e.Raw)))
				}
			}
		}
		b := displayCustom("ospf", name, key+" (observed OSPFv3)", typ, bits, "pdu", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
			if n := e.nativeNetworkFields; n != nil && n.frozen && n.protocol == "ospf" {
				for _, field := range n.ospfValues[name] {
					emit(field.value)
				}
				return
			}
		})
		b.collectWireFields = collect
		b.rangeFor = func(e *ProtocolEvent, ordinal int) *DisplayFieldRange {
			n := e.nativeNetworkFields
			if n == nil || !n.frozen || n.protocol != "ospf" || ordinal < 0 || ordinal >= len(n.ospfValues[name]) {
				return nil
			}
			span := n.ospfValues[name][ordinal].span
			if span.Length <= 0 {
				return nil
			}
			return &span
		}
		b.descriptor.Description = "Private observed OSPFv3 wire occurrence; exact logical-PDU span. No raw-byte search, LSDB, verified neighbor, authenticated sender or installed-route inference."
		out = append(out, b)
	}
	for _, spec := range []struct {
		name, key, scope string
		bits             int
	}{
		{"ospf.version", "Version", "header", 8}, {"ospf.msg", "Type", "header", 8},
		{"ospf.packet_length", "Packet Length", "header", 16}, {"ospf.checksum", "Checksum", "header", 16},
		{"ospfv3.instance_id", "Instance ID", "header", 8}, {"ospfv3.interface_id", "Interface ID", "", 32},
		{"ospfv3.options", "Options", "", 24}, {"ospfv3.router_priority", "Router Priority", "", 8},
		{"ospfv3.hello_interval", "Hello Interval", "", 16}, {"ospfv3.dead_interval", "Router Dead Interval", "", 16},
		{"ospfv3.interface_mtu", "Interface MTU", "", 16}, {"ospfv3.dd_sequence", "DD Sequence", "", 32},
		{"ospfv3.flags", "Flags", "", 8}, {"ospfv3.lsa_count", "LSA Count", "", 32},
		{"ospfv3.lsa.age", "LS Age", "lsa", 16}, {"ospfv3.lsa.type", "LS Type", "", 16},
		{"ospfv3.lsa.sequence", "Sequence", "lsa", 32}, {"ospfv3.lsa.checksum", "Checksum", "lsa", 16},
		{"ospfv3.lsa.length", "Length", "lsa", 16}, {"ospfv3.metric", "Metric", "", 32},
		{"ospfv3.prefix.length", "Prefix Length", "", 8}, {"ospfv3.prefix.options", "Prefix Options", "", 8},
		{"ospfv3.prefix.count", "Prefix Count", "", 32}, {"ospfv3.link.type", "Link Type", "", 8},
		{"ospfv3.link.neighbor_interface_id", "Neighbor Interface ID", "", 32},
		{"ospfv3.reference.ls_type", "Referenced LS Type", "", 16}, {"ospfv3.external.tag", "External Route Tag", "", 32},
	} {
		add(spec.name, spec.key, DisplayUnsigned, spec.bits, spec.scope)
	}
	for _, spec := range []struct{ name, key, scope string }{
		{"ospf.srcrouter", "Router ID", "header"}, {"ospf.area_id", "Area ID", "header"},
		{"ospfv3.designated_router", "Designated Router", ""}, {"ospfv3.backup_designated_router", "Backup Designated Router", ""},
		{"ospfv3.neighbor", "Neighbor Router ID", ""}, {"ospfv3.lsa.id", "Link State ID", ""},
		{"ospfv3.lsa.advertising_router", "Advertising Router", ""}, {"ospfv3.attached_router", "Attached Router", ""},
		{"ospfv3.destination_router", "Destination Router", ""}, {"ospfv3.link_local", "Link Local Address", ""},
		{"ospfv3.forwarding_address", "Forwarding Address", ""}, {"ospfv3.reference.id", "Referenced Link State ID", ""},
		{"ospfv3.reference.advertising_router", "Referenced Advertising Router", ""},
	} {
		add(spec.name, spec.key, DisplayAddress, 0, spec.scope)
	}
	add("ospfv3.prefix", "Prefix", DisplayString, 0, "")
	return out
}

func (e *ProtocolEvent) retainNativeOSPFv3Fields(wire []byte) {
	n := &nativeNetworkObservation{protocol: "ospf", values: map[string]nativeNetworkField{}, ospfValues: map[string][]nativeNetworkField{}}
	owned := *e
	owned.Raw, owned.Session = wire, e.semanticSession
	for _, binding := range displayFieldBindings {
		if binding.protocols != "|ospf|" || binding.collectWireFields == nil {
			continue
		}
		name := binding.descriptor.Name
		binding.collectWireFields(&owned, func(value any, span *DisplayFieldRange) {
			field := nativeNetworkField{value: value}
			if span != nil {
				field.span = *span
			}
			n.ospfValues[name] = append(n.ospfValues[name], field)
		})
	}
	e.nativeNetworkFields = n
}
