package stream_parser

import (
	"encoding/hex"
	"fmt"
)

// DecodeISISWire reads one bounded ISO10589 PDU with six-octet System IDs.
// It observes independent advertisements, never an elected DIS, authenticated
// adjacency, assembled LSDB, installed route, or a reliable transport session.
// A checksum failure after complete structural decoding can return owned fields
// with an error; receivers must retain the error and cannot certify acceptance.
func DecodeISISWire(w []byte, limit int) (map[string]any, error) {
	if len(w) < 8 || len(w) > 65535 {
		return nil, fmt.Errorf("isis: complete bounded common header required")
	}
	if limit <= 0 {
		return nil, ErrObservedWireBudget
	}
	r := &controlReader{w: w, info: map[string]any{
		"Protocol Name":           "IS-IS",
		"Observation Scope":       "independent link-state advertisement; no inherited fields, elected DIS, authenticated adjacency, assembled LSDB or installed routes",
		"Authentication Verified": false,
		"Checksum Verified":       false,
		"Checksum Observation":    "no checksum observed; integrity not verified",
		"Database Completeness":   "unavailable; LSP fragments and sequence-number reports are independent observations",
	}}
	r.need(r.num("Protocol Discriminator", 1, false) == 0x83, "IS-IS protocol discriminator")
	header := r.num("Length Indicator", 1, false)
	v := r.num("Protocol ID Extension", 1, false)
	idLength := r.num("ID Length", 1, false)
	rawType := r.num("PDU Type Octet", 1, false)
	typ := rawType & 31
	r.info["PDU Type"], r.info["Reserved Type Bits"] = typ, rawType&224
	version := r.num("Version", 1, false)
	r.num("Reserved", 1, false)
	r.num("Maximum Area Addresses", 1, false)
	if r.err != nil {
		return nil, r.err
	}
	if v != 1 || version != 1 || idLength != 0 && idLength != 6 {
		return nil, fmt.Errorf("%w: unselected IS-IS version or System ID width", ErrObservedWireContext)
	}
	lengths := map[uint64]uint64{15: 27, 16: 27, 17: 20, 18: 27, 20: 27, 24: 33, 25: 33, 26: 17, 27: 17}
	names := map[uint64]string{15: "L1 LAN Hello", 16: "L2 LAN Hello", 17: "Point-to-Point Hello", 18: "L1 Link State", 20: "L2 Link State", 24: "L1 Complete Sequence Numbers", 25: "L2 Complete Sequence Numbers", 26: "L1 Partial Sequence Numbers", 27: "L2 Partial Sequence Numbers"}
	want, selected := lengths[typ]
	if !selected || header != want {
		return nil, fmt.Errorf("%w: unselected IS-IS type/fixed header length", ErrObservedWireContext)
	}
	r.need(uint64(len(w)) >= header, "IS-IS fixed header truncated")
	r.info["Packet Name"] = names[typ]
	label := "IS-IS Hello"
	if typ == 18 || typ == 20 {
		label = "IS-IS LSP"
	}
	if typ == 24 || typ == 25 {
		label = "IS-IS CSNP"
	}
	if typ == 26 || typ == 27 {
		label = "IS-IS PSNP"
	}
	r.info["Protocol Name"] = label
	if rawType&224 != 0 || r.info["Reserved"] != uint64(0) {
		r.info["Conformance Warning"] = "ignored reserved header bits retained literally"
	}
	var length uint64
	switch typ {
	case 15, 16, 17:
		circuit := r.num("Circuit Type Octet", 1, false)
		r.info["Circuit Type"], r.info["Reserved Circuit Bits"] = circuit&3, circuit&252
		r.need(circuit&3 != 0, "IS-IS Hello zero circuit type")
		isisID(r, "Source System ID", 6)
		r.num("Holding Timer Seconds", 2, false)
		length = r.num("PDU Length", 2, false)
		if typ == 17 {
			r.num("Local Circuit ID", 1, false)
		} else {
			priority := r.num("Priority Octet", 1, false)
			r.info["Priority"], r.info["Reserved Priority Bit"] = priority&127, priority&128
			isisID(r, "LAN ID", 7)
		}
	case 18, 20:
		length = r.num("PDU Length", 2, false)
		r.num("Remaining Lifetime Seconds", 2, false)
		isisID(r, "LSP ID", 8)
		r.num("Sequence Number", 4, false)
		r.num("Wire Checksum", 2, false)
		flags := r.num("LSP Flags", 1, false)
		r.info["Partition Repair"], r.info["Attached Flags"] = flags&128 != 0, (flags>>3)&15
		r.info["Overload"], r.info["IS Type"] = flags&4 != 0, flags&3
		for i, metric := range []string{"Default", "Delay", "Expense", "Error"} {
			r.info["Attached "+metric+" Metric"] = flags&(8<<i) != 0
		}
	case 24, 25, 26, 27:
		length = r.num("PDU Length", 2, false)
		isisID(r, "Source Node ID", 7)
		if r.err == nil {
			node := r.info["Source Node ID"].(string)
			r.info["Source System ID"], r.info["Source Circuit ID"] = node[:len(node)-3], uint64(r.w[r.at-1])
		}
		if typ == 24 || typ == 25 {
			isisID(r, "Start LSP ID", 8)
			isisID(r, "End LSP ID", 8)
		}
	}
	r.need(length == uint64(len(w)) && r.at == int(header), "IS-IS exact declared PDU/fixed-header boundaries")
	if r.err != nil {
		return nil, r.err
	}
	r.info["Fixed Header Complete"] = true
	isisTLVs(r, typ, &limit)
	decodeError := r.err
	if decodeError != nil {
		// A bounded complete common/fixed header remains observed evidence even
		// when a later TLV is broken. Do not publish the failed TLV's children,
		// infer absent fields or discard earlier valid TLVs. A checksum over
		// this independently complete PDU remains a separate integrity fact;
		// success cannot repair malformed grammar or authenticate its sender.
		r.info["TLV Decode Failure"] = decodeError.Error()
	}
	r.info["Wire Field Ranges"] = isisFieldRanges(r.fields)
	r.info["Wire Grammar Complete"] = decodeError == nil
	var integrityError error
	if typ == 18 || typ == 20 {
		// Remaining lifetime is mutable in transit and outside the Fletcher
		// checksum. Purges do not verify it; retain literal checksum separately.
		lifetime, checksum := r.info["Remaining Lifetime Seconds"].(uint64), r.info["Wire Checksum"].(uint64)
		r.info["Checksum Verified"] = false
		r.info["Checksum Observation"] = "not present or purge; no integrity verification"
		if lifetime != 0 && checksum != 0 {
			valid := isisFletcherValid(w[12:])
			r.info["Checksum Verified"] = valid
			r.info["Checksum Observation"] = "valid active LSP Fletcher checksum; complete bounded PDU, no authentication"
			if !valid {
				r.info["Checksum Observation"] = "invalid active LSP Fletcher checksum; complete bounded PDU"
				integrityError = fmt.Errorf("isis: invalid active LSP Fletcher checksum")
			}
		}
	}
	if r.info["Optional Checksum Failure"] == true {
		integrityError = fmt.Errorf("isis: invalid optional PDU Fletcher checksum")
	}
	r.info["Checksum Failure"] = integrityError != nil
	if decodeError != nil {
		return r.info, decodeError
	}
	return r.info, integrityError
}

func isisID(r *controlReader, key string, length int) {
	w := r.take(key, length)
	if r.err == nil {
		s := hex.EncodeToString(w[:6])
		v := s[:4] + "." + s[4:8] + "." + s[8:]
		if length >= 7 {
			v += fmt.Sprintf(".%02x", w[6])
		}
		if length == 8 {
			v += fmt.Sprintf("-%02x", w[7])
		}
		r.info[key] = v
	}
}

func isisFletcherValid(w []byte) bool {
	var a, b uint32
	for _, c := range w {
		a = (a + uint32(c)) % 255
		b = (b + a) % 255
	}
	return a == 0 && b == 0
}

func isisFieldRanges(fields []tlsCertificateField) map[string]any {
	m := routingFieldRanges(fields)
	for key, source := range map[string]string{"PDU Type": "PDU Type Octet", "Reserved Type Bits": "PDU Type Octet", "Circuit Type": "Circuit Type Octet", "Reserved Circuit Bits": "Circuit Type Octet", "Priority": "Priority Octet", "Reserved Priority Bit": "Priority Octet", "Partition Repair": "LSP Flags", "Attached Flags": "LSP Flags", "Overload": "LSP Flags", "IS Type": "LSP Flags"} {
		if v, ok := m[source]; ok {
			m[key] = v
		}
	}
	for _, name := range []string{"Default", "Delay", "Expense", "Error"} {
		if span, ok := m["LSP Flags"]; ok {
			m["Attached "+name+" Metric"] = span
		}
		if span, ok := m[name+" Metric Octet"]; ok {
			m[name+" Metric"], m[name+" Metric External"] = span, span
			if name != "Default" {
				m[name+" Metric Supported"] = span
			} else {
				m["Up Down"], m["Default Metric Reserved Bit"] = span, span
			}
		}
	}
	if span, ok := m["Control Octet"]; ok {
		for _, key := range []string{"Up Down", "Sub-TLVs Present", "External", "Reserved Reachability Flags"} {
			m[key] = span
		}
		if _, explicit := m["Prefix Length"]; !explicit {
			m["Prefix Length"] = span
		}
	}
	if span, ok := m["Restart Flags"]; ok {
		for _, key := range []string{"Restart Requested", "Restart Acknowledged", "Suppress Adjacency"} {
			m[key] = span
		}
	}
	if span, ok := m["Legacy Neighbor Header Octet"]; ok {
		m["Virtual Flag"], m["Reserved Neighbor Octet"] = span, span
	}
	if span, ok := m["Source Node ID"].([]any); ok && len(span) == 2 {
		start, end := span[0].(uint64), span[1].(uint64)
		m["Source System ID"], m["Source Circuit ID"] = []any{start, end - 1}, []any{end - 1, end}
	}
	return m
}
