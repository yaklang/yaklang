package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// DecodeNetFlowV5Wire decodes a complete self-contained export. No schema from
// another packet is needed after loss, reordering or engine reuse. Its sequence
// counts reported flows, unlike v9 export packets.
func DecodeNetFlowV5Wire(w []byte, limit int) (map[string]any, error) {
	if len(w) < 24 || binary.BigEndian.Uint16(w) != 5 {
		return nil, fmt.Errorf("NetFlow v5: complete 24-byte version-5 header required")
	}
	count := int(binary.BigEndian.Uint16(w[2:4]))
	if count < 1 || count > 30 || len(w) != 24+48*count {
		return nil, fmt.Errorf("NetFlow v5: record count or complete datagram length mismatch")
	}
	if binary.BigEndian.Uint32(w[12:16]) >= 1000000000 {
		return nil, fmt.Errorf("NetFlow v5: invalid residual nanoseconds")
	}
	if limit <= 0 || count*19 > limit {
		return nil, ErrObservedWireBudget
	}
	ranges := map[string]any{}
	m := map[string]any{
		"Protocol Name": "NetFlow v5", "Fixed Header Complete": true,
		"Record Count Verified": true, "Wire Grammar Complete": true,
		"Wire Field Ranges": ranges,
		"Sequence Scope":    "total reported flows; not export packets or captured TCP flows",
		"Observation Scope": "self-contained reported records; no captured connection, delivery, uptime-derived absolute time or authenticated exporter identity implied",
	}
	for _, s := range []struct {
		name   string
		lo, hi int
	}{
		{"Version", 0, 2}, {"Header Record Count", 2, 4},
		{"System Uptime Milliseconds", 4, 8}, {"Unix Seconds", 8, 12}, {"Unix Nanoseconds", 12, 16},
		{"Sequence Number", 16, 20}, {"Engine Type", 20, 21}, {"Engine ID", 21, 22},
	} {
		var value uint64
		for _, b := range w[s.lo:s.hi] {
			value = value<<8 | uint64(b)
		}
		m[s.name], ranges[s.name] = value, []any{uint64(s.lo), uint64(s.hi)}
	}
	sampling := binary.BigEndian.Uint16(w[22:24])
	m["Sampling Mode"], m["Sampling Interval"] = uint64(sampling>>14), uint64(sampling&0x3fff)
	ranges["Sampling Mode"], ranges["Sampling Interval"] = []any{uint64(22), uint64(24)}, []any{uint64(22), uint64(24)}
	if sampling>>14 == 3 {
		m["Conformance Warning"] = "unassigned sampling mode retained literally"
	}
	var records []map[string]any
	for i := 0; i < count; i++ {
		at := 24 + i*48
		var fields []map[string]any
		for _, s := range []struct {
			typ        uint16
			off, width int
		}{
			{8, 0, 4}, {12, 4, 4}, {15, 8, 4}, {10, 12, 2}, {14, 14, 2}, {2, 16, 4}, {1, 20, 4},
			{22, 24, 4}, {21, 28, 4}, {7, 32, 2}, {11, 34, 2}, {6, 37, 1}, {4, 38, 1}, {5, 39, 1},
			{16, 40, 2}, {17, 42, 2}, {9, 44, 1}, {13, 45, 1},
		} {
			lo, hi := at+s.off, at+s.off+s.width
			value, kind, name := netflowV9Value(NetFlowV9Field{Type: s.typ, Length: uint16(s.width)}, w[lo:hi])
			f := map[string]any{"Field Type": uint64(s.typ), "Field Length": uint64(s.width), "Scope": false,
				"Field Name": name, "Value Kind": kind, "Value": value, "Wire Field Ranges": map[string]any{"Value": []any{uint64(lo), uint64(hi)}}}
			if (s.typ == 9 || s.typ == 13) && w[lo] > 32 {
				f["Conformance Warning"] = "reported IPv4 mask exceeds 32; no prefix inferred"
			}
			fields = append(fields, f)
		}
		row := map[string]any{"Index": uint64(i + 1), "Fields": fields, "Padding 1": uint64(w[at+36]), "Padding 2": uint64(binary.BigEndian.Uint16(w[at+46 : at+48])),
			"Wire Field Ranges": map[string]any{"Padding 1": []any{uint64(at + 36), uint64(at + 37)}, "Padding 2": []any{uint64(at + 46), uint64(at + 48)}}}
		if w[at+36] != 0 || w[at+46] != 0 || w[at+47] != 0 {
			row["Conformance Warning"] = "nonzero unused padding retained literally"
		}
		records = append(records, row)
	}
	m["Records"], m["Data Record Count"] = records, uint64(count)
	return m, nil
}
