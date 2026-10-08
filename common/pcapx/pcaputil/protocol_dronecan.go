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
		return unsupported("service is outside NodeStatus broadcast profile")
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
