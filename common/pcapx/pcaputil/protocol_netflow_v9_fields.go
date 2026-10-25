package pcaputil

func netflowV9DisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct{ name, key string }{
		{"cflow.version", "Version"}, {"cflow.count", "Header Record Count"},
		{"cflow.observed.sysuptime_ms", "System Uptime Milliseconds"}, {"cflow.unix_secs", "Unix Seconds"},
		{"cflow.sequence", "Sequence Number"}, {"cflow.source_id", "Source ID"},
	} {
		bits := 32
		if spec.name == "cflow.version" || spec.name == "cflow.count" {
			bits = 16
		}
		b := displayRoot("netflow-v9", spec.name, spec.key, DisplayUnsigned, bits, "pdu", spec.key)
		if spec.name != "cflow.source_id" {
			b.protocols = "|netflow-v5|netflow-v9|"
		}
		out = append(out, b)
	}
	for _, spec := range []struct {
		name, key string
		bits      int
	}{
		{"cflow.unix_nsecs", "Unix Nanoseconds", 32}, {"cflow.engine_type", "Engine Type", 8}, {"cflow.engine_id", "Engine ID", 8},
		{"cflow.samplingmode", "Sampling Mode", 2}, {"cflow.samplerate", "Sampling Interval", 14},
	} {
		out = append(out, displayRoot("netflow-v5", spec.name, spec.key, DisplayUnsigned, spec.bits, "pdu", spec.key))
	}
	add := func(name, key string, bits int, level string) {
		b := displayCustom("netflow-v9", name, key, DisplayUnsigned, bits, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			path := []string{"FlowSets", "*"}
			if level != "set" {
				path = append(path, "Templates", "*")
			}
			if level == "specifier" || level == "scope-specifier" {
				path = append(path, "Fields", "*")
			}
			p.walk(e.Session, path, func(v any) {
				row, ok := v.(map[string]any)
				if !ok {
					return
				}
				if key == "Field Type" && (level == "specifier" || level == "scope-specifier") && (row["Scope"] == true) != (level == "scope-specifier") {
					return
				}
				before := len(p.snapshot.Values)
				emit(row[key])
				if len(p.snapshot.Values) == before+1 {
					p.snapshot.Values[before].Range = (&ProtocolEvent{Session: row, Raw: e.Raw}).displayPDUFieldRange(key)
				}
			})
		})
		out = append(out, b)
	}
	add("cflow.flowset_id", "FlowSet ID", 16, "set")
	add("cflow.flowset_length", "FlowSet Length", 16, "set")
	add("cflow.template_id", "Template ID", 16, "template")
	add("cflow.template_field_count", "Field Count", 16, "template")
	add("cflow.template_field_type", "Field Type", 16, "specifier")
	add("cflow.template_field_length", "Field Length", 16, "specifier")
	add("cflow.scope_field_type", "Field Type", 16, "scope-specifier")
	add("cflow.option_scope_length", "Scope Length", 16, "template")
	add("cflow.option_length", "Options Length", 16, "template")
	for _, spec := range []struct {
		id         uint64
		name, kind string
		bits       int
	}{
		{1, "cflow.octets", "unsigned", 64}, {2, "cflow.packets", "unsigned", 64},
		{4, "cflow.protocol", "unsigned", 8}, {5, "cflow.tos", "unsigned", 8}, {6, "cflow.tcpflags", "unsigned", 8},
		{7, "cflow.srcport", "unsigned", 16}, {8, "cflow.srcaddr", "IPv4", 32},
		{9, "cflow.srcmask", "unsigned", 8}, {10, "cflow.inputint", "unsigned", 32},
		{11, "cflow.dstport", "unsigned", 16}, {12, "cflow.dstaddr", "IPv4", 32},
		{13, "cflow.dstmask", "unsigned", 8}, {14, "cflow.outputint", "unsigned", 32},
		{15, "cflow.nexthop", "IPv4", 32}, {16, "cflow.srcas", "unsigned", 32}, {17, "cflow.dstas", "unsigned", 32},
		{18, "cflow.bgpnexthop", "IPv4", 32}, {19, "cflow.mulpackets", "unsigned", 64}, {20, "cflow.muloctets", "unsigned", 64},
		{21, "cflow.observed.last_uptime_ms", "unsigned", 32}, {22, "cflow.observed.first_uptime_ms", "unsigned", 32},
		{23, "cflow.post_octets", "unsigned", 64}, {24, "cflow.post_packets", "unsigned", 64},
		{27, "cflow.srcaddrv6", "IPv6", 128}, {28, "cflow.dstaddrv6", "IPv6", 128},
		{32, "cflow.icmp_type_code_ipv4", "unsigned", 16},
		{61, "cflow.direction", "unsigned", 8}, {148, "cflow.flow_id", "unsigned", 64}, {233, "cflow.firewall_event", "unsigned", 8},
	} {
		spec := spec
		typ := DisplayUnsigned
		if spec.kind == "IPv4" || spec.kind == "IPv6" {
			typ = DisplayAddress
		}
		b := displayCustom("netflow-v9", spec.name, "Reported flow field; not a captured endpoint or transport fact", typ, spec.bits, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			if p.netflowV9ByType == nil {
				p.netflowV9ByType = map[uint64][]map[string]any{}
				path := []string{"FlowSets", "*", "Records", "*", "Fields", "*"}
				if e.Protocol == "netflow-v5" {
					path = []string{"Records", "*", "Fields", "*"}
				}
				p.walk(e.Session, path, func(v any) {
					if field, ok := v.(map[string]any); ok {
						id := routingNumber(field, "Field Type")
						p.netflowV9ByType[id] = append(p.netflowV9ByType[id], field)
					}
				})
			}
			for _, field := range p.netflowV9ByType[spec.id] {
				if field["Scope"] != false || field["Value Kind"] != spec.kind {
					continue
				}
				before := len(p.snapshot.Values)
				emit(field["Value"])
				if len(p.snapshot.Values) == before+1 {
					p.snapshot.Values[before].Range = (&ProtocolEvent{Session: field, Raw: e.Raw}).displayPDUFieldRange("Value")
				}
			}
		})
		b.protocols = "|netflow-v5|netflow-v9|"
		out = append(out, b)
	}
	return out
}
