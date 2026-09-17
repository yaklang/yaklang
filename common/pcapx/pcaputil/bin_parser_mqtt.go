package pcaputil

import (
	"encoding/binary"
	"fmt"
)

type binMQTT struct {
	client   int
	level    byte
	pending  [2]map[uint16]string
	aliases  [2]map[uint16]string
	aliasMax [2]uint16
}

func probeMQTT(w []byte, limit int) ProbeResult {
	if len(w) < 2 || w[0] != 0x10 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, header, err := mqttLength(w)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 0 || header == 0 {
		return probeNeed("mqtt", "5.0", len(w), 12)
	}
	if len(w) < header+7 {
		if len(w) >= limit {
			return ProbeResult{Verdict: ProbeReject}
		}
		return probeNeed("mqtt", "5.0", len(w), header+7)
	}
	nameLen := int(binary.BigEndian.Uint16(w[header : header+2]))
	if nameLen > 6 || header+2+nameLen+1 > len(w) {
		if header+2+nameLen+1 > limit && len(w) < limit {
			return probeNeed("mqtt", "5.0", len(w), header+2+nameLen+1)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	name, level := string(w[header+2:header+2+nameLen]), w[header+2+nameLen]
	if name == "MQTT" && level == 5 {
		return probeAccept("mqtt", "5.0", 95)
	}
	_ = n
	return ProbeResult{Verdict: ProbeReject}
}

func (f *binFlow) frameMQTT(w []byte) (int, *binSpec, error) {
	if f.mqtt == nil {
		return 0, nil, sessionContext("MQTT session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(f.mqtt.pending[0])+len(f.mqtt.pending[1]))*16); err != nil {
		return 0, nil, err
	}
	n, _, err := mqttLength(w)
	if err != nil {
		return 0, nil, err
	}
	if n == 0 {
		return 0, nil, nil
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	entry := "MQTT5PacketFields"
	if f.mqtt.level == 4 {
		entry = "MQTT311PacketFields"
	} else if f.mqtt.level == 3 {
		entry = "MQTT31PacketFields"
	}
	return n, f.spec("mqtt_fields", entry), nil
}

func (m *binMQTT) consume(dir int, raw []byte, result map[string]any) (map[string]any, error) {
	if len(raw) < 2 {
		return nil, fmt.Errorf("mqtt: truncated packet")
	}
	typ, flags := raw[0]>>4, raw[0]&15
	qos := flags >> 1 & 3
	info := map[string]any{
		"Packet Type":   uint64(typ),
		"Packet Name":   mqttSessionName(typ),
		"QoS":           uint64(qos),
		"DUP":           flags&8 != 0,
		"Context Level": "observed",
	}
	if meta, ok := result["metadata"].(map[string]any); ok {
		if v, ok := meta["Topic Alias"]; ok {
			info["Topic Alias"] = v
		}
		if v, ok := meta["Topic Alias Maximum"]; ok {
			info["Topic Alias Maximum"] = v
			if n, ok := uintFrom(v); ok {
				m.aliasMax[1-dir] = uint16(n)
			}
		}
		if v, ok := meta["Subscription Identifier"]; ok {
			info["Subscription Identifier"] = v
		}
		if v, ok := meta["Reason Code"]; ok {
			info["Reason Code"] = v
		}
		if name, _ := meta["Packet Name"].(string); name != "" {
			info["Packet Name"] = name
		}
	}
	_, header, err := mqttLength(raw)
	if err != nil || header == 0 || header > len(raw) {
		return info, err
	}
	body := raw[header:]
	switch typ {
	case 1:
		m.client = dir
		if max, ok := mqtt5ConnectAliasMax(body); ok {
			m.aliasMax[1-dir] = max
			info["Topic Alias Maximum"] = uint64(max)
		}
	case 2:
		if dir == m.client {
			return nil, fmt.Errorf("mqtt: CONNACK on the client direction")
		}
		if len(body) >= 2 {
			info["Reason Code"] = uint64(body[1])
			info["Session Present"] = body[0]&1 != 0
		}
	case 3:
		topic, pid, alias, err := mqtt5PublishView(body, qos)
		if err != nil {
			return nil, err
		}
		if alias != 0 {
			if m.aliasMax[dir] == 0 || alias > m.aliasMax[dir] {
				return nil, fmt.Errorf("mqtt: Topic Alias %d exceeds maximum %d", alias, m.aliasMax[dir])
			}
			if topic != "" {
				if m.aliases[dir] == nil {
					m.aliases[dir] = map[uint16]string{}
				}
				m.aliases[dir][alias] = topic
			} else if name, ok := m.aliases[dir][alias]; ok {
				topic = name
			} else {
				return nil, fmt.Errorf("mqtt: unknown Topic Alias %d", alias)
			}
			info["Topic Alias"] = uint64(alias)
		}
		info["Topic Name"] = topic
		if qos > 0 {
			info["Packet Identifier"] = uint64(pid)
			next := "PUBACK"
			if qos == 2 {
				next = "PUBREC"
			}
			m.pending[1-dir][pid] = next
			info["Outstanding"] = next
		}
	case 4, 5, 6, 7, 9, 11:
		if len(body) < 2 {
			return nil, fmt.Errorf("mqtt: missing packet identifier")
		}
		pid := binary.BigEndian.Uint16(body[:2])
		info["Packet Identifier"] = uint64(pid)
		want := mqttSessionName(typ)
		if got := m.pending[dir][pid]; got != "" && got != want {
			return nil, fmt.Errorf("mqtt: packet identifier %d expected %s, got %s", pid, got, want)
		}
		if got := m.pending[dir][pid]; got == want {
			delete(m.pending[dir], pid)
			info["Matched Request"] = true
			if typ == 5 {
				m.pending[1-dir][pid] = "PUBREL"
			} else if typ == 6 {
				m.pending[1-dir][pid] = "PUBCOMP"
			}
		}
		if len(body) > 2 {
			info["Reason Code"] = uint64(body[2])
		}
	case 8, 10:
		if len(body) < 2 {
			return nil, fmt.Errorf("mqtt: missing packet identifier")
		}
		pid := binary.BigEndian.Uint16(body[:2])
		info["Packet Identifier"] = uint64(pid)
		if typ == 8 {
			m.pending[1-dir][pid] = "SUBACK"
		} else {
			m.pending[1-dir][pid] = "UNSUBACK"
		}
	}
	return info, nil
}

func mqttSessionName(typ byte) string {
	names := [...]string{"", "CONNECT", "CONNACK", "PUBLISH", "PUBACK", "PUBREC", "PUBREL", "PUBCOMP", "SUBSCRIBE", "SUBACK", "UNSUBSCRIBE", "UNSUBACK", "PINGREQ", "PINGRESP", "DISCONNECT", "AUTH"}
	if int(typ) < len(names) {
		return names[typ]
	}
	return "UNKNOWN"
}

func uintFrom(v any) (uint64, bool) {
	switch n := v.(type) {
	case uint64:
		return n, true
	case uint16:
		return uint64(n), true
	case int:
		if n >= 0 {
			return uint64(n), true
		}
	}
	return 0, false
}

func mqtt5SkipVBI(w []byte) (int, int, error) {
	n, m, at := 0, 1, 0
	for i := 0; i < 4; i++ {
		if at >= len(w) {
			return 0, 0, fmt.Errorf("mqtt: truncated variable byte integer")
		}
		n += int(w[at]&127) * m
		at++
		if w[at-1]&128 == 0 {
			return n, at, nil
		}
		m *= 128
	}
	return 0, 0, fmt.Errorf("mqtt: variable byte integer exceeds four bytes")
}

func mqtt5ConnectAliasMax(body []byte) (uint16, bool) {
	if len(body) < 7 {
		return 0, false
	}
	nlen := int(binary.BigEndian.Uint16(body[:2]))
	at := 2 + nlen + 1 + 1 + 2
	if at >= len(body) {
		return 0, false
	}
	plen, hdr, err := mqtt5SkipVBI(body[at:])
	if err != nil || at+hdr+plen > len(body) {
		return 0, false
	}
	props := body[at+hdr : at+hdr+plen]
	for len(props) > 0 {
		id := props[0]
		props = props[1:]
		switch id {
		case 0x22:
			if len(props) < 2 {
				return 0, false
			}
			return binary.BigEndian.Uint16(props[:2]), true
		case 0x11, 0x27:
			if len(props) < 4 {
				return 0, false
			}
			props = props[4:]
		case 0x21, 0x13:
			if len(props) < 2 {
				return 0, false
			}
			props = props[2:]
		case 0x17, 0x19, 0x24, 0x25, 0x01, 0x28, 0x29, 0x2A:
			if len(props) < 1 {
				return 0, false
			}
			props = props[1:]
		case 0x0B:
			_, n, err := mqtt5SkipVBI(props)
			if err != nil {
				return 0, false
			}
			props = props[n:]
		case 0x03, 0x08, 0x12, 0x15, 0x1A, 0x1C, 0x1F, 0x09, 0x16, 0x26:
			if len(props) < 2 {
				return 0, false
			}
			l := int(binary.BigEndian.Uint16(props[:2]))
			props = props[2:]
			if id == 0x26 {
				if len(props) < l+2 {
					return 0, false
				}
				l2 := int(binary.BigEndian.Uint16(props[l : l+2]))
				props = props[l+2:]
				if len(props) < l2 {
					return 0, false
				}
				props = props[l2:]
				continue
			}
			if len(props) < l {
				return 0, false
			}
			props = props[l:]
		default:
			return 0, false
		}
	}
	return 0, false
}

func mqtt5PublishView(body []byte, qos byte) (topic string, pid uint16, alias uint16, err error) {
	if len(body) < 2 {
		return "", 0, 0, fmt.Errorf("mqtt: truncated PUBLISH")
	}
	n := int(binary.BigEndian.Uint16(body[:2]))
	if 2+n > len(body) {
		return "", 0, 0, fmt.Errorf("mqtt: truncated PUBLISH topic")
	}
	topic = string(body[2 : 2+n])
	at := 2 + n
	if qos > 0 {
		if at+2 > len(body) {
			return "", 0, 0, fmt.Errorf("mqtt: truncated PUBLISH packet identifier")
		}
		pid = binary.BigEndian.Uint16(body[at : at+2])
		at += 2
	}
	plen, hdr, err := mqtt5SkipVBI(body[at:])
	if err != nil || at+hdr+plen > len(body) {
		return topic, pid, 0, err
	}
	props := body[at+hdr : at+hdr+plen]
	for len(props) > 0 {
		id := props[0]
		props = props[1:]
		switch id {
		case 0x23:
			if len(props) < 2 {
				return "", 0, 0, fmt.Errorf("mqtt: truncated Topic Alias")
			}
			alias = binary.BigEndian.Uint16(props[:2])
			props = props[2:]
		case 0x01:
			if len(props) < 1 {
				return "", 0, 0, fmt.Errorf("mqtt: truncated property")
			}
			props = props[1:]
		case 0x02:
			if len(props) < 4 {
				return "", 0, 0, fmt.Errorf("mqtt: truncated property")
			}
			props = props[4:]
		case 0x0B:
			_, n, err := mqtt5SkipVBI(props)
			if err != nil {
				return "", 0, 0, err
			}
			props = props[n:]
		case 0x03, 0x08, 0x1F, 0x09:
			if len(props) < 2 {
				return "", 0, 0, fmt.Errorf("mqtt: truncated property")
			}
			l := int(binary.BigEndian.Uint16(props[:2]))
			if 2+l > len(props) {
				return "", 0, 0, fmt.Errorf("mqtt: truncated property")
			}
			props = props[2+l:]
		case 0x26:
			if len(props) < 2 {
				return "", 0, 0, fmt.Errorf("mqtt: truncated user property")
			}
			l := int(binary.BigEndian.Uint16(props[:2]))
			if 2+l+2 > len(props) {
				return "", 0, 0, fmt.Errorf("mqtt: truncated user property")
			}
			l2 := int(binary.BigEndian.Uint16(props[2+l : 2+l+2]))
			if 2+l+2+l2 > len(props) {
				return "", 0, 0, fmt.Errorf("mqtt: truncated user property")
			}
			props = props[2+l+2+l2:]
		default:
			return topic, pid, alias, nil
		}
	}
	return topic, pid, alias, nil
}
