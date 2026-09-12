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

func (r *mqttFieldsReader) packetID() {
	if r.number("Packet Identifier", 2) == 0 && r.err == nil {
		r.err = fmt.Errorf("mqtt-fields: packet identifier must be nonzero")
	}
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
	r.flagFields = append(r.flagFields, map[string]any{
		"Field": "Connect Flags", "Relative Byte Range": [2]int{flagAt, flagAt + 1},
		"User Name Flag": flags&128 != 0, "Password Flag": flags&64 != 0,
		"Will Retain": flags&32 != 0, "Will QoS": flags >> 3 & 3,
		"Will Flag": flags&4 != 0, "Clean Session": flags&2 != 0, "Reserved": flags & 1,
	})
	r.number("Keep Alive", 2)
	id := r.vector("Client Identifier", true, false)
	if r.err == nil && (r.level == 3 && (utf8.RuneCount(id) == 0 || utf8.RuneCount(id) > 23) || r.level == 4 && len(id) == 0 && flags&2 == 0) {
		r.err = fmt.Errorf("mqtt-fields: invalid client identifier for profile/clean-session flag")
	}
	if flags&4 != 0 {
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

func decodeMQTTFields(wire []byte, level int) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) < 2 || len(wire) > mqttFieldsMaxBytes || level != 3 && level != 4 {
		return nil, nil, fmt.Errorf("mqtt-fields: explicit level 3/4 and 2..1048576 bytes required")
	}
	typ, flags := wire[0]>>4, wire[0]&15
	if typ == 0 || typ > 14 {
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
	switch typ {
	case 1:
		r.connect()
	case 2:
		flagAt := r.at
		ack := r.number("Acknowledgment Flags", 1)
		code := r.number("Return Code", 1)
		if ack > 1 || level == 3 && ack != 0 || code > 5 || code != 0 && ack != 0 {
			r.err = fmt.Errorf("mqtt-fields: invalid CONNACK flags/code")
		}
		if level == 4 {
			r.flagFields = append(r.flagFields, map[string]any{"Field": "Acknowledgment Flags", "Session Present": ack&1 != 0, "Reserved": ack >> 1, "Relative Byte Range": [2]int{flagAt, flagAt + 1}})
		}
	case 3:
		r.topic("Topic Name", false)
		if qos > 0 {
			r.packetID()
		}
		r.take("Application Message", "raw", len(wire)-r.at)
	case 4, 5, 6, 7, 11:
		r.packetID()
	case 8, 9, 10:
		r.packetID()
		listStart := len(r.fields)
		for r.at < len(wire) && r.err == nil {
			if items >= mqttFieldsMaxItems {
				r.err = fmt.Errorf("mqtt-fields: list exceeds 1024 items")
				break
			}
			start := len(r.fields)
			if typ == 9 {
				code := r.number("Granted QoS or Return Code", 1)
				if code > 2 && !(level == 4 && code == 128) {
					r.err = fmt.Errorf("mqtt-fields: invalid SUBACK value for profile")
				}
			} else {
				r.topic("Topic Filter", true)
				if typ == 8 && r.number("Requested QoS", 1) > 2 {
					r.err = fmt.Errorf("mqtt-fields: invalid requested QoS")
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
	case 12, 13, 14:
		// These levels have no variable header or body for these types.
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	if r.at != len(wire) {
		return nil, nil, fmt.Errorf("mqtt-fields: trailing packet bytes")
	}
	info := map[string]any{
		"Profile": "MQTT exact packet fields", "Protocol Level Context": level,
		"Packet Type": uint64(typ), "Fixed Flags": uint64(flags), "Remaining Length": remaining,
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
	return r.fields, info, nil
}

func parseMQTTFields(node *base.Node, process func(*base.Node) (func(bool), error), level int) error {
	return parseCertificateFieldTree(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeMQTTFields(w, level)
	}, "mqtt-fields")
}
