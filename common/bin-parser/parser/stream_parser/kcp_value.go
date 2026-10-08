package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// This is a local allocation boundary, not a KCP MTU or a UDP size claim.
const kcpMaxBytes = 65536

type kcpField struct {
	Name, Type string
	Start, End int
	Children   []kcpField
	Info       map[string]any
}

// decodeKCP reads the original author's little-endian, 24-byte segment layout.
// https://github.com/skywind3000/kcp/blob/b1a7a2101dcbb96017681a500d6b82bbe5a88766/ikcp.c
// Unlike ikcp_input's tolerated final 1..23 bytes, this explicit bounded profile
// requires exact segment consumption. It performs no receiver-state simulation.
func decodeKCP(wire []byte, singleSegment bool) ([]kcpField, map[string]any, error) {
	if len(wire) < 24 || len(wire) > kcpMaxBytes {
		return nil, nil, fmt.Errorf("kcp: explicit boundary must be 24..65536 bytes")
	}
	conversation := binary.LittleEndian.Uint32(wire)
	var fields []kcpField
	for offset := 0; offset < len(wire); {
		if singleSegment && len(fields) != 0 {
			return nil, nil, fmt.Errorf("kcp: single segment has trailing bytes")
		}
		if len(wire)-offset < 24 {
			return nil, nil, fmt.Errorf("kcp: incomplete segment header at byte %d (exact profile)", offset)
		}
		p := wire[offset:]
		if binary.LittleEndian.Uint32(p) != conversation {
			return nil, nil, fmt.Errorf("kcp: inconsistent conversation ID at byte %d", offset)
		}
		length := binary.LittleEndian.Uint32(p[20:24])
		if uint64(length) > uint64(len(p)-24) {
			return nil, nil, fmt.Errorf("kcp: declared segment data length exceeds boundary at byte %d", offset)
		}
		var command string
		switch p[4] {
		case 81:
			command = "PUSH"
		case 82:
			command = "ACK"
		case 83:
			command = "WASK"
		case 84:
			command = "WINS"
		default:
			return nil, nil, fmt.Errorf("kcp: unsupported command %d at byte %d", p[4], offset+4)
		}
		end := offset + 24 + int(length)
		leaf := func(name, typ string, start, end int) kcpField {
			return kcpField{Name: name, Type: typ, Start: offset + start, End: offset + end}
		}
		fields = append(fields, kcpField{
			Name: fmt.Sprintf("Segment %d", len(fields)), Start: offset, End: end,
			Children: []kcpField{
				leaf("Conversation ID", "uint32", 0, 4),
				leaf("Command", "uint8", 4, 5),
				leaf("Fragment", "uint8", 5, 6),
				leaf("Window", "uint16", 6, 8),
				leaf("Timestamp", "uint32", 8, 12),
				leaf("Sequence Number", "uint32", 12, 16),
				leaf("Unacknowledged Sequence", "uint32", 16, 20),
				leaf("Data Length", "uint32", 20, 24),
				leaf("Segment Data", "raw", 24, 24+int(length)),
			},
			Info: map[string]any{
				"Command Name": command, "Receiver Ignores Segment Data": p[4] != 81,
				"Application Data Decoded": false, "Message Reassembled": false,
			},
		})
		offset = end
	}
	return fields, map[string]any{
		"Protocol": "KCP", "Profile": "KCP exact bounded segment layout",
		"Single Segment": singleSegment, "Segment Count": len(fields),
		"Conversation ID": uint64(conversation), "Exact Consumption": true,
		"Implementation Byte Limit":             kcpMaxBytes,
		"Receiver Trailing Bytes Compatibility": false,
		"External Conversation Validated":       false, "Receiver State Validated": false,
		"Message Reassembled": false, "Application Data Decoded": false,
		"Outer Prefix Decoded": false, "Application Identity Inferred": false,
	}, nil
}
