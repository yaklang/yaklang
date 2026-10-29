package pcaputil

func rsvpRows(value any) []map[string]any {
	switch v := value.(type) {
	case map[string]any:
		return []map[string]any{v}
	case []map[string]any:
		return v
	case []any:
		var out []map[string]any
		for _, r := range v {
			if row, ok := r.(map[string]any); ok {
				out = append(out, row)
			}
		}
		return out
	}
	return nil
}
func rsvpScopedRows(root map[string]any, scope string) []map[string]any {
	if scope == "root" {
		return []map[string]any{root}
	}
	if scope == "header" {
		return rsvpRows(root["Header"])
	}
	var out []map[string]any
	for _, row := range rsvpRows(root["Objects"]) {
		h, _ := row["Header"].(map[string]any)
		class, _ := h["Class"].(uint64)
		if scope == "object" {
			out = append(out, h)
			continue
		}
		body, _ := row["Body"].(map[string]any)
		if class == 9 || class == 12 || class == 13 {
			out = append(out, rsvpIntservScopedRows(body, scope)...)
			continue
		}
		match := map[string]uint64{"session": 1, "hop": 3, "time": 5, "error": 6, "scope": 7, "style": 8, "filter": 10, "sender": 11, "confirm": 15}[scope]
		if match != class {
			continue
		}
		if scope == "scope" {
			out = append(out, rsvpRows(body["Addresses"])...)
		} else if body != nil {
			out = append(out, body)
		}
	}
	return out
}

func rsvpIntservScopedRows(body map[string]any, scope string) []map[string]any {
	if scope == "intserv-header" {
		return rsvpRows(body["Intserv Header"])
	}
	var out []map[string]any
	for _, service := range rsvpRows(body["Services"]) {
		if scope == "intserv-service" {
			out = append(out, rsvpRows(service["Header"])...)
			continue
		}
		for _, parameter := range rsvpRows(service["Parameters"]) {
			if scope == "intserv-parameter" {
				out = append(out, rsvpRows(parameter["Header"])...)
				continue
			}
			value, _ := parameter["Body"].(map[string]any)
			if scope == "intserv-value" {
				out = append(out, value)
				continue
			}
			key := map[string]string{"intserv-rate": "Token Bucket Rate", "intserv-bucket": "Token Bucket Size", "intserv-peak": "Peak Data Rate", "intserv-bandwidth": "Composed Available Path Bandwidth"}[scope]
			if key != "" {
				out = append(out, rsvpRows(value[key])...)
			}
		}
	}
	return out
}
func rsvpDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	add := func(name, key, scope string, typ DisplayFieldType, bits int, derived bool) {
		origin := "pdu"
		if derived {
			origin = "derived"
		}
		b := displayCustom("rsvp", name, key+" (observed RSVP)", typ, bits, origin, func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
			if n := e.nativeNetworkFields; n != nil && n.frozen && n.protocol == "rsvp" {
				for _, v := range n.rsvpValues[name] {
					emit(v.value)
				}
			}
		})
		b.collectWireFields = func(e *ProtocolEvent, emit func(any, *DisplayFieldRange)) {
			for _, row := range rsvpScopedRows(e.Session, scope) {
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
			if n == nil || !n.frozen || n.protocol != "rsvp" || ordinal < 0 || ordinal >= len(n.rsvpValues[name]) {
				return nil
			}
			span := n.rsvpValues[name][ordinal].span
			if span.Length <= 0 {
				return nil
			}
			return &span
		}
		b.descriptor.Description = "Private RSVP wire occurrence. SESSION selectors and HOP handles are advertisements, not captured UDP/session identity. Literal fields retain their own RSVP-PDU offsets; derived bits/checksum arithmetic have no invented octet range. No reservation, replay or peer-authentication claim."
		out = append(out, b)
	}
	for _, s := range []struct {
		name, key, scope string
		bits             int
	}{
		{"rsvp.version_and_flags", "Version and Flags", "header", 8}, {"rsvp.msg", "Message Type", "header", 8}, {"rsvp.message_checksum", "Checksum", "header", 16}, {"rsvp.sending_ttl", "Send TTL", "header", 8}, {"rsvp.reserved", "Reserved", "header", 8}, {"rsvp.message_length", "Message Length", "header", 16},
		{"rsvp.length", "Length", "object", 16}, {"rsvp.object", "Class", "object", 8}, {"rsvp.ctype", "C-Type", "object", 8},
		{"rsvp.session.proto", "Protocol ID", "session", 8}, {"rsvp.session.flags", "Flags", "session", 8}, {"rsvp.session.port", "Destination Port", "session", 16}, {"rsvp.hop.logical_interface", "Logical Interface Handle", "hop", 32}, {"rsvp.refresh_interval", "Refresh Period Milliseconds", "time", 32},
		{"rsvp.error_flags", "Flags", "error", 8}, {"rsvp.error.error_code", "Error Code", "error", 8}, {"rsvp.error_value", "Error Value", "error", 16},
		{"rsvp.style.flags", "Flags", "style", 8}, {"rsvp.style.style", "Option Vector", "style", 24},
		{"rsvp.filter.unused", "Unused", "filter", 16}, {"rsvp.filter.port", "Source Port", "filter", 16}, {"rsvp.sender.unused", "Unused", "sender", 16}, {"rsvp.sender.port", "Source Port", "sender", 16},
	} {
		add(s.name, s.key, s.scope, DisplayUnsigned, s.bits, false)
	}
	add("rsvp.version", "Version", "header", DisplayUnsigned, 4, true)
	add("rsvp.flags", "Flags", "header", DisplayUnsigned, 4, true)
	for _, s := range []struct{ name, key, scope string }{
		{"rsvp.session.ip", "Destination Address", "session"}, {"rsvp.hop.neighbor_address", "Hop Address", "hop"}, {"rsvp.error.error_node", "Error Node Address", "error"}, {"rsvp.filter.ip", "Source Address", "filter"}, {"rsvp.sender.ip", "Source Address", "sender"}, {"rsvp.confirm.receiver_address", "Receiver Address", "confirm"}, {"rsvp.scope.address", "Address", "scope"},
	} {
		add(s.name, s.key, s.scope, DisplayAddress, 0, false)
	}
	add("rsvp.checksum_omitted", "Checksum Omitted", "root", DisplayBoolean, 0, true)
	add("rsvp.checksum_verified", "Checksum Verified", "root", DisplayBoolean, 0, true)
	for _, s := range []struct {
		name, key, scope string
		bits             int
	}{
		{"rsvp.intserv.version_and_unused", "Version and Unused", "intserv-header", 16}, {"rsvp.intserv.message_length_words", "Message Length Words", "intserv-header", 16},
		{"rsvp.intserv.service", "Service Number", "intserv-service", 8}, {"rsvp.intserv.service_flags", "Flags", "intserv-service", 8}, {"rsvp.intserv.service_length_words", "Data Length Words", "intserv-service", 16},
		{"rsvp.intserv.parameter", "Parameter Number", "intserv-parameter", 8}, {"rsvp.intserv.parameter_flags", "Flags", "intserv-parameter", 8}, {"rsvp.intserv.parameter_length_words", "Data Length Words", "intserv-parameter", 16},
		{"rsvp.intserv.minimum_policed_unit", "Minimum Policed Unit", "intserv-value", 32}, {"rsvp.intserv.maximum_packet_size", "Maximum Packet Size", "intserv-value", 32},
		{"rsvp.intserv.composed_hop_count", "Composed IS Hop Count", "intserv-value", 32}, {"rsvp.intserv.composed_minimum_latency", "Composed Minimum Path Latency", "intserv-value", 32}, {"rsvp.intserv.composed_mtu", "Composed Path MTU", "intserv-value", 32},
	} {
		add(s.name, s.key, s.scope, DisplayUnsigned, s.bits, false)
	}
	add("rsvp.intserv.version", "Version", "intserv-header", DisplayUnsigned, 4, true)
	add("rsvp.intserv.unused", "Unused Bits", "intserv-header", DisplayUnsigned, 12, true)
	add("rsvp.intserv.service_break", "Break Bit Observed", "intserv-service", DisplayBoolean, 0, true)
	add("rsvp.intserv.service_reserved", "Reserved Flag Bits", "intserv-service", DisplayUnsigned, 7, true)
	add("rsvp.intserv.parameter_invalid_observed", "Invalid Bit Observed", "intserv-parameter", DisplayBoolean, 0, true)
	add("rsvp.intserv.parameter_reserved", "Reserved Flag Bits", "intserv-parameter", DisplayUnsigned, 7, true)
	for _, s := range []struct{ name, scope string }{{"token_bucket_rate", "intserv-rate"}, {"token_bucket_size", "intserv-bucket"}, {"peak_data_rate", "intserv-peak"}, {"available_path_bandwidth", "intserv-bandwidth"}} {
		// These private aliases expose exact decimal text and IEEE32 bits
		// separately. They are deliberately not Wireshark float aliases.
		add("rsvp.intserv."+s.name+"_bits", "IEEE754 Bits", s.scope, DisplayUnsigned, 32, false)
		add("rsvp.intserv."+s.name, "Value", s.scope, DisplayString, 0, false)
		add("rsvp.intserv."+s.name+"_classification", "Classification", s.scope, DisplayString, 0, true)
	}
	return out
}
func (e *ProtocolEvent) retainNativeRSVPFields(wire []byte) {
	n := &nativeNetworkObservation{protocol: "rsvp", values: map[string]nativeNetworkField{}, rsvpValues: map[string][]nativeNetworkField{}}
	owned := *e
	owned.Raw, owned.Session = wire, e.semanticSession
	for _, binding := range displayFieldBindings {
		if binding.protocols != "|rsvp|" || binding.collectWireFields == nil {
			continue
		}
		name := binding.descriptor.Name
		binding.collectWireFields(&owned, func(value any, span *DisplayFieldRange) {
			f := nativeNetworkField{value: value}
			if span != nil {
				f.span = *span
			}
			n.rsvpValues[name] = append(n.rsvpValues[name], f)
		})
	}
	e.nativeNetworkFields = n
}
