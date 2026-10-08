package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// This test encoder uses a bitwise CRC polynomial independently of the
// optimized production accumulator. Every payload is newly authored.
func mavlinkTestFrame(version int, id uint32, payload []byte, flags byte, extra byte) []byte {
	raw := []byte{0xfe, byte(len(payload)), 7, 1, 2, byte(id)}
	if version == 2 {
		raw = []byte{0xfd, byte(len(payload)), flags, 0, 7, 1, 2, byte(id), byte(id >> 8), byte(id >> 16)}
	}
	raw = append(raw, payload...)
	crc := uint16(0xffff)
	for _, b := range append(bytes.Clone(raw[1:]), extra) {
		crc ^= uint16(b)
		for bit := 0; bit < 8; bit++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0x8408
			} else {
				crc >>= 1
			}
		}
	}
	raw = append(raw, byte(crc), byte(crc>>8))
	if version == 2 && flags&1 != 0 {
		raw = append(raw, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13)
	}
	return raw
}

func mavlinkTestPayload(id uint32) []byte {
	switch id {
	case 0:
		return []byte{4, 3, 2, 1, 2, 3, 128, 4, 3}
	case 1:
		p := make([]byte, 43)
		for i := range p {
			p[i] = byte(i + 1)
		}
		p[16], p[17], p[30] = 255, 255, 255
		return p
	case 33:
		p := make([]byte, 28)
		for i := range p {
			p[i] = byte(i + 1)
		}
		p[4], p[5], p[6], p[7] = 255, 255, 255, 255
		p[20], p[21], p[26], p[27] = 255, 255, 255, 255
		return p
	}
	return nil
}

func TestMAVLinkCompleteFields(t *testing.T) {
	for _, version := range []int{1, 2} {
		for _, id := range []uint32{0, 1, 33} {
			def, _ := mavlinkDefinitionFor(id)
			p := mavlinkTestPayload(id)
			if version == 1 {
				p = p[:def.min]
			}
			raw := mavlinkTestFrame(version, id, p, 0, def.extra)
			messages, err := decodeMAVLinkDatagram(raw, 280, 4)
			require.NoError(t, err)
			require.Len(t, messages, 1)
			f := messages[0].Fields
			require.Equal(t, version, f["Version"])
			require.Equal(t, id, f["Message ID"])
			require.Equal(t, def.name, f["Message Name"])
			require.Equal(t, 7, f["Sequence"])
			require.Equal(t, 1, f["System ID"])
			require.Equal(t, 2, f["Component ID"])
			fields := f["Message Fields"].(map[string]any)
			switch id {
			case 0:
				require.Equal(t, map[string]any{"custom_mode": uint32(0x01020304), "type": byte(2), "autopilot": byte(3), "base_mode": byte(128), "system_status": byte(4), "mavlink_version": byte(3)}, fields)
			case 1:
				want := map[string]any{"onboard_control_sensors_present": uint32(0x04030201), "onboard_control_sensors_enabled": uint32(0x08070605), "onboard_control_sensors_health": uint32(0x0c0b0a09), "load": uint16(0x0e0d), "voltage_battery": uint16(0x100f), "current_battery": int16(-1), "drop_rate_comm": uint16(0x1413), "errors_comm": uint16(0x1615), "errors_count1": uint16(0x1817), "errors_count2": uint16(0x1a19), "errors_count3": uint16(0x1c1b), "errors_count4": uint16(0x1e1d), "battery_remaining": int8(-1)}
				if version == 2 {
					want["onboard_control_sensors_present_extended"] = uint32(0x23222120)
					want["onboard_control_sensors_enabled_extended"] = uint32(0x27262524)
					want["onboard_control_sensors_health_extended"] = uint32(0x2b2a2928)
				}
				require.Equal(t, want, fields)
				if version == 2 {
					require.Len(t, fields, 16)
				} else {
					require.Len(t, fields, 13)
				}
				require.Equal(t, int16(-1), fields["current_battery"])
				require.Equal(t, int8(-1), fields["battery_remaining"])
				require.Equal(t, uint16(0x1e1d), fields["errors_count4"])
				if version == 2 {
					require.Equal(t, uint32(0x23222120), fields["onboard_control_sensors_present_extended"])
				} else {
					require.NotContains(t, fields, "onboard_control_sensors_present_extended")
				}
			case 33:
				require.Equal(t, map[string]any{"time_boot_ms": uint32(0x04030201), "lat": int32(-1), "lon": int32(0x0c0b0a09), "alt": int32(0x100f0e0d), "relative_alt": int32(0x14131211), "vx": int16(-1), "vy": int16(0x1817), "vz": int16(0x1a19), "hdg": uint16(65535)}, fields)
				require.Len(t, fields, 9)
				require.Equal(t, int32(-1), fields["lat"])
				require.Equal(t, int16(-1), fields["vx"])
				require.Equal(t, uint16(65535), fields["hdg"])
				require.Equal(t, uint32(0x04030201), fields["time_boot_ms"])
			}
			require.Equal(t, ProbeAccept, probeMAVLink(raw, 280).Verdict)
			for n := 1; n < len(raw); n++ {
				require.NotEqual(t, ProbeAccept, probeMAVLink(raw[:n], 280).Verdict)
			}
		}
	}
}

func TestMAVLinkV2ZeroTailAndOpaqueSignature(t *testing.T) {
	for _, id := range []uint32{1, 33} {
		def, _ := mavlinkDefinitionFor(id)
		messages, err := decodeMAVLinkDatagram(mavlinkTestFrame(2, id, []byte{42}, 0, def.extra), 280, 1)
		require.NoError(t, err)
		fields := messages[0].Fields["Message Fields"].(map[string]any)
		key := "time_boot_ms"
		if id == 1 {
			key = "onboard_control_sensors_present"
		}
		require.Equal(t, uint32(42), fields[key])
		for k, v := range fields {
			if k != key {
				require.Zero(t, v, k)
			}
		}
	}
	raw := mavlinkTestFrame(2, 0, mavlinkTestPayload(0), 1, 50)
	before := bytes.Clone(raw)
	messages, err := decodeMAVLinkDatagram(raw, 280, 1)
	require.NoError(t, err)
	require.Equal(t, true, messages[0].Fields["Signature Present"])
	require.Equal(t, "opaque-unverified", messages[0].Fields["Signature Status"])
	signature := messages[0].Fields["Signature"].([]byte)
	raw[len(raw)-1] = 0
	require.Equal(t, before, messages[0].Raw)
	require.Equal(t, byte(13), signature[12])
	signature[0] = 99
	require.Equal(t, before, messages[0].Raw)
	for cut := 1; cut <= 13; cut++ {
		out, err := decodeMAVLinkDatagram(before[:len(before)-cut], 280, 1)
		require.Nil(t, out)
		require.ErrorContains(t, err, "signature is truncated")
	}
}

func TestMAVLinkFailsClosedAndDatagramBoundaries(t *testing.T) {
	valid := mavlinkTestFrame(2, 0, mavlinkTestPayload(0), 0, 50)
	controls := map[string][]byte{
		"bad-crc":                    append(bytes.Clone(valid[:len(valid)-1]), valid[len(valid)-1]^1),
		"other-dialect-crc-extra":    mavlinkTestFrame(2, 0, mavlinkTestPayload(0), 0, 51),
		"unknown-message-id":         mavlinkTestFrame(2, 65536, []byte{1}, 0, 50),
		"unknown-incompatibility":    mavlinkTestFrame(2, 0, mavlinkTestPayload(0), 2, 50),
		"v1-omitted-tail":            mavlinkTestFrame(1, 33, []byte{1}, 0, 104),
		"v2-empty-payload":           mavlinkTestFrame(2, 1, nil, 0, 124),
		"payload-exceeds-definition": mavlinkTestFrame(2, 0, make([]byte, 10), 0, 50),
		"partial-second-message":     append(bytes.Clone(valid), valid[:5]...),
	}
	for name, wire := range controls {
		t.Run(name, func(t *testing.T) {
			out, err := decodeMAVLinkDatagram(wire, 280, 10)
			require.Error(t, err)
			require.Nil(t, out)
		})
	}
	for n := 1; n < len(valid); n++ {
		for _, datagram := range [][]byte{valid[:n], valid[n:]} {
			out, err := decodeMAVLinkDatagram(datagram, 280, 10)
			require.Error(t, err)
			require.Nil(t, out)
		}
	}
	pair := append(bytes.Clone(valid), valid...)
	out, err := decodeMAVLinkDatagram(pair, 280, 2)
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, len(valid), out[1].Offset)
	out, err = decodeMAVLinkDatagram(pair, 280, 1)
	require.Nil(t, out)
	require.ErrorContains(t, err, "collection limit")
	out, err = decodeMAVLinkDatagram(valid, len(valid)-1, 1)
	require.Nil(t, out)
	require.ErrorContains(t, err, "configured limit")
	// Compatible flags do not change wire framing; preserve their value.
	compatible := bytes.Clone(valid)
	compatible[3] = 128
	crc := mavlinkChecksum(compatible[1:len(compatible)-2], 50)
	binary.LittleEndian.PutUint16(compatible[len(compatible)-2:], crc)
	out, err = decodeMAVLinkDatagram(compatible, 280, 1)
	require.NoError(t, err)
	require.Equal(t, 128, out[0].Fields["Compatibility Flags"])
}

func TestMAVLinkCopackedEvidenceOwnership(t *testing.T) {
	wire, err := trafficfixture.ReadFile("incremental-mvp/mavlink/captures/copacked-heartbeat-position.pcap")
	require.NoError(t, err)
	for _, deferred := range []bool{false, true} {
		var events []*ProtocolEvent
		require.NoError(t, ReplayPcap(bytes.NewReader(wire), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
		require.Len(t, events, 2)
		require.Len(t, events[0].SourceBytes.PacketRefs, 1)
		require.Len(t, events[1].SourceBytes.PacketRefs, 1)
		first, second := events[0], events[1]
		require.EqualValues(t, 1, second.SourceBytes.PacketRefs[0].Number)
		first.SourceBytes.PacketRefs[0].Number = 90001
		require.EqualValues(t, 1, second.SourceBytes.PacketRefs[0].Number, "separate messages must own separate source references")
		raw := bytes.Clone(second.Raw)
		first.Raw[0] = 0
		require.Equal(t, raw, second.Raw)
		f, err := second.GetFields()
		require.NoError(t, err)
		before := cloneSession(f)
		first.Session["Message ID"] = uint32(999)
		after, err := second.GetFields()
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}
