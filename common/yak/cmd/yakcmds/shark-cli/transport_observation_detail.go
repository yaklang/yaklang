package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

func transportObservationDetail(v *packetTransportView) ([]field, []detailSource) {
	if v == nil || len(v.Header) != v.HeaderLength || len(v.Header) < 8 {
		return nil, nil
	}
	w := v.Header
	name := strings.ToUpper(v.Protocol)
	out := []field{{text: name + " · observed transport header", branch: true, source: 1, start: 0, end: len(w), hasRange: true},
		{text: "IP endpoints: " + v.Source + " → " + v.Destination},
		{text: v.Evidence},
		{text: fmt.Sprintf("Header bytes: %d · captured payload bytes: %d · %d contributing frame(s)", len(w), v.PayloadLength, len(v.PacketRefs))},
	}
	add := func(label string, start, end int, value any) {
		if start >= 0 && end <= len(w) && end > start {
			out = append(out, field{text: fmt.Sprintf("%s: %v", label, value), source: 1, start: start, end: end, hasRange: true})
		}
	}
	add("Source Port", 0, 2, binary.BigEndian.Uint16(w))
	add("Destination Port", 2, 4, binary.BigEndian.Uint16(w[2:]))
	if v.Protocol == "tcp" && len(w) >= 20 {
		add("Sequence Number", 4, 8, v.Sequence)
		add("Acknowledgment Number", 8, 12, v.Acknowledgment)
		add("Header Length", 12, 13, fmt.Sprintf("%d bytes", v.HeaderLength))
		add("Flags", 12, 14, fmt.Sprintf("0x%03x · sequence acceptance separate", v.TCPFlags))
		add("Window", 14, 16, binary.BigEndian.Uint16(w[14:]))
		add("Checksum", 16, 18, fmt.Sprintf("0x%04x · not verified", binary.BigEndian.Uint16(w[16:])))
		add("Urgent Pointer", 18, 20, binary.BigEndian.Uint16(w[18:]))
		if len(w) > 20 {
			add("Options", 20, len(w), fmt.Sprintf("%x", w[20:]))
		}
	} else if v.Protocol == "udp" {
		add("Declared UDP Length", 4, 6, binary.BigEndian.Uint16(w[4:]))
		add("Checksum", 6, 8, fmt.Sprintf("0x%04x · not verified", binary.BigEndian.Uint16(w[6:])))
	}
	return out, []detailSource{{Name: name + " header · network observation; logical offsets", Data: bytes.Clone(w)}}
}

func appendTransportObservation(d *packetDetail, row packetSummary) {
	if row.TransportView == nil {
		return
	}
	fields, sources := transportObservationDetail(row.TransportView)
	for i := range fields {
		if fields[i].source > 0 {
			fields[i].source += len(d.sources)
		}
	}
	d.fields = append(d.fields, fields...)
	d.sources = append(d.sources, sources...)
	if row.PhysicalSource != "" {
		d.fields = append(d.fields, field{text: "Physical frame endpoints: " + row.PhysicalSource + " → " + row.PhysicalDestination})
	}
}
