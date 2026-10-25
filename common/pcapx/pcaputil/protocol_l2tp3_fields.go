package pcaputil

import "fmt"

func l2tp3DisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	v2 := displayCustom("l2tp", "l2tp.session", "L2TPv2 session ID", DisplayUnsigned, 16, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
		if routingNumber(e.Session, "Version") != 3 {
			emit(e.Session["Session ID"])
		}
	})
	v2.rangeKey = "Session ID"
	out = append(out, v2)
	for _, f := range []struct {
		name, key string
		bits      int
		nested    bool
	}{
		{"version", "Version", 16, false}, {"sid", "Session ID", 32, false}, {"ccid", "Control Connection ID", 32, false}, {"res", "Reserved", 16, false},
		{"avp.length", "AVP Length", 16, true}, {"avp.vendor_id", "Vendor ID", 32, true}, {"avp.type", "Attribute Type", 16, true},
		{"avp.assigned_control_conn_id", "Assigned Control Connection ID", 32, true}, {"avp.local_session_id", "Local Session ID", 32, true}, {"avp.remote_session_id", "Remote Session ID", 32, true},
		{"avp.pseudowire_type", "Pseudowire Type", 16, true}, {"avp.layer2_specific_sublayer", "Sublayer Type", 16, true}, {"avp.data_sequencing", "Data Sequencing", 16, true},
		{"avp.physical_channel", "Physical Channel ID", 32, true}, {"avp.call_serial_number", "Serial Number", 32, true},
		{"l2_spec_sequence", "Sequence Number", 24, false},
	} {
		f := f
		b := displayCustom("l2tp", "l2tp."+f.name, f.key, DisplayUnsigned, f.bits, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			if routingNumber(e.Session, "Version") != 3 {
				return
			}
			if f.name == "version" {
				if _, ok := e.Session["Flags"]; !ok {
					return
				}
			}
			if f.name == "l2_spec_sequence" && e.Session["Sequence Present"] != true {
				return
			}
			if f.nested {
				for _, m := range pimFieldRows(e.Session) {
					if _, avp := m["Attribute Type"]; avp {
						emit(m[f.key])
					}
				}
			} else {
				emit(e.Session[f.key])
			}
		})
		// The generic emitter cannot attach a different span to each repeated
		// value. Nested AVP spans remain available in the field/byte tree.
		if !f.nested {
			b.rangeKey = f.key
		}
		out = append(out, b)
	}
	for _, f := range []struct {
		name, key string
		nested    bool
	}{
		{"avp.mandatory", "Mandatory", true}, {"avp.hidden", "Hidden", true}, {"avp.circuit_status", "Active", true}, {"avp.circuit_type", "New Circuit", true}, {"l2_spec_s", "Sequence Present", false},
	} {
		f := f
		b := displayCustom("l2tp", "l2tp."+f.name, f.key, DisplayBoolean, 0, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			if routingNumber(e.Session, "Version") != 3 {
				return
			}
			if f.nested {
				for _, m := range pimFieldRows(e.Session) {
					if _, avp := m["Attribute Type"]; avp {
						emit(m[f.key])
					}
				}
			} else {
				emit(e.Session[f.key])
			}
		})
		if !f.nested {
			b.rangeKey = f.key
		}
		out = append(out, b)
	}
	for _, f := range []struct{ name, key string }{{"cookie", "Cookie"}, {"avp.assigned_cookie", "Assigned Cookie"}, {"avp.host_name", "Host Name"}, {"avp.vendor_name", "Vendor Name"}} {
		f := f
		b := displayCustom("l2tp", "l2tp."+f.name, f.key, DisplayString, 0, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			if routingNumber(e.Session, "Version") != 3 {
				return
			}
			for _, m := range pimFieldRows(e.Session) {
				if v, ok := m[f.key]; ok {
					if b, ok := v.([]byte); ok {
						if len(b) > 0 {
							emit(fmt.Sprintf("%x", b))
						}
					} else {
						emit(v)
					}
				}
			}
		})
		if f.key == "Cookie" {
			b.rangeKey = f.key
		}
		out = append(out, b)
	}
	return out
}
