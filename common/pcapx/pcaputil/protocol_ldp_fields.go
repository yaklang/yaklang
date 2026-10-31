package pcaputil

func ldpSessionDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	add := func(name, key string, typ DisplayFieldType, bits int, tail ...string) {
		b := displayRoot("ldp", name, key, typ, bits, "pdu", "")
		b.path = append([]string{"Messages", "*", "TLVs", "*"}, tail...)
		b.path = append(b.path, key)
		out = append(out, b)
	}
	for _, f := range []struct {
		name, key string
		bits      int
	}{
		{"ldp.msg.tlv.sess.ver", "Session Version", 16}, {"ldp.msg.tlv.sess.ka", "KeepAlive Time", 16}, {"ldp.msg.tlv.sess.pvlim", "Path Vector Limit", 8}, {"ldp.msg.tlv.sess.mxpdu", "Max PDU Length", 16},
		{"ldp.msg.tlv.sess.rxls", "Receiver Label Space ID", 16}, {"ldp.msg.tlv.generic.label", "Generic Label", 32}, {"ldp.msg.tlv.hc.value", "Hop Count", 8}, {"ldp.msg.tlv.lbl_req_msg_id", "Label Request Message ID", 32},
		{"ldp.msg.tlv.status.data", "Status Code", 32}, {"ldp.msg.tlv.status.msg.id", "Referenced Message ID", 32}, {"ldp.msg.tlv.status.msg.type", "Referenced Message Type", 16}, {"ldp.msg.tlv.addrl.addr_family", "Address Family", 16},
	} {
		add(f.name, f.key, DisplayUnsigned, f.bits)
	}
	for _, f := range []struct{ name, key string }{
		{"ldp.msg.tlv.sess.advbit", "On Demand Advertisement"}, {"ldp.msg.tlv.sess.ldetbit", "Loop Detection"}, {"ldp.msg.tlv.status.ebit", "Fatal"}, {"ldp.msg.tlv.status.fbit", "Forward Status"},
	} {
		add(f.name, f.key, DisplayBoolean, 0)
	}
	add("ldp.msg.tlv.sess.rxlsr", "Receiver LSR ID", DisplayAddress, 0)
	add("ldp.msg.tlv.addrl.addr", "Address", DisplayString, 0, "Addresses", "*")
	add("ldp.msg.tlv.pv.lsrid", "LSR ID", DisplayAddress, 0, "Path Vector", "*")
	for _, f := range []struct {
		name, key string
		typ       DisplayFieldType
		bits      int
	}{
		{"ldp.msg.tlv.fec.type", "FEC Type", DisplayUnsigned, 8}, {"ldp.msg.tlv.fec.af", "Address Family", DisplayUnsigned, 16}, {"ldp.msg.tlv.fec.len", "Prefix Length", DisplayUnsigned, 8}, {"ldp.msg.tlv.fec.pfval", "Prefix Address", DisplayString, 0},
	} {
		add(f.name, f.key, f.typ, f.bits, "FEC Elements", "*")
	}
	b := displayRoot("ldp", "ldp.msg.ubit", "Unknown message bit", DisplayBoolean, 0, "pdu", "")
	b.path = []string{"Messages", "*", "Unknown Message"}
	out = append(out, b)
	for _, f := range []struct {
		name, key string
		typ       DisplayFieldType
		bits      int
	}{
		{"ldp.observed.state", "Connection State", DisplayString, 0}, {"ldp.observed.max_pdu_length", "Observed Maximum PDU Length", DisplayUnsigned, 16}, {"ldp.observed.keepalive", "Observed KeepAlive Time", DisplayUnsigned, 16},
	} {
		out = append(out, displayRoot("ldp", f.name, f.key, f.typ, f.bits, "context", f.key))
	}
	return out
}
