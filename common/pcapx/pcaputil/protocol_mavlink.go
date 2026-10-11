package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

// The common-field profile is pinned to mavlink/c_library_v2 commit
// 28eae47457249ab6c8c1cadd104fcbe25eacd7a3 and mavlink/mavlink XML commit
// dccd82c4ba56a721aa5bd760498cbf5f2338012b. It admits only HEARTBEAT,
// SYS_STATUS, GPS_RAW_INT, ATTITUDE and GLOBAL_POSITION_INT; a checksum cannot identify a dialect or
// authenticate the sender. A v2 signature is opaque by default; explicit
// MAVLinkSigningConfig enables bounded receiver-context signature verification.
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
	case 24:
		return mavlinkDefinition{"GPS_RAW_INT", 30, 52, 24}, true
	case 30:
		return mavlinkDefinition{"ATTITUDE", 28, 28, 39}, true
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

func mavlinkProfile(w []byte) string {
	if len(w) >= 6 && w[0] == 0xfe && w[5] == 24 || len(w) >= 10 && w[0] == 0xfd && w[7] == 24 && w[8] == 0 && w[9] == 0 {
		return "mavlink-common-gps-raw-int"
	}
	if len(w) >= 6 && w[0] == 0xfe && w[5] == 30 || len(w) >= 10 && w[0] == 0xfd && w[7] == 30 && w[8] == 0 && w[9] == 0 {
		return "mavlink-common-attitude"
	}
	return "mavlink-common-three-messages"
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
			return probeNeed("mavlink", mavlinkProbeProfile(w), len(w), n)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("mavlink", mavlinkProbeProfile(w), 99)
}

func mavlinkProbeProfile(w []byte) string {
	if mavlinkProfile(w) == "mavlink-common-gps-raw-int" {
		return "v1/v2-common-gps-raw-int"
	}
	if mavlinkProfile(w) == "mavlink-common-attitude" {
		return "v1/v2-common-attitude"
	}
	return "v1/v2-common-three-messages"
}

// Each call validates one UDP datagram. An incomplete message is never joined
// to the next datagram, and an invalid co-packed tail yields no decoded prefix.
// Validate before allocating owned message bytes or field maps.
func decodeMAVLinkDatagram(w []byte, maxMessage, maxMessages int) ([]mavlinkMessage, error) {
	return decodeMAVLinkDatagramBudget(w, maxMessage, maxMessages, DefaultParserBudget().MaxRecursionDepth)
}

func decodeMAVLinkDatagramBudget(w []byte, maxMessage, maxMessages, maxDepth int) ([]mavlinkMessage, error) {
	count, err := validateMAVLinkDatagramBudget(w, maxMessage, maxMessages, maxDepth)
	if err != nil {
		return nil, err
	}
	return projectMAVLinkDatagram(w, maxMessage, count), nil
}

func validateMAVLinkDatagramBudget(w []byte, maxMessage, maxMessages, maxDepth int) (int, error) {
	if len(w) == 0 {
		return 0, protocolError(ErrNeedMore, "MAVLink datagram is empty")
	}
	if maxMessages <= 0 {
		return 0, protocolError(ErrResourceExceeded, "MAVLink message collection limit")
	}
	count := 0
	for at := 0; at < len(w); {
		if count >= maxMessages {
			return 0, protocolError(ErrResourceExceeded, "MAVLink message collection limit")
		}
		n, _, err := mavlinkFrameSize(w[at:], maxMessage)
		if err != nil {
			return 0, err
		}
		// ATTITUDE adds one bounded map for each of six binary32 fields. Check
		// the complete datagram before expanding any projection or prefix.
		if mavlinkProfile(w[at:]) == "mavlink-common-attitude" && maxDepth < 3 {
			return 0, protocolError(ErrResourceExceeded, "MAVLink ATTITUDE field projection exceeds configured depth")
		}
		if mavlinkProfile(w[at:]) == "mavlink-common-gps-raw-int" && maxDepth < 2 {
			return 0, protocolError(ErrResourceExceeded, "MAVLink GPS_RAW_INT field projection exceeds configured depth")
		}
		// HEARTBEAT, SYS_STATUS and GLOBAL_POSITION_INT also contain a
		// nested Message Fields map. Admit no prefix before checking every
		// co-packed message against this shared projection boundary.
		if maxDepth < 2 {
			return 0, protocolError(ErrResourceExceeded, "MAVLink common field projection exceeds configured depth")
		}
		at += n
		count++
	}
	return count, nil
}

// Caller validated the complete datagram before constructing owned fields.
func projectMAVLinkDatagram(w []byte, maxMessage, count int) []mavlinkMessage {
	result := make([]mavlinkMessage, 0, count)
	for at := 0; at < len(w); {
		n, def, _ := mavlinkFrameSize(w[at:], maxMessage)
		raw := w[at : at+n]
		fields := mavlinkFields(raw, def)
		result = append(result, mavlinkMessage{Raw: bytes.Clone(raw), Fields: fields, Offset: at})
		at += n
	}
	return result
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
	var p [52]byte
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
	case 24:
		// Literal common-dialect integers, including unknown-value sentinels.
		// time_usec may be epoch or boot-relative; no time origin, fix quality
		// or physical position is inferred. v1 has no extension fields.
		message = map[string]any{
			"time_usec": binary.LittleEndian.Uint64(p[:8]), "lat": int32(u32(8)), "lon": int32(u32(12)), "alt": int32(u32(16)),
			"eph": u16(20), "epv": u16(22), "vel": u16(24), "cog": u16(26), "fix_type": p[28], "satellites_visible": p[29],
		}
		if version == 2 {
			message["alt_ellipsoid"] = int32(u32(30))
			message["h_acc"], message["v_acc"] = u32(34), u32(38)
			message["vel_acc"], message["hdg_acc"] = u32(42), u32(46)
			message["yaw"] = u16(50)
			fields["Payload Zero Fill Bytes"] = 52 - payloadLength
		}
	case 33:
		message = map[string]any{"time_boot_ms": u32(0), "lat": int32(u32(4)), "lon": int32(u32(8)), "alt": int32(u32(12)), "relative_alt": int32(u32(16)), "vx": int16(u16(20)), "vy": int16(u16(22)), "vz": int16(u16(24)), "hdg": u16(26)}
	case 30:
		message["time_boot_ms"] = u32(0)
		fields["Payload Zero Fill Bytes"] = 28 - payloadLength
		payload := raw[header : header+payloadLength]
		for i, key := range []string{"roll", "pitch", "yaw", "rollspeed", "pitchspeed", "yawspeed"} {
			at := 4 + 4*i
			message[key] = mavlinkFloatField(payload[min(at, payloadLength):min(at+4, payloadLength)])
		}
	}
	fields["Message Fields"] = message
	fields["Summary"] = fmt.Sprintf("MAVLink v%d %s system %d component %d", version, def.name, raw[sequenceAt+1], raw[sequenceAt+2])
	return fields
}

// MAVLink v2's omitted zero tail is inferred, never reported as captured wire.
// Preserve all IEEE bits including NaN payloads and signed zero. Non-finite
// values use JSON-safe text; physical attitude and source authenticity are not
// inferred from these literal telemetry fields.
func mavlinkFloatField(wire []byte) map[string]any {
	var padded [4]byte
	copy(padded[:], wire)
	bits := binary.LittleEndian.Uint32(padded[:])
	exponent, fraction := bits>>23&255, bits&0x7fffff
	kind := "normal"
	var value any = float64(math.Float32frombits(bits))
	if exponent == 255 {
		kind, value = "infinite", "Infinity"
		if bits>>31 != 0 {
			value = "-Infinity"
		}
		if fraction != 0 {
			kind, value = "nan", "NaN"
		}
	} else if exponent == 0 {
		kind = "zero"
		if fraction != 0 {
			kind = "subnormal"
		}
	}
	return map[string]any{"ieee754_bits": bits, "wire_hex": hex.EncodeToString(wire), "zero_fill_bytes": 4 - len(wire), "float_class": kind, "negative": bits>>31 != 0, "value": value}
}
