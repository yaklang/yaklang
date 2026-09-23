package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// MQTT-SN v1.2 is a UDP message protocol, not MQTT's TCP control-packet
// encoding. The one- or three-octet length includes the header itself.
// Admit only message types whose complete wire shape is handled below.
func decodeMQTTSNMessage(w []byte, _ int) (map[string]any, error) {
	if len(w) < 2 {
		return nil, fmt.Errorf("mqtt-sn: truncated header")
	}
	header := 1
	length := int(w[0])
	if w[0] == 1 {
		if len(w) < 4 {
			return nil, fmt.Errorf("mqtt-sn: truncated extended length")
		}
		header = 3
		length = int(binary.BigEndian.Uint16(w[1:3]))
	}
	if length != len(w) || length < header+1 {
		return nil, fmt.Errorf("mqtt-sn: declared length does not match datagram")
	}
	kind := w[header]
	body := w[header+1:]
	fields := map[string]any{"Length": length, "Message Type Code": kind}
	switch kind {
	case 0x01: // SEARCHGW
		if len(body) != 1 {
			return nil, fmt.Errorf("mqtt-sn: SEARCHGW length")
		}
		fields["Message Type"], fields["Radius"] = "SEARCHGW", body[0]
	case 0x02: // GWINFO without optional gateway address
		if len(body) < 1 {
			return nil, fmt.Errorf("mqtt-sn: GWINFO length")
		}
		fields["Message Type"], fields["Gateway ID"] = "GWINFO", body[0]
		if len(body) > 1 {
			fields["Gateway Address"] = append([]byte(nil), body[1:]...)
		}
	case 0x04: // CONNECT
		if len(body) < 5 || body[1] != 1 || len(body[4:]) == 0 {
			return nil, fmt.Errorf("mqtt-sn: CONNECT fields")
		}
		fields["Message Type"], fields["Flags"] = "CONNECT", body[0]
		fields["Protocol ID"] = body[1]
		fields["Duration"] = binary.BigEndian.Uint16(body[2:4])
		fields["Client ID"] = string(body[4:])
	case 0x05: // CONNACK
		if len(body) != 1 || body[0] > 3 {
			return nil, fmt.Errorf("mqtt-sn: CONNACK fields")
		}
		fields["Message Type"], fields["Return Code"] = "CONNACK", body[0]
	case 0x0a: // REGISTER
		if len(body) < 5 {
			return nil, fmt.Errorf("mqtt-sn: REGISTER fields")
		}
		fields["Message Type"] = "REGISTER"
		fields["Topic ID"] = binary.BigEndian.Uint16(body[:2])
		fields["Message ID"] = binary.BigEndian.Uint16(body[2:4])
		fields["Topic Name"] = string(body[4:])
	case 0x0b: // REGACK
		if len(body) != 5 || body[4] > 3 {
			return nil, fmt.Errorf("mqtt-sn: REGACK fields")
		}
		fields["Message Type"] = "REGACK"
		fields["Topic ID"] = binary.BigEndian.Uint16(body[:2])
		fields["Message ID"] = binary.BigEndian.Uint16(body[2:4])
		fields["Return Code"] = body[4]
	case 0x0c: // PUBLISH
		if len(body) < 5 {
			return nil, fmt.Errorf("mqtt-sn: PUBLISH fields")
		}
		fields["Message Type"], fields["Flags"] = "PUBLISH", body[0]
		fields["QoS"] = (body[0] >> 5) & 3
		fields["Topic ID"] = binary.BigEndian.Uint16(body[1:3])
		fields["Message ID"] = binary.BigEndian.Uint16(body[3:5])
		fields["Data"] = append([]byte(nil), body[5:]...)
	case 0x0d: // PUBACK
		if len(body) != 5 || body[4] > 3 {
			return nil, fmt.Errorf("mqtt-sn: PUBACK fields")
		}
		fields["Message Type"] = "PUBACK"
		fields["Topic ID"] = binary.BigEndian.Uint16(body[:2])
		fields["Message ID"] = binary.BigEndian.Uint16(body[2:4])
		fields["Return Code"] = body[4]
	case 0x18: // DISCONNECT, optionally with two-byte duration
		if len(body) != 0 && len(body) != 2 {
			return nil, fmt.Errorf("mqtt-sn: DISCONNECT length")
		}
		fields["Message Type"] = "DISCONNECT"
		if len(body) == 2 {
			fields["Duration"] = binary.BigEndian.Uint16(body)
		}
	default:
		return nil, fmt.Errorf("mqtt-sn: unsupported message type %#x", kind)
	}
	return fields, nil
}

func validMQTTSNMessage(w []byte) bool {
	_, err := decodeMQTTSNMessage(w, 0)
	return err == nil
}
