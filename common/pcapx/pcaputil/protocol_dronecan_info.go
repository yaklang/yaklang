package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// GetNodeInfo is an explicitly selected, unauthenticated v0 response wire
// observation. UID, certificate, revision and image hash remain device claims.
var droneCANNodeInfoSpec = droneCANResponseSpec{1, 0xee468a8121c46a9e, 376, "dronecan-v0-node-info", "uavcan.protocol.GetNodeInfo"}

func decodeDroneCANNodeInfoBody(body []byte, budget ParserBudget) (map[string]any, error) {
	bad := func(why string) (map[string]any, error) {
		return nil, protocolError(ErrMalformedMessage, "DroneCAN GetNodeInfo "+why)
	}
	// Byte-aligned nested status/software/hardware, eight-bit certificate
	// length, then a tail-optimized nonempty name (no name length prefix).
	if len(body) < 42 || len(body) > 376 {
		return bad("response body length")
	}
	certificateLength := int(body[40])
	nameStart := 41 + certificateLength
	if nameStart >= len(body) || len(body)-nameStart > 80 {
		return bad("certificate length or tail-optimized name length")
	}
	name := body[nameStart:]
	for _, v := range name {
		if !(v >= 'a' && v <= 'z' || v >= '0' && v <= '9' || v == '.' || v == '-' || v == '_') {
			return bad("node name contains a disallowed ASCII octet")
		}
	}
	if max(16, certificateLength, len(name)) > sessionCollectionLimit(budget.MaxCollectionElements) {
		return nil, protocolError(ErrResourceExceeded, "DroneCAN GetNodeInfo byte-array element budget")
	}
	flags := body[9]
	uid := bytes.Clone(body[24:40])
	defined := false
	for _, v := range uid {
		defined = defined || v != 0
	}
	return map[string]any{
		"Observation":      "unverified-node-information",
		"Node Status":      map[string]any{"Uptime Seconds": binary.LittleEndian.Uint32(body), "Health": body[4] >> 6, "Mode": body[4] >> 3 & 7, "Sub Mode": body[4] & 7, "Vendor Status Code": binary.LittleEndian.Uint16(body[5:7])},
		"Software Version": map[string]any{"Major": body[7], "Minor": body[8], "Defined": body[7] != 0 || body[8] != 0, "Optional Field Flags": flags, "VCS Commit Present": flags&1 != 0, "Image CRC Present": flags&2 != 0, "VCS Commit": binary.LittleEndian.Uint32(body[10:14]), "Image CRC": binary.LittleEndian.Uint64(body[14:22])},
		"Hardware Version": map[string]any{"Major": body[22], "Minor": body[23], "Unique ID": uid, "Unique ID Defined": defined, "Certificate Length": certificateLength, "Certificate": bytes.Clone(body[41:nameStart])},
		"Node Name":        string(name),
	}, nil
}
