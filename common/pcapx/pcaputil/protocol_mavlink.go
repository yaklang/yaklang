package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// The common-field profile is pinned to mavlink/c_library_v2 commit
// 28eae47457249ab6c8c1cadd104fcbe25eacd7a3 and mavlink/mavlink XML commit
// dccd82c4ba56a721aa5bd760498cbf5f2338012b. It admits only HEARTBEAT,
// SYS_STATUS and GLOBAL_POSITION_INT; a checksum cannot identify a dialect or
// authenticate the sender. A v2 signature is retained as opaque observed bytes.
// https://mavlink.io/en/guide/serialization.html
type mavlinkMessage struct {
	Raw    []byte
	Fields map[string]any
	Offset int
}

type mavlinkDefinition struct {
	name     string
	min, max int
	extra    byte
}

func mavlinkDefinitionFor(id uint32) (mavlinkDefinition, bool) {
	switch id {
	case 0:
		return mavlinkDefinition{"HEARTBEAT", 9, 9, 50}, true
	case 1:
		return mavlinkDefinition{"SYS_STATUS", 31, 43, 124}, true
	case 33:
		return mavlinkDefinition{"GLOBAL_POSITION_INT", 28, 28, 104}, true
	default:
		return mavlinkDefinition{}, false
	}
}

// CRC-16/MCRF4XX over the wire header excluding magic, transmitted payload,
// and the pinned message CRC_EXTRA. Zero fill is used for fields, not CRC.
func mavlinkChecksum(w []byte, extra byte) uint16 {
	crc := uint16(0xffff)
	accumulate := func(b byte) {
		tmp := b ^ byte(crc)
		tmp ^= tmp << 4
		crc = crc>>8 ^ uint16(tmp)<<8 ^ uint16(tmp)<<3 ^ uint16(tmp)>>4
	}
	for _, b := range w {
		accumulate(b)
	}
	accumulate(extra)
	return crc
}

func mavlinkFrameSize(w []byte, maxMessage int) (int, mavlinkDefinition, error) {
	var def mavlinkDefinition
	if len(w) == 0 {
		return 0, def, protocolError(ErrNeedMore, "MAVLink datagram is empty")
	}
	header := 6
	switch w[0] {
	case 0xfe:
	case 0xfd:
		header = 10
	default:
		return 0, def, protocolError(ErrUnsupportedVersion, "MAVLink magic is outside v1/v2")
	}
	if len(w) < header {
		return 0, def, protocolError(ErrNeedMore, "MAVLink header is truncated in this datagram")
	}
	id := uint32(w[5])
	signed := false
	if header == 10 {
		if w[2]&^byte(1) != 0 {
			return 0, def, protocolError(ErrUnsupportedFeature, "MAVLink incompatibility flag requires another profile")
		}
		signed = w[2]&1 != 0
		id = uint32(w[7]) | uint32(w[8])<<8 | uint32(w[9])<<16
	}
	var ok bool
	def, ok = mavlinkDefinitionFor(id)
	if !ok {
		return 0, def, protocolError(ErrUnsupportedFeature, "MAVLink message ID has no pinned CRC_EXTRA or field definition")
	}
	payload := int(w[1])
	if header == 6 && payload != def.min || header == 10 && (payload < 1 || payload > def.max) {
		return 0, def, protocolError(ErrMalformedMessage, "MAVLink payload length is outside the message definition")
	}
	n := header + payload + 2
	if signed {
		n += 13
	}
	if maxMessage <= 0 || n > maxMessage {
		return 0, def, protocolError(ErrResourceExceeded, "MAVLink message exceeds configured limit")
	}
	if len(w) < n {
		return n, def, protocolError(ErrNeedMore, "MAVLink payload, checksum or signature is truncated in this datagram")
	}
	checksumAt := header + payload
	if binary.LittleEndian.Uint16(w[checksumAt:checksumAt+2]) != mavlinkChecksum(w[1:checksumAt], def.extra) {
		return n, def, protocolError(ErrMalformedMessage, "MAVLink CRC_EXTRA checksum mismatch")
	}
	return n, def, nil
}

func probeMAVLink(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 0xfe && w[0] != 0xfd {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, _, err := mavlinkFrameSize(w, limit)
	if err != nil {
		if typed, ok := err.(*ProtocolError); ok && typed.Kind == ErrNeedMore {
			if n == 0 {
				n = 6
				if w[0] == 0xfd {
					n = 10
				}
			}
			return probeNeed("mavlink", "v1/v2-common-three-messages", len(w), n)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("mavlink", "v1/v2-common-three-messages", 99)
}

// Each call validates one UDP datagram. An incomplete message is never joined
// to the next datagram, and an invalid co-packed tail yields no decoded prefix.
// Validate before allocating owned message bytes or field maps.
func decodeMAVLinkDatagram(w []byte, maxMessage, maxMessages int) ([]mavlinkMessage, error) {
	if len(w) == 0 {
		return nil, protocolError(ErrNeedMore, "MAVLink datagram is empty")
	}
	if maxMessages <= 0 {
		return nil, protocolError(ErrResourceExceeded, "MAVLink message collection limit")
	}
	count := 0
	for at := 0; at < len(w); {
		if count >= maxMessages {
			return nil, protocolError(ErrResourceExceeded, "MAVLink message collection limit")
		}
		n, _, err := mavlinkFrameSize(w[at:], maxMessage)
		if err != nil {
			return nil, err
		}
		at += n
		count++
	}
	result := make([]mavlinkMessage, 0, count)
	for at := 0; at < len(w); {
		n, def, _ := mavlinkFrameSize(w[at:], maxMessage)
		raw := w[at : at+n]
		fields := mavlinkFields(raw, def)
		result = append(result, mavlinkMessage{Raw: bytes.Clone(raw), Fields: fields, Offset: at})
		at += n
	}
	return result, nil
}

func mavlinkFields(raw []byte, def mavlinkDefinition) map[string]any {
	header, sequenceAt, version := 6, 2, 1
	if raw[0] == 0xfd {
		header, sequenceAt, version = 10, 4, 2
	}
	id := uint32(raw[5])
	if version == 2 {
		id = uint32(raw[7]) | uint32(raw[8])<<8 | uint32(raw[9])<<16
	}
	payloadLength := int(raw[1])
	var p [43]byte
	copy(p[:], raw[header:header+payloadLength])
	fields := map[string]any{
		"Version": version, "Payload Length": payloadLength,
		"Sequence": int(raw[sequenceAt]), "System ID": int(raw[sequenceAt+1]), "Component ID": int(raw[sequenceAt+2]),
		"Message ID": id, "Message Name": def.name, "CRC Extra": int(def.extra),
		"Checksum":          binary.LittleEndian.Uint16(raw[header+payloadLength:]),
		"Signature Present": false, "Signature Status": "absent", "Observation": "unverified-telemetry",
	}
	if version == 2 {
		fields["Incompatibility Flags"], fields["Compatibility Flags"] = int(raw[2]), int(raw[3])
		if raw[2]&1 != 0 {
			fields["Signature Present"], fields["Signature Status"] = true, "opaque-unverified"
			fields["Signature"] = bytes.Clone(raw[len(raw)-13:])
		}
	}
	message := map[string]any{}
	u32 := func(at int) uint32 { return binary.LittleEndian.Uint32(p[at:]) }
	u16 := func(at int) uint16 { return binary.LittleEndian.Uint16(p[at:]) }
	switch id {
	case 0:
		message = map[string]any{"custom_mode": u32(0), "type": p[4], "autopilot": p[5], "base_mode": p[6], "system_status": p[7], "mavlink_version": p[8]}
	case 1:
		message = map[string]any{
			"onboard_control_sensors_present": u32(0), "onboard_control_sensors_enabled": u32(4), "onboard_control_sensors_health": u32(8),
			"load": u16(12), "voltage_battery": u16(14), "current_battery": int16(u16(16)),
			"drop_rate_comm": u16(18), "errors_comm": u16(20), "errors_count1": u16(22), "errors_count2": u16(24), "errors_count3": u16(26), "errors_count4": u16(28), "battery_remaining": int8(p[30]),
		}
		if version == 2 {
			message["onboard_control_sensors_present_extended"] = u32(31)
			message["onboard_control_sensors_enabled_extended"] = u32(35)
			message["onboard_control_sensors_health_extended"] = u32(39)
		}
	case 33:
		message = map[string]any{"time_boot_ms": u32(0), "lat": int32(u32(4)), "lon": int32(u32(8)), "alt": int32(u32(12)), "relative_alt": int32(u32(16)), "vx": int16(u16(20)), "vy": int16(u16(22)), "vz": int16(u16(24)), "hdg": u16(26)}
	}
	fields["Message Fields"] = message
	fields["Summary"] = fmt.Sprintf("MAVLink v%d %s system %d component %d", version, def.name, raw[sequenceAt+1], raw[sequenceAt+2])
	return fields
}
