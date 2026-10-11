package pcaputil

import "encoding/binary"

// NodeStatus is the fixed seven-octet DSDL-v0 broadcast body. Admission is an
// explicit per-interface choice: an extended CAN ID does not identify its stack.
// This profile retains no transfer/session state and cannot establish node
// identity, availability, restart, redundant-interface delivery or bus integrity.
func decodeDroneCANNodeStatus(identifier uint32, data []byte) (map[string]any, error) {
	bad := func(why string) (map[string]any, error) {
		return nil, protocolError(ErrMalformedMessage, "DroneCAN "+why)
	}
	unsupported := func(why string) (map[string]any, error) {
		return nil, protocolError(ErrUnsupportedFeature, "DroneCAN "+why)
	}
	if len(data) == 0 {
		return bad("transfer tail is absent")
	}
	tail := data[len(data)-1]
	start, end, toggle := tail&128 != 0, tail&64 != 0, tail&32 != 0
	source := identifier & 127
	service := identifier&128 != 0
	if start && toggle {
		return bad("first/single frame Toggle must be zero")
	}
	if !end && len(data) != 8 {
		return bad("non-final classical frame must fill its data field")
	}
	if service && (source == 0 || identifier>>8&127 == 0) {
		return bad("service source/destination node ID is zero")
	}
	if !service && source == 0 {
		if !start || !end {
			return bad("anonymous transfer cannot be multi-frame")
		}
		return unsupported("anonymous message is outside NodeStatus profile")
	}
	if !start || !end {
		return unsupported("multi-frame transfer requires reassembly and type-signature CRC")
	}
	if service {
		return decodeDroneCANRestartNode(identifier, data)
	}
	dataType := identifier >> 8 & 65535
	if dataType != 341 {
		return unsupported("message type is outside NodeStatus profile")
	}
	if len(data) != 8 {
		return bad("NodeStatus requires seven body octets and one tail")
	}
	// Multi-byte scalars are little-endian. Unaligned DSDL fields fill each
	// octet MSB-first; reserved Mode and ignored SubMode values stay observable.
	return map[string]any{"Protocol Version": "v0", "Transfer Type": "message", "Priority": identifier >> 24 & 31, "Data Type ID": dataType, "Source Node ID": source, "Transfer ID": tail & 31, "Start of Transfer": start, "End of Transfer": end, "Toggle": toggle, "Content": "uavcan.protocol.NodeStatus", "Observation": "unverified-node-status", "Uptime Seconds": binary.LittleEndian.Uint32(data), "Health": data[4] >> 6, "Mode": data[4] >> 3 & 7, "Sub Mode": data[4] & 7, "Vendor Status Code": binary.LittleEndian.Uint16(data[5:7])}, nil
}

// RestartNode is a five-octet uint40 request or one-bit response. An incorrect
// magic number is a valid request that the server should reject; it is not a
// serialization error. These are stateless captured transfers: neither matching
// CAN IDs nor an OK bit establish invocation association or actual node restart.
func decodeDroneCANRestartNode(identifier uint32, data []byte) (map[string]any, error) {
	if identifier>>16&255 != 5 {
		return nil, protocolError(ErrUnsupportedFeature, "DroneCAN service is outside selected RestartNode profile")
	}
	request := identifier&0x8000 != 0
	tail := data[len(data)-1] // complete single-frame tail validated by caller
	typeName := "response"
	length := 2
	if request {
		typeName, length = "request", 6
	}
	if len(data) != length {
		return nil, protocolError(ErrMalformedMessage, "DroneCAN RestartNode body length is invalid")
	}
	f := map[string]any{"Protocol Version": "v0", "Transfer Type": typeName, "Priority": identifier >> 24 & 31, "Data Type ID": uint32(5), "Source Node ID": identifier & 127, "Destination Node ID": identifier >> 8 & 127, "Transfer ID": tail & 31, "Start of Transfer": true, "End of Transfer": true, "Toggle": false, "Content": "uavcan.protocol.RestartNode", "Observation": "unverified-service-transfer"}
	if request {
		magic := uint64(data[0]) | uint64(data[1])<<8 | uint64(data[2])<<16 | uint64(data[3])<<24 | uint64(data[4])<<32
		f["Magic Number"], f["Magic Number Matches"] = magic, magic == 0xACCE551B1E
	} else {
		// DSDL fills octets MSB-first; the remaining seven bits are opaque
		// padding, not additional boolean fields or a second response.
		f["OK"], f["Payload Padding Bits"] = data[0]&128 != 0, data[0]&127
	}
	return f, nil
}
