package stream_parser

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const mqttFieldsMaxBytes = 1 << 20
const mqttFieldsMaxItems = 1024

// Exact single-packet layouts. A non-CONNECT packet has no version field:
// callers must supply the observed connection's level, never guess it from
// the port or a previous parse in a shared rule cache.
type mqttFieldsReader struct {
	wire       []byte
	at         int
	level      int
	fields     []tlsCertificateField
	err        error
	missing    []string
	flagFields []map[string]any
	props      map[string]any
}

func (r *mqttFieldsReader) take(name, typ string, size int) []byte {
	if r.err != nil {
		return nil
	}
	if size < 0 || size > len(r.wire)-r.at {
		r.err = fmt.Errorf("mqtt-fields: truncated %s", name)
		return nil
	}
	at := r.at
	r.at += size
	r.fields = append(r.fields, tlsCertificateLeaf(name, typ, at, r.at))
	return r.wire[at:r.at]
}

func (r *mqttFieldsReader) number(name string, size int) uint64 {
	b := r.take(name, fmt.Sprintf("uint%d", size*8), size)
	var n uint64
	for _, v := range b {
		n = n<<8 | uint64(v)
	}
	return n
}

func (r *mqttFieldsReader) vector(name string, text bool, raw bool) []byte {
	size := int(r.number(name+" Length", 2))
	typ := "raw"
	if text && !raw {
		typ = "string"
	}
	b := r.take(name, typ, size)
	if r.err == nil && text && (!utf8.Valid(b) || r.level == 4 && bytes.IndexByte(b, 0) >= 0) {
		r.err = fmt.Errorf("mqtt-fields: invalid UTF-8 field %s", name)
	}
	return b
}

func (r *mqttFieldsReader) topic(name string, filter bool) {
	b := r.vector(name, true, false)
	if r.err != nil {
		return
	}
	if len(b) == 0 || bytes.IndexByte(b, 0) >= 0 || r.level == 3 && utf8.RuneCount(b) > 32767 {
		r.err = fmt.Errorf("mqtt-fields: invalid topic length/content")
		return
	}
	for i, c := range b {
		if c == '#' && (!filter || i != len(b)-1 || i > 0 && b[i-1] != '/') ||
			c == '+' && (!filter || i > 0 && b[i-1] != '/' || i+1 < len(b) && b[i+1] != '/') {
			r.err = fmt.Errorf("mqtt-fields: invalid topic wildcard placement")
			return
		}
	}
}

func (r *mqttFieldsReader) packetID() uint64 {
	id := r.number("Packet Identifier", 2)
	if id == 0 && r.err == nil {
		r.err = fmt.Errorf("mqtt-fields: packet identifier must be nonzero")
	}
	return id
}

func (r *mqttFieldsReader) vbi(name string) int {
	n, m := 0, 1
	for i := 0; i < 4; i++ {
		b := r.take(name, "uint8", 1)
		if r.err != nil {
			return 0
		}
		n += int(b[0]&127) * m
		if b[0]&128 == 0 {
			if i > 0 && b[0] == 0 {
				r.err = fmt.Errorf("mqtt-fields: non-canonical variable byte integer")
			}
			return n
		}
		m *= 128
	}
	r.err = fmt.Errorf("mqtt-fields: variable byte integer exceeds four bytes")
	return 0
}

func (r *mqttFieldsReader) properties(label string, allowed map[byte]bool) map[string]any {
	info := map[string]any{}
	n := r.vbi(label + " Properties Length")
	if r.err != nil {
		return info
	}
	end := r.at + n
	if end > len(r.wire) {
		r.err = fmt.Errorf("mqtt-fields: truncated %s properties", label)
		return info
	}
	seen := map[byte]int{}
	for r.at < end && r.err == nil {
		id := byte(r.number(label+" Property Identifier", 1))
		if r.err != nil {
			return info
		}
		if !allowed[id] {
			r.err = fmt.Errorf("mqtt-fields: property 0x%02x is not valid in %s", id, label)
			return info
		}
		seen[id]++
		if id != 0x26 && !(id == 0x0B && label == "PUBLISH") && seen[id] > 1 {
			r.err = fmt.Errorf("mqtt-fields: duplicate property 0x%02x in %s", id, label)
			return info
		}
		switch id {
		case 0x01, 0x17, 0x19, 0x24, 0x25, 0x28, 0x29, 0x2A:
			r.number(mqtt5PropertyName[id], 1)
		case 0x22, 0x13, 0x21, 0x23:
			v := r.number(mqtt5PropertyName[id], 2)
			if id == 0x23 && v == 0 && r.err == nil {
				r.err = fmt.Errorf("mqtt-fields: Topic Alias must be nonzero")
			}
			if id == 0x22 {
				info["Topic Alias Maximum"] = v
			}
			if id == 0x23 {
				info["Topic Alias"] = v
			}
		case 0x02, 0x11, 0x18, 0x27:
			r.number(mqtt5PropertyName[id], 4)
		case 0x03, 0x08, 0x12, 0x15, 0x1A, 0x1C, 0x1F:
			r.vector(mqtt5PropertyName[id], true, false)
		case 0x09, 0x16:
			r.vector(mqtt5PropertyName[id], false, true)
		case 0x0B:
			info["Subscription Identifier"] = uint64(r.vbi("Subscription Identifier"))
		case 0x26:
			r.vector("User Property Name", true, false)
			r.vector("User Property Value", true, false)
		default:
			r.err = fmt.Errorf("mqtt-fields: unhandled property 0x%02x", id)
		}
	}
	if r.err == nil && r.at != end {
		r.err = fmt.Errorf("mqtt-fields: %s properties length mismatch", label)
	}
	if r.props == nil {
		r.props = info
	} else {
		for k, v := range info {
			r.props[k] = v
		}
	}
	return info
}

func (r *mqttFieldsReader) reasonAndProperties(label string, remainingAfterID int, allowed map[byte]bool) uint64 {
	if remainingAfterID == 0 {
		return 0
	}
	code := r.number("Reason Code", 1)
	if remainingAfterID > 1 {
		r.properties(label, allowed)
	}
	return code
}

func (r *mqttFieldsReader) connect() {
	name := string(r.vector("Protocol Name", true, false))
	level := r.number("Protocol Level", 1)
	want := "MQTT"
	if r.level == 3 {
		want = "MQIsdp"
	}
	if r.err != nil {
		return
	}
	if name != want || level != uint64(r.level) {
		r.err = fmt.Errorf("mqtt-fields: protocol name/level differs from explicit profile")
		return
	}
	flagAt := r.at
	flags := r.number("Connect Flags", 1)
	if flags&1 != 0 || flags&4 == 0 && flags&0x38 != 0 || (flags>>3)&3 == 3 || flags&0xc0 == 0x40 {
		r.err = fmt.Errorf("mqtt-fields: invalid CONNECT flag combination")
		return
	}
	sessionName := "Clean Session"
	if r.level == 5 {
		sessionName = "Clean Start"
	}
	r.flagFields = append(r.flagFields, map[string]any{
		"Field": "Connect Flags", "Relative Byte Range": [2]int{flagAt, flagAt + 1},
		"User Name Flag": flags&128 != 0, "Password Flag": flags&64 != 0,
		"Will Retain": flags&32 != 0, "Will QoS": flags >> 3 & 3,
		"Will Flag": flags&4 != 0, sessionName: flags&2 != 0, "Reserved": flags & 1,
	})
	r.number("Keep Alive", 2)
	if r.level == 5 {
		r.properties("CONNECT", mqtt5ConnectProps)
	}
	id := r.vector("Client Identifier", true, false)
	if r.err == nil && (r.level == 3 && (utf8.RuneCount(id) == 0 || utf8.RuneCount(id) > 23) || (r.level == 4 || r.level == 5) && len(id) == 0 && flags&2 == 0) {
		r.err = fmt.Errorf("mqtt-fields: invalid client identifier for profile/clean-session flag")
	}
	if flags&4 != 0 {
		if r.level == 5 {
			r.properties("Will", mqtt5WillProps)
		}
		r.topic("Will Topic", false)
		will := r.vector("Will Message", r.level == 3, true)
		// The published legacy Will is specified as seven-bit ASCII.
		if r.level == 3 && r.err == nil {
			for _, c := range will {
				if c > 127 {
					r.err = fmt.Errorf("mqtt-fields: legacy Will outside ASCII profile")
					break
				}
			}
		}
	}
	for _, spec := range []struct {
		flag uint64
		name string
		text bool
	}{{128, "User Name", true}, {64, "Password", r.level == 3}} {
		if flags&spec.flag == 0 || r.err != nil {
			continue
		}
		// MQTT 3.1 explicitly permits missing trailing User Name/Password
		// fields even when their flags are set. 3.1.1 requires the fields.
		if r.level == 3 && r.at == len(r.wire) {
			r.missing = append(r.missing, spec.name)
			continue
		}
		// Keep these original bytes out of convenience string metadata.
		r.vector(spec.name, spec.text, true)
	}
}

var mqtt5PropertyName = map[byte]string{
	0x01: "Payload Format Indicator", 0x02: "Message Expiry Interval", 0x03: "Content Type",
	0x08: "Response Topic", 0x09: "Correlation Data", 0x0B: "Subscription Identifier",
	0x11: "Session Expiry Interval", 0x12: "Assigned Client Identifier", 0x13: "Server Keep Alive",
	0x15: "Authentication Method", 0x16: "Authentication Data", 0x17: "Request Problem Information",
	0x18: "Will Delay Interval", 0x19: "Request Response Information", 0x1A: "Response Information",
	0x1C: "Server Reference", 0x1F: "Reason String", 0x21: "Receive Maximum",
	0x22: "Topic Alias Maximum", 0x23: "Topic Alias", 0x24: "Maximum QoS",
	0x25: "Retain Available", 0x26: "User Property", 0x27: "Maximum Packet Size",
	0x28: "Wildcard Subscription Available", 0x29: "Subscription Identifier Available",
	0x2A: "Shared Subscription Available",
}

var (
	mqtt5ConnectProps     = mqtt5PropSet(0x11, 0x15, 0x16, 0x17, 0x19, 0x21, 0x22, 0x26, 0x27)
	mqtt5WillProps        = mqtt5PropSet(0x01, 0x02, 0x03, 0x08, 0x09, 0x18, 0x26)
	mqtt5ConnackProps     = mqtt5PropSet(0x11, 0x12, 0x13, 0x15, 0x16, 0x1A, 0x1C, 0x1F, 0x21, 0x22, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2A)
	mqtt5PublishProps     = mqtt5PropSet(0x01, 0x02, 0x03, 0x08, 0x09, 0x0B, 0x23, 0x26)
	mqtt5AckProps         = mqtt5PropSet(0x1F, 0x26)
	mqtt5SubscribeProps   = mqtt5PropSet(0x0B, 0x26)
	mqtt5UnsubscribeProps = mqtt5PropSet(0x26)
	mqtt5DisconnectProps  = mqtt5PropSet(0x11, 0x1C, 0x1F, 0x26)
	mqtt5AuthProps        = mqtt5PropSet(0x15, 0x16, 0x1F, 0x26)
)

func mqtt5PropSet(ids ...byte) map[byte]bool {
	m := make(map[byte]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func decodeMQTTFields(wire []byte, level int) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) < 2 || len(wire) > mqttFieldsMaxBytes || level != 3 && level != 4 && level != 5 {
		return nil, nil, fmt.Errorf("mqtt-fields: explicit level 3/4/5 and 2..1048576 bytes required")
	}
	typ, flags := wire[0]>>4, wire[0]&15
	maxType := byte(14)
	if level == 5 {
		maxType = 15
	}
	if typ == 0 || typ > maxType {
		return nil, nil, fmt.Errorf("mqtt-fields: unsupported packet type")
	}
	qos := flags >> 1 & 3
	if typ == 3 {
		if qos == 3 || qos == 0 && flags&8 != 0 {
			return nil, nil, fmt.Errorf("mqtt-fields: invalid PUBLISH flags")
		}
	} else {
		want := byte(0)
		if typ == 6 || typ == 8 || typ == 10 {
			want = 2
			if level == 3 {
				want |= flags & 8 // Legacy DUP is meaningful for these types.
			}
		}
		if flags != want {
			return nil, nil, fmt.Errorf("mqtt-fields: invalid fixed flags for profile")
		}
	}
	r := &mqttFieldsReader{wire: wire, level: level}
	r.number("Fixed Header", 1)
	remaining, digits := 0, 0
	for ; digits < 4; digits++ {
		b := byte(r.number(fmt.Sprintf("Remaining Length Byte %d", digits), 1))
		if r.err != nil {
			return nil, nil, r.err
		}
		remaining |= int(b&127) << (7 * digits)
		if b&128 == 0 {
			break
		}
	}
	if digits == 4 || remaining != len(wire)-r.at {
		return nil, nil, fmt.Errorf("mqtt-fields: exact remaining-length boundary required")
	}
	digits++
	canonical := digits == 1 || remaining >= 1<<(7*(digits-1))
	items := 0
	var props map[string]any
	switch typ {
	case 1:
		r.connect()
	case 2:
		flagAt := r.at
		ack := r.number("Acknowledgment Flags", 1)
		if level == 5 {
			code := r.number("Reason Code", 1)
			if ack > 1 {
				r.err = fmt.Errorf("mqtt-fields: invalid CONNACK flags/code")
			}
			props = r.properties("CONNACK", mqtt5ConnackProps)
			_ = code
		} else {
			code := r.number("Return Code", 1)
			if ack > 1 || level == 3 && ack != 0 || code > 5 || code != 0 && ack != 0 {
				r.err = fmt.Errorf("mqtt-fields: invalid CONNACK flags/code")
			}
		}
		if level >= 4 {
			r.flagFields = append(r.flagFields, map[string]any{"Field": "Acknowledgment Flags", "Session Present": ack&1 != 0, "Reserved": ack >> 1, "Relative Byte Range": [2]int{flagAt, flagAt + 1}})
		}
	case 3:
		topicAt := r.at
		if level == 5 && r.at+2 <= len(wire) && int(wire[r.at])<<8|int(wire[r.at+1]) == 0 {
			r.vector("Topic Name", true, false)
		} else {
			r.topic("Topic Name", false)
		}
		if qos > 0 {
			r.packetID()
		}
		if level == 5 {
			props = r.properties("PUBLISH", mqtt5PublishProps)
			if r.err == nil {
				topicLen := 0
				if topicAt+2 <= r.at {
					topicLen = int(wire[topicAt])<<8 | int(wire[topicAt+1])
				}
				if topicLen == 0 && props["Topic Alias"] == nil {
					r.err = fmt.Errorf("mqtt-fields: empty PUBLISH topic requires a Topic Alias")
				}
			}
		}
		if r.err == nil {
			r.take("Application Message", "raw", len(wire)-r.at)
		}
	case 4, 5, 6, 7:
		r.packetID()
		if level == 5 {
			r.reasonAndProperties(mqttPacketName(typ), remaining-2, mqtt5AckProps)
		}
	case 11:
		r.packetID()
		if level == 5 {
			props = r.properties("UNSUBACK", mqtt5AckProps)
			listStart := len(r.fields)
			for r.at < len(wire) && r.err == nil {
				if items >= mqttFieldsMaxItems {
					r.err = fmt.Errorf("mqtt-fields: list exceeds 1024 items")
					break
				}
				r.number("Reason Code", 1)
				items++
			}
			if items == 0 && r.err == nil {
				r.err = fmt.Errorf("mqtt-fields: required nonempty payload list")
			}
			if r.err == nil && items > 0 {
				children := append([]tlsCertificateField(nil), r.fields[listStart:]...)
				r.fields = append(r.fields[:listStart], tlsCertificateField{Name: "Payload Items", Start: children[0].Start, End: children[len(children)-1].End, Children: children, List: true})
			}
		}
	case 8, 9, 10:
		r.packetID()
		if level == 5 {
			allowed := mqtt5SubscribeProps
			if typ == 9 {
				allowed = mqtt5AckProps
			} else if typ == 10 {
				allowed = mqtt5UnsubscribeProps
			}
			props = r.properties(mqttPacketName(typ), allowed)
		}
		listStart := len(r.fields)
		for r.at < len(wire) && r.err == nil {
			if items >= mqttFieldsMaxItems {
				r.err = fmt.Errorf("mqtt-fields: list exceeds 1024 items")
				break
			}
			start := len(r.fields)
			if typ == 9 {
				code := r.number("Granted QoS or Return Code", 1)
				if level < 5 && code > 2 && !(level == 4 && code == 128) {
					r.err = fmt.Errorf("mqtt-fields: invalid SUBACK value for profile")
				}
			} else {
				r.topic("Topic Filter", true)
				if typ == 8 {
					if level == 5 {
						opt := r.number("Subscription Options", 1)
						if r.err == nil && (opt&3 > 2 || opt&0xc0 != 0) {
							r.err = fmt.Errorf("mqtt-fields: invalid requested QoS")
						}
					} else if r.number("Requested QoS", 1) > 2 {
						r.err = fmt.Errorf("mqtt-fields: invalid requested QoS")
					}
				}
			}
			if r.err == nil {
				children := append([]tlsCertificateField(nil), r.fields[start:]...)
				r.fields = append(r.fields[:start], tlsCertificateField{Name: fmt.Sprintf("Item %d", items), Start: children[0].Start, End: children[len(children)-1].End, Children: children})
			}
			items++
		}
		if items == 0 && r.err == nil {
			r.err = fmt.Errorf("mqtt-fields: required nonempty payload list")
		}
		if r.err == nil {
			children := append([]tlsCertificateField(nil), r.fields[listStart:]...)
			r.fields = append(r.fields[:listStart], tlsCertificateField{Name: "Payload Items", Start: children[0].Start, End: children[len(children)-1].End, Children: children, List: true})
		}
	case 12, 13:
		// PINGREQ/PINGRESP have no variable header.
	case 14:
		if level == 5 && remaining > 0 {
			r.reasonAndProperties("DISCONNECT", remaining, mqtt5DisconnectProps)
		}
	case 15:
		if remaining == 0 {
			r.err = fmt.Errorf("mqtt-fields: AUTH requires a reason code")
		} else {
			r.reasonAndProperties("AUTH", remaining, mqtt5AuthProps)
		}
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	if r.at != len(wire) {
		return nil, nil, fmt.Errorf("mqtt-fields: trailing packet bytes")
	}
	info := map[string]any{
		"Profile": "MQTT exact packet fields", "Protocol Level Context": level,
		"Packet Type": uint64(typ), "Packet Name": mqttPacketName(typ), "Fixed Flags": uint64(flags), "Remaining Length": remaining,
		"Remaining Length Canonical": canonical, "List Item Count": items,
		"Legacy Omitted Fields": r.missing, "Version Field Present": typ == 1,
		"Decoded Flag Fields":      r.flagFields,
		"TCP Reassembly Performed": false, "Session State Validated": false,
		"Delivery Outcome Validated": false, "Application Body Decoded": false,
		"Structured Generation Supported": false,
	}
	if typ == 3 {
		info["Publish Flags"] = map[string]any{"DUP": flags&8 != 0, "QoS": uint64(qos), "Retain": flags&1 != 0, "Relative Byte Range": [2]int{0, 1}}
	}
	for k, v := range props {
		info[k] = v
	}
	for k, v := range r.props {
		info[k] = v
	}
	return r.fields, info, nil
}

func mqttPacketName(typ byte) string {
	names := [...]string{"", "CONNECT", "CONNACK", "PUBLISH", "PUBACK", "PUBREC", "PUBREL", "PUBCOMP", "SUBSCRIBE", "SUBACK", "UNSUBSCRIBE", "UNSUBACK", "PINGREQ", "PINGRESP", "DISCONNECT", "AUTH"}
	if int(typ) < len(names) {
		return names[typ]
	}
	return "UNKNOWN"
}

func parseMQTTFields(node *base.Node, process func(*base.Node) (func(bool), error), level int) error {
	return parseCertificateFieldTree(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeMQTTFields(w, level)
	}, "mqtt-fields")
}
