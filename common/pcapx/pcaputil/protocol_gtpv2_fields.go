package pcaputil

func gtpv2Rows(value any) []map[string]any {
	switch value := value.(type) {
	case map[string]any:
		return []map[string]any{value}
	case []map[string]any:
		return value
	case []any:
		var out []map[string]any
		for _, row := range value {
			if row, ok := row.(map[string]any); ok {
				out = append(out, row)
			}
		}
		return out
	}
	return nil
}

func gtpv2ScopedRows(root map[string]any, scope string) []map[string]any {
	var rows []map[string]any
	if scope == "header" {
		for _, message := range gtpv2Rows(root["Messages"]) {
			rows = append(rows, gtpv2Rows(message["Header"])...)
		}
		return rows
	}
	var visit func(map[string]any)
	visit = func(parent map[string]any) {
		for _, ie := range gtpv2Rows(parent["IEs"]) {
			if scope == "ie" {
				rows = append(rows, ie)
			} else {
				rows = append(rows, gtpv2Rows(ie["Body"])...)
			}
			visit(ie)
		}
	}
	for _, message := range gtpv2Rows(root["Messages"]) {
		visit(message)
	}
	return rows
}

func gtpv2DisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	add := func(name, key, scope string, typ DisplayFieldType, bits int, derived bool) {
		origin := "pdu"
		if derived {
			origin = "derived"
		}
		b := displayCustom("gtpv2", name, key+" (observed GTPv2-C)", typ, bits, origin, func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
			if n := e.nativeNetworkFields; n != nil && n.frozen && n.protocol == "gtpv2" {
				for _, value := range n.gtpv2Values[name] {
					emit(value.value)
				}
			}
		})
		b.collectWireFields = func(e *ProtocolEvent, emit func(any, *DisplayFieldRange)) {
			for _, row := range gtpv2ScopedRows(e.Session, scope) {
				if value, ok := row[key]; ok {
					var span *DisplayFieldRange
					if !derived {
						span = pimOccurrenceRange(row, key, len(e.Raw))
						if span == nil {
							continue
						}
					}
					emit(value, span)
				}
			}
		}
		b.rangeFor = func(e *ProtocolEvent, ordinal int) *DisplayFieldRange {
			n := e.nativeNetworkFields
			if n == nil || !n.frozen || n.protocol != "gtpv2" || ordinal < 0 || ordinal >= len(n.gtpv2Values[name]) {
				return nil
			}
			span := n.gtpv2Values[name][ordinal].span
			if span.Length <= 0 {
				return nil
			}
			return &span
		}
		b.descriptor.Description = "Private observed GTPv2-C occurrence. Literal fields have exact UDP-PDU spans; masked bit values are derived. No inherited TEID/sequence context, subscriber, NAS, verified procedure, bearer/session or peer-authentication claim."
		out = append(out, b)
	}
	for _, s := range []struct {
		name, key, scope string
		bits             int
	}{
		{"gtpv2.flags", "Flags", "header", 8}, {"gtpv2.message_type", "Message Type", "header", 8}, {"gtpv2.message_length", "Message Length", "header", 16},
		{"gtpv2.teid", "TEID", "header", 32}, {"gtpv2.seq", "Sequence Number", "header", 24}, {"gtpv2.priority_or_spare", "Priority or Spare Octet", "header", 8},
		{"gtpv2.ie_type", "IE Type", "ie", 8}, {"gtpv2.ie_len", "IE Length", "ie", 16}, {"gtpv2.ie_spare_and_instance", "Spare and Instance", "ie", 8},
		{"gtpv2.recovery", "Recovery Counter", "body", 8}, {"gtpv2.cause", "Cause", "body", 8}, {"gtpv2.cause.flags", "Cause Flags", "body", 8},
		{"gtpv2.offending_ie.type", "Offending IE Type", "body", 8}, {"gtpv2.offending_ie.length", "Offending IE Length", "body", 16}, {"gtpv2.offending_ie.instance_octet", "Offending IE Instance Octet", "body", 8},
		{"gtpv2.ebi_octet", "Bearer ID Octet", "body", 8}, {"gtpv2.f_teid.flags", "F-TEID Flags", "body", 8}, {"gtpv2.f_teid.teid", "TEID", "body", 32},
	} {
		add(s.name, s.key, s.scope, DisplayUnsigned, s.bits, false)
	}
	for _, s := range []struct {
		name, key, scope string
		bits             int
	}{
		{"gtpv2.version", "Version", "header", 3}, {"gtpv2.msg_priority", "Message Priority", "header", 4}, {"gtpv2.ie_instance", "Instance", "ie", 4},
		{"gtpv2.ie_spare", "Ignored IE Spare", "ie", 4}, {"gtpv2.ebi", "EPS Bearer ID", "body", 4}, {"gtpv2.offending_ie.instance", "Offending IE Instance", "body", 4}, {"gtpv2.f_teid.interface_type", "Interface Type", "body", 6},
	} {
		add(s.name, s.key, s.scope, DisplayUnsigned, s.bits, true)
	}
	for _, s := range []struct{ name, key string }{
		{"gtpv2.p", "Piggyback"}, {"gtpv2.t", "TEID Present"}, {"gtpv2.mp_bit", "MP Bit Observed"},
	} {
		add(s.name, s.key, "header", DisplayBoolean, 0, true)
	}
	add("gtpv2.f_teid.ipv4", "IPv4 Address", "body", DisplayAddress, 0, false)
	add("gtpv2.f_teid.ipv6", "IPv6 Address", "body", DisplayAddress, 0, false)
	return out
}

func (e *ProtocolEvent) retainNativeGTPv2Fields(wire []byte) {
	n := &nativeNetworkObservation{protocol: "gtpv2", values: map[string]nativeNetworkField{}, gtpv2Values: map[string][]nativeNetworkField{}}
	owned := *e
	owned.Raw, owned.Session = wire, e.semanticSession
	for _, binding := range displayFieldBindings {
		if binding.protocols != "|gtpv2|" || binding.collectWireFields == nil {
			continue
		}
		name := binding.descriptor.Name
		binding.collectWireFields(&owned, func(value any, span *DisplayFieldRange) {
			field := nativeNetworkField{value: value}
			if span != nil {
				field.span = *span
			}
			n.gtpv2Values[name] = append(n.gtpv2Values[name], field)
		})
	}
	e.nativeNetworkFields = n
}
