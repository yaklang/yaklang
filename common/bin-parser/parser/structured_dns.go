package parser

import "encoding/binary"

// Project the pinned dns.yaml grammar, including its original integer types,
// label lists, unresolved RR pointers, DNSA/DNSPTR children and omitted empty
// sections. This is deliberately not a replacement RFC DNS decoder. Flags'
// AddInfo belongs to the legacy Flags node, not the selected DNS node; the
// public structured projection therefore has nil metadata, just as before.
func captureDNSStructured(w []byte) (map[string]any, bool) {
	if len(w) < 12 {
		return nil, false
	}
	header := make(map[string]any, 6)
	for i, name := range [...]string{"ID", "Flags", "Questions", "Answer RRs", "Authority RRs", "Additional RRs"} {
		header[name] = binary.BigEndian.Uint16(w[2*i:])
	}
	fields := map[string]any{"Header": header}
	d := captureDNSReader{wire: w, pos: 12}
	for i, name := range [...]string{"Questions", "Answers", "Authority", "Additional"} {
		count := int(binary.BigEndian.Uint16(w[4+2*i:]))
		// Reject impossible counts before allocating attacker-sized lists. The
		// shortest question is 5 bytes; the shortest RR is 11 bytes.
		minimum := 11
		if i == 0 {
			minimum = 5
		}
		if count > (len(w)-d.pos)/minimum {
			return nil, false
		}
		if count == 0 {
			continue
		}
		rows := make([]any, 0, count)
		for j := 0; j < count; j++ {
			var record map[string]any
			if i == 0 {
				record = d.question()
			} else {
				record = d.record()
			}
			if record == nil {
				return nil, false
			}
			rows = append(rows, record)
		}
		fields[name] = rows
	}
	if d.pos != len(w) {
		return nil, false
	}
	return map[string]any{"fields": fields, "metadata": nil}, true
}

type captureDNSReader struct {
	wire []byte
	pos  int
}

func (d *captureDNSReader) labels() []any {
	var labels []any
	for d.pos < len(d.wire) {
		n := d.wire[d.pos]
		d.pos++
		if int(n) > len(d.wire)-d.pos {
			return nil
		}
		label := map[string]any{"Count": n}
		if n != 0 {
			label["Text"] = string(d.wire[d.pos : d.pos+int(n)])
			d.pos += int(n)
		}
		labels = append(labels, label)
		if n == 0 {
			return labels
		}
	}
	return nil
}

func (d *captureDNSReader) question() map[string]any {
	name := d.labels()
	if name == nil || len(d.wire)-d.pos < 4 {
		return nil
	}
	w := d.wire[d.pos:]
	d.pos += 4
	return map[string]any{"Name": name, "Type": binary.BigEndian.Uint16(w), "Class": binary.BigEndian.Uint16(w[2:])}
}

func (d *captureDNSReader) record() map[string]any {
	if d.pos == len(d.wire) {
		return nil
	}
	var name map[string]any
	if d.wire[d.pos]>>6 == 3 {
		if len(d.wire)-d.pos < 2 {
			return nil
		}
		name = map[string]any{"PointerFlag": uint8(3), "Pointer": binary.BigEndian.Uint16(d.wire[d.pos:]) & 0x3fff}
		d.pos += 2
	} else {
		labels := d.labels()
		if labels == nil {
			return nil
		}
		name = map[string]any{"Labels": labels}
	}
	if len(d.wire)-d.pos < 10 {
		return nil
	}
	w := d.wire[d.pos:]
	d.pos += 10
	typ, n := binary.BigEndian.Uint16(w), int(binary.BigEndian.Uint16(w[8:]))
	if n > len(d.wire)-d.pos {
		return nil
	}
	record := map[string]any{"Name": name, "Type": typ, "Class": binary.BigEndian.Uint16(w[2:]), "TTL": binary.BigEndian.Uint32(w[4:]), "RDLength": uint16(n)}
	switch {
	case n == 0:
	case typ == 1 && n == 4:
		record["DNSA"] = map[string]any{"Address": append([]byte(nil), d.wire[d.pos:d.pos+n]...)}
		d.pos += n
	case typ == 12:
		end := d.pos + n
		labels := d.labels()
		// The legacy PTR operator is not bounded by RDLength. Ambiguous
		// length mismatches retain its interpretation and diagnostics.
		if labels == nil || d.pos != end {
			return nil
		}
		record["DNSPTR"] = map[string]any{"Name": labels}
	default:
		record["RData"] = append([]byte(nil), d.wire[d.pos:d.pos+n]...)
		d.pos += n
	}
	return record
}
