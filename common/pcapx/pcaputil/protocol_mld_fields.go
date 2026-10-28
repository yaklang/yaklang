package pcaputil

func nativeMLDDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct {
		name, label string
		typ         DisplayFieldType
		bits        int
	}{
		{"icmpv6.checksum", "Observed ICMPv6 checksum (unverified)", DisplayUnsigned, 16},
		{"icmpv6.mld.maximum_response_delay", "MLDv1 maximum response delay (ms)", DisplayUnsigned, 16},
		{"icmpv6.mld.maximum_response_code.raw", "MLD response code/delay raw word", DisplayUnsigned, 16},
		{"icmpv6.mld.maximum_response_code", "MLDv2 maximum response delay (ms, decoded code)", DisplayUnsigned, 32},
		{"icmpv6.mld.multicast_address", "MLD queried/reported multicast address", DisplayAddress, 128},
		{"icmpv6.mld.flag", "MLDv2 query flags", DisplayUnsigned, 8},
		{"icmpv6.mld.flag.s", "MLDv2 suppress router-side processing bit", DisplayBoolean, 0},
		{"icmpv6.mld.flag.qrv", "MLDv2 querier robustness variable", DisplayUnsigned, 8},
		{"icmpv6.mld.flag.reserved", "MLDv2 query extension flags (raw)", DisplayUnsigned, 8},
		{"icmpv6.mld.qqi.raw", "MLDv2 raw query interval code", DisplayUnsigned, 8},
		{"icmpv6.mld.qqi", "MLDv2 query interval (s, decoded code)", DisplayUnsigned, 16},
		{"icmpv6.mld.nb_sources", "MLDv2 query source count", DisplayUnsigned, 16},
		{"icmpv6.mld.source_address", "MLDv2 queried source address", DisplayAddress, 128},
		{"icmpv6.mldr.nb_mcast_records", "MLDv2 report address record count", DisplayUnsigned, 16},
		{"icmpv6.mldr.mar.record_type", "MLDv2 address record type", DisplayUnsigned, 8},
		{"icmpv6.mldr.mar.aux_data_len", "MLDv2 auxiliary data length (32-bit words)", DisplayUnsigned, 8},
		{"icmpv6.mldr.mar.nb_sources", "MLDv2 address record source count", DisplayUnsigned, 16},
		{"icmpv6.mldr.mar.multicast_address", "MLDv2 reported multicast address", DisplayAddress, 128},
		{"icmpv6.mldr.mar.source_address", "MLDv2 reported source address", DisplayAddress, 128},
		{"icmpv6.mldr.mar.auxiliary_data", "MLDv2 uninterpreted auxiliary bytes (hex)", DisplayString, 0},
	} {
		spec := spec
		binding := displayCustom("icmpv6", spec.name, spec.label, spec.typ, spec.bits, "pdu", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
			n := e.nativeNetworkFields
			if n != nil && n.frozen {
				for _, field := range n.mldValues[spec.name] {
					emit(field.value)
				}
			}
		})
		binding.rangeFor = func(e *ProtocolEvent, ordinal int) *DisplayFieldRange {
			n := e.nativeNetworkFields
			if n == nil || !n.frozen || ordinal < 0 || ordinal >= len(n.mldValues[spec.name]) {
				return nil
			}
			span := n.mldValues[spec.name][ordinal].span
			return &span
		}
		binding.descriptor.Description = "Private ingress-owned MLD wire value with its own exact range, including repeated records. Durations decode RFC9777 floating codes; raw words are separate. Historical Wireshark reserved aliases retain raw extension bits without inferring their meaning. No checksum verification, IP envelope validity or listener state is inferred."
		out = append(out, binding)
	}
	return out
}
