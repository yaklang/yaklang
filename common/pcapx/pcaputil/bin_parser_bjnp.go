package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// This bounded BJNP datagram profile covers the command families witnessed in
// the printer corpus. Its length, direction and command checks are necessary
// because a UDP port alone is not evidence of a valid printer message.
func decodeBJNPMessage(w []byte, _ int) (map[string]any, error) {
	if len(w) < 16 || !bytes.Equal(w[:4], []byte("BJNP")) {
		return nil, fmt.Errorf("bjnp: missing header")
	}
	device := w[4]
	if device != 0x01 && device != 0x81 {
		return nil, fmt.Errorf("bjnp: unsupported message direction")
	}
	command := w[5]
	var name string
	switch command {
	case 0x01:
		name = "Discover"
	case 0x10:
		name = "Print Job Details"
	case 0x20:
		name = "Get Printer Status"
	case 0x30:
		name = "Get Printer Identity"
	default:
		return nil, fmt.Errorf("bjnp: unsupported command %#x", command)
	}
	length := binary.BigEndian.Uint32(w[12:16])
	if length > uint32(len(w)-16) || int(length) != len(w)-16 {
		return nil, fmt.Errorf("bjnp: payload length mismatch")
	}
	sequence := binary.BigEndian.Uint32(w[6:10])
	if sequence == 0 {
		return nil, fmt.Errorf("bjnp: zero sequence")
	}
	session := binary.BigEndian.Uint16(w[10:12])
	if command == 0x01 && device == 0x01 && session != 0 {
		return nil, fmt.Errorf("bjnp: discovery request has session")
	}
	role := "request"
	if device == 0x81 {
		role = "response"
	}
	return map[string]any{
		"Packet Name": name,
		"Role":        role,
		"Command":     command,
		"Sequence":    sequence,
		"Session":     session,
		"Payload":     bytes.Clone(w[16:]),
	}, nil
}

func validBJNPMessage(w []byte) bool {
	_, err := decodeBJNPMessage(w, 0)
	return err == nil
}
