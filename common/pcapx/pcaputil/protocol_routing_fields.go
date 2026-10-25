package pcaputil

import "strings"

// Names follow Wireshark's public fields; HSRPv1 seconds and HSRPv2
// milliseconds intentionally remain separate. TLV type gates prevent a
// forwarder priority from appearing as a gateway priority.
func routingDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	add := func(proto, name, key string, typ DisplayFieldType, bits int, version, tlv uint64) {
		b := displayRoot(proto, name, key, typ, bits, "pdu", key)
		b.custom = func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			if version != 0 && routingNumber(e.Session, "Protocol Version") != version {
				return
			}
			if tlv != 0 {
				p.walk(e.Session, []string{"TLVs", "*"}, func(v any) {
					if row, ok := v.(map[string]any); ok && routingNumber(row, "Type") == tlv {
						emit(row[key])
					}
				})
			} else {
				emit(e.Session[key])
			}
		}
		out = append(out, b)
	}
	for _, f := range []struct {
		name, key string
		bits      int
	}{
		{"version", "Version", 8}, {"opcode", "Opcode", 8}, {"state", "State", 8}, {"hellotime", "Hello Time", 8}, {"holdtime", "Hold Time", 8}, {"priority", "Priority", 8}, {"group", "Group", 8},
	} {
		add("hsrp", "hsrp."+f.name, f.key, DisplayUnsigned, f.bits, 1, 0)
		bits := f.bits
		if f.name == "group" {
			bits = 16
		}
		if f.name == "priority" || f.name == "hellotime" || f.name == "holdtime" {
			bits = 32
		}
		add("hsrp", "hsrp2."+f.name, f.key, DisplayUnsigned, bits, 2, 0)
	}
	add("hsrp", "hsrp.virt_ip", "Virtual IP", DisplayAddress, 0, 1, 0)
	add("hsrp", "hsrp2.ipversion", "IP Version", DisplayUnsigned, 8, 2, 0)
	add("hsrp", "hsrp2.identifier", "Identifier", DisplayString, 0, 2, 0)
	for _, key := range []string{"Active Groups", "Passive Groups"} {
		suffix := "active_groups"
		old := "activegrp"
		if key == "Passive Groups" {
			suffix = "passive_groups"
			old = "passivegrp"
		}
		add("hsrp", "hsrp2."+suffix, key, DisplayUnsigned, 16, 2, 2)
		add("hsrp", "hsrp.adv."+old, key, DisplayUnsigned, 16, 1, 0)
	}
	for _, f := range []struct {
		name, key string
		bits      int
	}{{"tlvtype", "Advertisement Type", 16}, {"tlvlength", "Advertisement Length", 16}, {"state", "Advertisement State", 8}} {
		add("hsrp", "hsrp.adv."+f.name, f.key, DisplayUnsigned, f.bits, 1, 0)
	}
	for _, f := range []struct {
		proto, name, key string
		typ              DisplayFieldType
		bits             int
		tlv              uint64
	}{
		{"glbp", "glbp.version", "Version", DisplayUnsigned, 8, 0}, {"glbp", "glbp.group", "Group", DisplayUnsigned, 16, 0}, {"glbp", "glbp.ownerid", "Owner ID", DisplayString, 0, 0},
		{"glbp", "glbp.hello.vgstate", "Gateway State", DisplayUnsigned, 8, 1}, {"glbp", "glbp.hello.priority", "Priority", DisplayUnsigned, 8, 1}, {"glbp", "glbp.hello.helloint", "Hello Interval", DisplayUnsigned, 32, 1}, {"glbp", "glbp.hello.holdint", "Hold Interval", DisplayUnsigned, 32, 1}, {"glbp", "glbp.hello.redirect", "Redirect Interval", DisplayUnsigned, 16, 1}, {"glbp", "glbp.hello.timeout", "Timeout Interval", DisplayUnsigned, 16, 1}, {"glbp", "glbp.hello.addrtype", "Address Type", DisplayUnsigned, 8, 1}, {"glbp", "glbp.hello.addrlen", "Address Length", DisplayUnsigned, 8, 1},
		{"glbp", "glbp.reqresp.forwarder", "Forwarder", DisplayUnsigned, 8, 2}, {"glbp", "glbp.reqresp.vfstate", "Forwarder State", DisplayUnsigned, 8, 2}, {"glbp", "glbp.reqresp.priority", "Priority", DisplayUnsigned, 8, 2}, {"glbp", "glbp.reqresp.weight", "Weight", DisplayUnsigned, 8, 2}, {"glbp", "glbp.reqresp.virtualmac", "Virtual MAC", DisplayString, 0, 2}, {"glbp", "glbp.auth.authtype", "Authentication Type", DisplayUnsigned, 8, 3}, {"glbp", "glbp.auth.authlength", "Authentication Length", DisplayUnsigned, 8, 3},
		{"bfd", "bfd.version", "Version", DisplayUnsigned, 8, 0}, {"bfd", "bfd.diag", "Diagnostic", DisplayUnsigned, 8, 0}, {"bfd", "bfd.sta", "State", DisplayUnsigned, 8, 0}, {"bfd", "bfd.detect_time_multiplier", "Detect Multiplier", DisplayUnsigned, 8, 0}, {"bfd", "bfd.message_length", "Length", DisplayUnsigned, 8, 0}, {"bfd", "bfd.my_discriminator", "My Discriminator", DisplayUnsigned, 32, 0}, {"bfd", "bfd.your_discriminator", "Your Discriminator", DisplayUnsigned, 32, 0}, {"bfd", "bfd.desired_min_tx_interval", "Desired Min TX", DisplayUnsigned, 32, 0}, {"bfd", "bfd.required_min_rx_interval", "Required Min RX", DisplayUnsigned, 32, 0}, {"bfd", "bfd.required_min_echo_interval", "Required Min Echo RX", DisplayUnsigned, 32, 0},
	} {
		add(f.proto, f.name, f.key, f.typ, f.bits, 0, f.tlv)
	}
	for _, f := range []struct{ name, key string }{{"p", "Poll"}, {"f", "Final"}, {"c", "Control Plane Independent"}, {"a", "Authentication Present"}, {"d", "Demand"}, {"m", "Multipoint"}} {
		add("bfd", "bfd.flags."+f.name, f.key, DisplayBoolean, 0, 0, 0)
	}
	for _, key := range []string{"Type", "Length"} {
		name := "glbp.type"
		if key == "Length" {
			name = "glbp.length"
		}
		b := displayCustom("glbp", name, "TLV "+key, DisplayUnsigned, 8, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			p.walk(e.Session, []string{"TLVs", "*", key}, emit)
		})
		out = append(out, b)
	}
	for _, f := range []struct {
		name, key string
		bits      int
	}{{"type", "Authentication Type", 8}, {"len", "Authentication Length", 8}, {"key", "Key ID", 8}, {"seq_num", "Sequence", 32}} {
		b := displayRoot("bfd", "bfd.auth."+f.name, f.key, DisplayUnsigned, f.bits, "pdu", f.key)
		b.path = []string{"Authentication", f.key}
		out = append(out, b)
	}
	for _, f := range []struct {
		name, key string
		bits      int
	}{{"md5_algorithm", "MD5 Algorithm", 8}, {"md5_padding", "MD5 Padding", 8}, {"md5_flags", "MD5 Flags", 16}, {"md5_key_id", "MD5 Key ID", 32}} {
		add("hsrp", "hsrp2."+f.name, f.key, DisplayUnsigned, f.bits, 2, 4)
	}
	for _, proto := range []string{"hsrp", "glbp"} {
		for _, ipv6 := range []bool{false, true} {
			name := "hsrp2.virt_ip"
			if proto == "glbp" {
				name = "glbp.hello.virtualipv4"
			}
			if ipv6 {
				name = "hsrp2.virt_ip_v6"
				if proto == "glbp" {
					name = "glbp.hello.virtualipv6"
				}
			}
			b := displayCustom(proto, name, "Virtual IP", DisplayAddress, 0, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
				visit := func(v any) {
					if s, ok := v.(string); ok && strings.Contains(s, ":") == ipv6 {
						emit(s)
					}
				}
				if proto == "hsrp" {
					if routingNumber(e.Session, "Protocol Version") == 2 {
						visit(e.Session["Virtual IP"])
					}
				} else {
					p.walk(e.Session, []string{"TLVs", "*"}, func(v any) {
						if row, ok := v.(map[string]any); ok && routingNumber(row, "Type") == 1 {
							visit(row["Virtual IP"])
						}
					})
				}
			})
			out = append(out, b)
		}
	}
	return out
}
