package pcaputil

func nativeIGMPDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct {
		name, label string
		typ         DisplayFieldType
		bits        int
	}{
		{"igmp.type", "IGMP membership type", DisplayUnsigned, 8},
		{"igmp.checksum", "IGMP checksum (unverified)", DisplayUnsigned, 16},
		{"igmp.max_resp", "IGMP raw response code", DisplayUnsigned, 8},
		{"igmp.max_resp.decoded_ms", "IGMP response time (ms, version-specific code)", DisplayUnsigned, 32},
		{"igmp.maddr", "IGMP group/multicast address", DisplayAddress, 32},
		{"igmp.query.flags", "IGMPv3 query flags", DisplayUnsigned, 8},
		{"igmp.s", "IGMPv3 suppress router-side processing bit", DisplayBoolean, 0},
		{"igmp.qrv", "IGMPv3 querier robustness variable", DisplayUnsigned, 8},
		{"igmp.query.reserved", "IGMPv3 query extension flags (raw)", DisplayUnsigned, 8},
		{"igmp.qqic", "IGMPv3 raw query interval code", DisplayUnsigned, 8},
		{"igmp.qqic.decoded_seconds", "IGMPv3 query interval (seconds)", DisplayUnsigned, 16},
		{"igmp.num_src", "IGMPv3 source count", DisplayUnsigned, 16},
		{"igmp.saddr", "IGMPv3 source address", DisplayAddress, 32},
		{"igmp.num_grp_recs", "IGMPv3 group record count", DisplayUnsigned, 16},
		{"igmp.record_type", "IGMPv3 group record type", DisplayUnsigned, 8},
		{"igmp.aux_data_len", "IGMPv3 auxiliary data words", DisplayUnsigned, 8},
		{"igmp.aux_data", "IGMPv3 auxiliary bytes (hex, uninterpreted)", DisplayString, 0},
	} {
		spec := spec
		binding := displayCustom("igmp", spec.name, spec.label, spec.typ, spec.bits, "pdu", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
			if n := e.nativeNetworkFields; n != nil && n.frozen {
				for _, f := range n.igmpValues[spec.name] {
					emit(f.value)
				}
			}
		})
		binding.rangeFor = func(e *ProtocolEvent, ordinal int) *DisplayFieldRange {
			n := e.nativeNetworkFields
			if n == nil || !n.frozen || ordinal < 0 || ordinal >= len(n.igmpValues[spec.name]) {
				return nil
			}
			span := n.igmpValues[spec.name][ordinal].span
			return &span
		}
		binding.descriptor.Description = "Private ingress-owned IGMP membership wire observation. Every occurrence owns an exact range. Raw response/interval octets remain distinct from decoded times; neither checksum, IPv4 trust nor membership state is verified."
		out = append(out, binding)
	}
	return out
}
