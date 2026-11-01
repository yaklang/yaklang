package stream_parser

import (
	"bytes"
	"fmt"
	"net"
)

// DecodeSTPWire reads one complete, explicitly bounded BPDU, after LLC/SNAP.
// pvst must come from the observed Cisco SNAP OUI/PID, not a destination MAC
// or a previous packet. Advertised roots/roles never prove topology or peer
// authentication. Timers retain their exact 1/256-second wire values.
func DecodeSTPWire(w []byte, pvst bool, budget int) (map[string]any, error) {
	if len(w) < 4 || len(w) > 1500 {
		return nil, fmt.Errorf("stp: bounded complete BPDU header required")
	}
	if budget <= 0 {
		return nil, ErrObservedWireBudget
	}
	r := &controlReader{w: w, info: map[string]any{
		"Observation Scope":       "independent BPDU advertisement; no elected root, operational port, authenticated bridge or inherited message fields",
		"Authentication Verified": false,
		"PVST Encapsulation":      pvst,
	}}
	r.need(r.num("Protocol ID", 2, false) == 0, "STP protocol identifier")
	v := r.num("Version", 1, false)
	typ := r.num("BPDU Type", 1, false)
	if r.err != nil {
		return nil, r.err
	}
	if typ != 0 && typ != 128 && v != 0 && v != 2 && v != 3 {
		return nil, fmt.Errorf("%w: unselected STP version %d", ErrObservedWireContext, v)
	}
	if typ == 128 {
		r.need(len(w) == 4, "TCN requires exact four-byte message")
		r.info["Packet Name"], r.info["Protocol Name"] = "Topology Change Notification", "STP"
		if v != 0 {
			r.info["Conformance Warning"] = "TCN layout is selected by Type128; literal wire version is not evidence of a rapid/multiple BPDU"
		}
		if pvst {
			r.info["Protocol Name"] = "PVST+"
		}
		return stpFinish(r)
	}
	r.need(typ == 0 || ((v == 2 || v == 3) && typ == 2), "unselected STP version/type combination")
	if r.err != nil {
		return nil, r.err
	}
	if v == 3 && typ == 2 && pvst {
		return nil, fmt.Errorf("%w: MST over PVST SNAP is unselected", ErrObservedWireContext)
	}
	flags := r.num("Flags", 1, false)
	r.info["Topology Change"], r.info["Topology Change Acknowledgement"] = flags&1 != 0, flags&128 != 0
	if typ == 2 {
		r.info["Proposal"], r.info["Port Role"] = flags&2 != 0, (flags>>2)&3
		r.info["Learning"], r.info["Forwarding"], r.info["Agreement"] = flags&16 != 0, flags&32 != 0, flags&64 != 0
		r.info["Port Role Name"] = []string{"Unknown", "Alternate/Backup", "Root", "Designated"}[(flags>>2)&3]
	}
	stpBridge(r, "Root")
	r.num("Root Path Cost", 4, false)
	stpBridge(r, "Bridge")
	port := r.num("Port ID", 2, false)
	r.info["Port Priority"], r.info["Port Number"] = port&0xf000, port&0x0fff
	for _, key := range []string{"Message Age", "Max Age", "Hello Time", "Forward Delay"} {
		ticks := r.num(key+" Ticks", 2, false)
		r.info[key+" Seconds"] = float64(ticks) / 256
	}
	if r.err != nil {
		return nil, r.err
	}
	r.info["Protocol Name"], r.info["Packet Name"] = "STP", "Configuration"
	if typ == 0 && v != 0 {
		r.info["Conformance Warning"] = "Configuration layout is selected by Type0; literal wire version is not evidence of an RSTP/MSTP message"
	}
	if typ == 2 {
		r.need(r.num("Version 1 Length", 1, false) == 0, "selected rapid/multiple BPDU requires Version1Length0")
		r.info["Protocol Name"], r.info["Packet Name"] = "RSTP", "Rapid Configuration"
	}
	if typ == 2 && v == 3 {
		r.info["Protocol Name"], r.info["Packet Name"] = "MSTP", "Multiple Configuration"
		stpMST(r, &budget)
	}
	if r.err != nil {
		return nil, r.err
	}
	if pvst {
		if typ == 0 {
			r.take("PVST Alignment", 1)
		}
		stpPVST(r, &budget)
		if typ == 0 {
			r.info["Protocol Name"] = "PVST+"
		} else {
			r.info["Protocol Name"] = "RPVST+"
		}
	}
	return stpFinish(r)
}

func stpFinish(r *controlReader) (map[string]any, error) {
	r.need(r.at == len(r.w), "STP trailing bytes inside declared carrier length")
	if r.err != nil {
		return nil, r.err
	}
	r.info["Wire Field Ranges"] = stpFieldRanges(r.fields)
	r.info["PDU Length"] = uint64(len(r.w))
	return r.info, nil
}

func stpFieldRanges(fields []tlsCertificateField) map[string]any {
	ranges := routingFieldRanges(fields)
	for _, prefix := range []string{"Root", "Bridge", "CIST Bridge", "Regional Root"} {
		if span, ok := ranges[prefix+" Priority and Extension"]; ok {
			ranges[prefix+" Priority"], ranges[prefix+" System ID Extension"], ranges[prefix+" Priority Nibble"] = span, span, span
		}
	}
	for _, key := range []string{"Message Age", "Max Age", "Hello Time", "Forward Delay"} {
		if span, ok := ranges[key+" Ticks"]; ok {
			ranges[key+" Seconds"] = span
		}
	}
	return ranges
}

func stpBridge(r *controlReader, prefix string) {
	n := r.num(prefix+" Priority and Extension", 2, false)
	r.info[prefix+" Priority"], r.info[prefix+" System ID Extension"] = n&0xf000, n&0x0fff
	b := r.take(prefix+" MAC", 6)
	if r.err == nil {
		r.info[prefix+" MAC"] = net.HardwareAddr(b).String()
	}
}

func stpPVST(r *controlReader, budget *int) {
	var rows []map[string]any
	var vlan uint64
	seen := false
	for r.at < len(r.w) && r.err == nil {
		if !pimCharge(r, budget) {
			return
		}
		start := r.at
		typ := r.num("PVST TLV Type", 2, false)
		n := r.num("PVST TLV Length", 2, false)
		r.need(n <= uint64(len(r.w)-r.at), "PVST TLV exceeds carrier")
		if r.err != nil {
			return
		}
		row := map[string]any{"Type": typ, "Length": n}
		ranges := map[string]any{"Type": []any{uint64(start), uint64(start + 2)}, "Length": []any{uint64(start + 2), uint64(start + 4)}}
		if typ == 0 {
			r.need(n == 2, "PVST originating VLAN TLV must contain two bytes")
			if r.err != nil {
				return
			}
			value := r.num("Originating VLAN", 2, false)
			r.need(value >= 1 && value <= 4094, "PVST originating VLAN range")
			r.need(!seen || vlan == value, "PVST conflicting originating VLAN claims")
			vlan, seen = value, true
			row["Originating VLAN"] = value
			ranges["Originating VLAN"] = []any{uint64(start + 4), uint64(r.at)}
		} else {
			row["Value"] = r.take("PVST Unknown TLV Value", int(n))
			r.info["Unselected Extensions"] = true
			ranges["Value"] = []any{uint64(start + 4), uint64(r.at)}
		}
		row["Wire Field Ranges"] = ranges
		rows = append(rows, row)
	}
	r.need(seen, "PVST requires originating VLAN TLV")
	r.info["PVST TLVs"] = rows
}

func stpMST(r *controlReader, budget *int) {
	n := r.num("Version 3 Length", 2, false)
	if r.err != nil {
		return
	}
	// Older vendor formats use counts or a different position for the length.
	// Retain the refusal rather than treating them as a standard MST layout.
	if n < 64 {
		r.err = fmt.Errorf("%w: legacy/alternative MST length encoding", ErrObservedWireContext)
		return
	}
	r.need((n-64)%16 == 0 && n <= 64+64*16 && int(n) == len(r.w)-38, "MST exact version3 length and complete MSTI records")
	if r.err != nil {
		return
	}
	format := r.num("MST Configuration Format Selector", 1, false)
	if format != 0 && r.err == nil {
		r.err = fmt.Errorf("%w: unselected MST configuration format", ErrObservedWireContext)
		return
	}
	name := r.take("MST Configuration Name Bytes", 32)
	if r.err == nil {
		r.info["MST Configuration Name"] = string(bytes.TrimRight(name, "\x00"))
	}
	r.num("MST Configuration Revision", 2, false)
	r.take("MST Configuration Digest", 16)
	r.info["MST Digest Verified"] = false // mapping table is not in the BPDU
	r.num("CIST Internal Root Path Cost", 4, false)
	stpBridge(r, "CIST Bridge")
	r.num("CIST Remaining Hops", 1, false)
	var rows []map[string]any
	seen := map[uint64]bool{}
	for r.at < len(r.w) && r.err == nil {
		if !pimCharge(r, budget) {
			return
		}
		row := r.child("MSTI", r.at+16, func(c *controlReader) {
			flags := c.num("Flags", 1, false)
			c.info["Topology Change"], c.info["Proposal"], c.info["Port Role"] = flags&1 != 0, flags&2 != 0, (flags>>2)&3
			c.info["Learning"], c.info["Forwarding"], c.info["Agreement"], c.info["Master"] = flags&16 != 0, flags&32 != 0, flags&64 != 0, flags&128 != 0
			stpBridge(c, "Regional Root")
			c.info["Regional Root Priority Nibble"] = c.info["Regional Root Priority"].(uint64) >> 12
			id := c.info["Regional Root System ID Extension"].(uint64)
			c.need(id != 0 && id <= 4094 && !seen[id], "MSTI zero/duplicate/invalid instance identifier")
			seen[id] = true
			c.info["Instance ID"] = id
			c.num("Internal Root Path Cost", 4, false)
			c.info["Bridge Priority"] = c.num("Bridge Priority Byte", 1, false) >> 4
			c.info["Port Priority"] = c.num("Port Priority Byte", 1, false) >> 4
			c.num("Remaining Hops", 1, false)
			c.info["Wire Field Ranges"] = stpFieldRanges(c.fields)
			ranges := c.info["Wire Field Ranges"].(map[string]any)
			ranges["Bridge Priority"], ranges["Port Priority"] = ranges["Bridge Priority Byte"], ranges["Port Priority Byte"]
		})
		if r.err == nil {
			rows = append(rows, row)
		}
	}
	r.info["MST Instances"] = rows
}
