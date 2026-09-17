package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// binAMQP is the M0 session state for AMQP 0-9-1. AMQP 1.0 is rejected at
// Probe and never mixed into this decoder. Ports are never consulted.
type binAMQP struct {
	client   int
	header   bool
	chans    map[uint16]*amqpChan
	pending  map[uint16][]string
	delivers map[uint64]uint16
}

type amqpChan struct {
	method        string
	waitingHeader bool
	remaining     uint64
}

var amqp091 = []byte{'A', 'M', 'Q', 'P', 0x00, 0x00, 0x09, 0x01}

func probeAMQP(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 'A' {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 8 {
		if bytes.HasPrefix(amqp091, w) {
			return probeNeed("amqp", "0-9-1", len(w), 8)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	if bytes.Equal(w[:8], amqp091) {
		_ = limit
		return probeAccept("amqp", "0-9-1", 95)
	}
	if bytes.Equal(w[:4], []byte("AMQP")) {
		// AMQP 1.0 is AMQP 0 1 0 0; other major versions are not 0-9-1.
		return ProbeResult{Verdict: ProbeReject, Protocol: "amqp", Reason: "not AMQP 0-9-1"}
	}
	return ProbeResult{Verdict: ProbeReject}
}

func (f *binFlow) frameAMQP(w []byte) (int, *binSpec, error) {
	a := f.amqp
	if a == nil {
		return 0, nil, sessionContext("AMQP session was not observed")
	}
	nchan := 0
	if a.chans != nil {
		nchan = len(a.chans)
	}
	if err := f.reserveSession(256 + int64(nchan+len(a.pending)+len(a.delivers))*32); err != nil {
		return 0, nil, err
	}
	if len(w) == 0 {
		return 0, nil, nil
	}
	if !a.header {
		if w[0] != 'A' {
			return 0, nil, fmt.Errorf("amqp: protocol header required")
		}
		if len(w) < 8 {
			return 0, nil, nil
		}
		if !bytes.Equal(w[:8], amqp091) {
			if bytes.Equal(w[:4], []byte("AMQP")) {
				return 0, nil, protocolError(ErrUnsupportedVersion, "AMQP 1.0 is not mixed into the 0-9-1 session")
			}
			return 0, nil, fmt.Errorf("amqp: protocol header magic")
		}
		return 8, f.a.specs["amqp/AMQP"], nil
	}
	if w[0] == 'A' {
		return 0, nil, fmt.Errorf("amqp: protocol header after handshake")
	}
	if len(w) < 7 {
		return 0, nil, nil
	}
	typ := w[0]
	if typ != 1 && typ != 2 && typ != 3 && typ != 8 {
		return 0, nil, fmt.Errorf("amqp: unknown frame type %d", typ)
	}
	n := 8 + int(binary.BigEndian.Uint32(w[3:7]))
	if n < 8 {
		return 0, nil, fmt.Errorf("amqp: frame length smaller than header")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	if w[n-1] != 0xce {
		return 0, nil, fmt.Errorf("amqp: frame end must be 0xce")
	}
	if typ == 1 && n < 12 {
		return 0, nil, fmt.Errorf("amqp: method frame shorter than class/method")
	}
	return n, f.a.specs["amqp/AMQP"], nil
}

func (a *binAMQP) consume(dir int, raw []byte, max int) (map[string]any, error) {
	if a.client < 0 {
		a.client = dir
	}
	if !a.header {
		if !bytes.Equal(raw, amqp091) {
			return nil, fmt.Errorf("amqp: protocol header must be AMQP 0-9-1")
		}
		a.header = true
		return map[string]any{
			"Packet Name":   "protocol-header",
			"Version":       "0-9-1",
			"Context Level": "observed",
		}, nil
	}
	if len(raw) < 8 {
		return nil, fmt.Errorf("amqp: truncated frame")
	}
	typ := raw[0]
	ch := binary.BigEndian.Uint16(raw[1:3])
	size := int(binary.BigEndian.Uint32(raw[3:7]))
	if 7+size+1 != len(raw) || raw[len(raw)-1] != 0xce {
		return nil, fmt.Errorf("amqp: framed length disagrees with payload")
	}
	payload := raw[7 : 7+size]
	info := map[string]any{
		"Frame Type":    typ,
		"Channel":       ch,
		"Context Level": "observed",
	}
	switch typ {
	case 1:
		return a.consumeMethod(ch, payload, info, max)
	case 2:
		return a.consumeHeader(ch, payload, info)
	case 3:
		return a.consumeBody(ch, payload, info)
	case 8:
		info["Packet Name"] = "heartbeat"
		if ch != 0 || size != 0 {
			return nil, fmt.Errorf("amqp: heartbeat must use channel 0 and empty payload")
		}
		return info, nil
	default:
		return nil, fmt.Errorf("amqp: unknown frame type %d", typ)
	}
}

func (a *binAMQP) consumeMethod(ch uint16, payload []byte, info map[string]any, max int) (map[string]any, error) {
	if len(payload) < 4 {
		return nil, fmt.Errorf("amqp: truncated method")
	}
	class := binary.BigEndian.Uint16(payload[:2])
	method := binary.BigEndian.Uint16(payload[2:4])
	name := amqpMethodName(class, method)
	info["Class ID"] = class
	info["Method ID"] = method
	info["Packet Name"] = name
	args := payload[4:]
	if err := amqpParseMethodArgs(class, method, args, info); err != nil {
		return nil, err
	}
	st := a.chanState(ch, max)
	if st == nil {
		return nil, protocolError(ErrResourceExceeded, "AMQP channel count exceeds budget")
	}
	switch name {
	case "connection.start", "connection.tune", "connection.open", "channel.open", "connection.close", "channel.close":
		reply := amqpOKName(name)
		if err := a.push(ch, reply, max); err != nil {
			return nil, err
		}
		info["Outstanding"] = true
	case "connection.start-ok", "connection.tune-ok", "connection.open-ok", "channel.open-ok", "connection.close-ok", "channel.close-ok":
		a.match(ch, name, info)
	case "basic.publish", "basic.deliver", "basic.return", "basic.get-ok":
		st.method = name
		st.waitingHeader = true
		st.remaining = 0
		if name == "basic.deliver" {
			if tag, ok := info["Delivery Tag"].(uint64); ok {
				a.delivers[tag] = ch
				info["Outstanding"] = true
			}
		}
	case "basic.ack", "basic.nack":
		if tag, ok := info["Delivery Tag"].(uint64); ok {
			if _, ok := a.delivers[tag]; ok {
				delete(a.delivers, tag)
				info["Matched Request"] = "basic.deliver"
				info["Association Status"] = "matched"
			} else {
				info["Unmatched"] = true
				info["Association Status"] = "missing-request"
				info["Context Level"] = "partial"
			}
		}
	}
	return info, nil
}

func (a *binAMQP) consumeHeader(ch uint16, payload []byte, info map[string]any) (map[string]any, error) {
	if len(payload) < 14 {
		return nil, fmt.Errorf("amqp: truncated content header")
	}
	class := binary.BigEndian.Uint16(payload[:2])
	weight := binary.BigEndian.Uint16(payload[2:4])
	body := binary.BigEndian.Uint64(payload[4:12])
	flags := binary.BigEndian.Uint16(payload[12:14])
	if weight != 0 {
		return nil, fmt.Errorf("amqp: content header weight must be 0")
	}
	info["Packet Name"] = "content-header"
	info["Class ID"] = class
	info["Body Size"] = body
	info["Property Flags"] = flags
	st := a.chans[ch]
	if st == nil || !st.waitingHeader {
		return nil, sessionContext("AMQP content header without a preceding content method")
	}
	st.waitingHeader = false
	st.remaining = body
	info["Content Method"] = st.method
	if body == 0 {
		st.method = ""
	}
	return info, nil
}

func (a *binAMQP) consumeBody(ch uint16, payload []byte, info map[string]any) (map[string]any, error) {
	st := a.chans[ch]
	if st == nil || st.waitingHeader || st.remaining == 0 && st.method == "" {
		return nil, sessionContext("AMQP body without a content header")
	}
	n := uint64(len(payload))
	if n > st.remaining {
		return nil, fmt.Errorf("amqp: body exceeds declared content size")
	}
	st.remaining -= n
	info["Packet Name"] = "content-body"
	info["Body Bytes"] = len(payload)
	info["Remaining Body"] = st.remaining
	info["Content Method"] = st.method
	if st.remaining == 0 {
		info["Body Complete"] = true
		st.method = ""
	}
	return info, nil
}

func (a *binAMQP) chanState(ch uint16, max int) *amqpChan {
	if a.chans == nil {
		a.chans = map[uint16]*amqpChan{}
	}
	if st, ok := a.chans[ch]; ok {
		return st
	}
	if max <= 0 {
		max = 4096
	}
	if len(a.chans) >= max {
		return nil
	}
	st := &amqpChan{}
	a.chans[ch] = st
	return st
}

func (a *binAMQP) push(ch uint16, name string, max int) error {
	if max <= 0 {
		max = 4096
	}
	n := 0
	for _, list := range a.pending {
		n += len(list)
	}
	if n >= max {
		return protocolError(ErrResourceExceeded, "AMQP outstanding methods exceed budget")
	}
	a.pending[ch] = append(a.pending[ch], name)
	return nil
}

func (a *binAMQP) match(ch uint16, name string, info map[string]any) {
	list := a.pending[ch]
	if len(list) == 0 || list[0] != name {
		info["Unmatched"] = true
		info["Association Status"] = "missing-request"
		info["Context Level"] = "partial"
		return
	}
	info["Matched Request"] = true
	info["Association Status"] = "matched"
	a.pending[ch] = list[1:]
	if len(a.pending[ch]) == 0 {
		delete(a.pending, ch)
	}
}

func amqpOKName(name string) string {
	switch name {
	case "connection.start":
		return "connection.start-ok"
	case "connection.tune":
		return "connection.tune-ok"
	case "connection.open":
		return "connection.open-ok"
	case "channel.open":
		return "channel.open-ok"
	case "connection.close":
		return "connection.close-ok"
	case "channel.close":
		return "channel.close-ok"
	}
	return name + "-ok"
}

func amqpMethodName(class, method uint16) string {
	switch class {
	case 10:
		switch method {
		case 10:
			return "connection.start"
		case 11:
			return "connection.start-ok"
		case 20:
			return "connection.secure"
		case 21:
			return "connection.secure-ok"
		case 30:
			return "connection.tune"
		case 31:
			return "connection.tune-ok"
		case 40:
			return "connection.open"
		case 41:
			return "connection.open-ok"
		case 50:
			return "connection.close"
		case 51:
			return "connection.close-ok"
		}
	case 20:
		switch method {
		case 10:
			return "channel.open"
		case 11:
			return "channel.open-ok"
		case 20:
			return "channel.flow"
		case 21:
			return "channel.flow-ok"
		case 40:
			return "channel.close"
		case 41:
			return "channel.close-ok"
		}
	case 60:
		switch method {
		case 10:
			return "basic.qos"
		case 11:
			return "basic.qos-ok"
		case 20:
			return "basic.consume"
		case 21:
			return "basic.consume-ok"
		case 40:
			return "basic.publish"
		case 50:
			return "basic.return"
		case 60:
			return "basic.deliver"
		case 70:
			return "basic.get"
		case 71:
			return "basic.get-ok"
		case 72:
			return "basic.get-empty"
		case 80:
			return "basic.ack"
		case 90:
			return "basic.reject"
		case 120:
			return "basic.nack"
		}
	}
	return fmt.Sprintf("class-%d.method-%d", class, method)
}

func amqpParseMethodArgs(class, method uint16, args []byte, info map[string]any) error {
	switch {
	case class == 10 && method == 10:
		if len(args) < 2 {
			return fmt.Errorf("amqp: truncated connection.start")
		}
		info["Version Major"] = args[0]
		info["Version Minor"] = args[1]
	case class == 60 && method == 40:
		i := 2
		ex, i, err := amqpReadShortstr(args, i)
		if err != nil {
			return err
		}
		rk, _, err := amqpReadShortstr(args, i)
		if err != nil {
			return err
		}
		info["Exchange"] = ex
		info["Routing Key"] = rk
	case class == 60 && method == 60:
		ct, i, err := amqpReadShortstr(args, 0)
		if err != nil {
			return err
		}
		if i+8 > len(args) {
			return fmt.Errorf("amqp: truncated basic.deliver")
		}
		tag := binary.BigEndian.Uint64(args[i : i+8])
		i += 8
		if i >= len(args) {
			return fmt.Errorf("amqp: truncated basic.deliver")
		}
		i++ // redelivered bit + padding
		ex, i, err := amqpReadShortstr(args, i)
		if err != nil {
			return err
		}
		rk, _, err := amqpReadShortstr(args, i)
		if err != nil {
			return err
		}
		info["Consumer Tag"] = ct
		info["Delivery Tag"] = tag
		info["Exchange"] = ex
		info["Routing Key"] = rk
	case class == 60 && (method == 80 || method == 120):
		if len(args) < 8 {
			return fmt.Errorf("amqp: truncated basic.ack/nack")
		}
		info["Delivery Tag"] = binary.BigEndian.Uint64(args[:8])
		if method == 120 {
			info["Packet Name"] = "basic.nack"
		}
	case class == 10 && method == 50:
		if len(args) < 2 {
			return fmt.Errorf("amqp: truncated connection.close")
		}
		info["Reply Code"] = binary.BigEndian.Uint16(args[:2])
		text, _, err := amqpReadShortstr(args, 2)
		if err != nil {
			return err
		}
		info["Reply Text"] = text
	}
	return nil
}

func amqpReadShortstr(b []byte, i int) (string, int, error) {
	if i >= len(b) {
		return "", i, fmt.Errorf("amqp: truncated shortstr")
	}
	n := int(b[i])
	i++
	if i+n > len(b) {
		return "", i, fmt.Errorf("amqp: truncated shortstr")
	}
	return string(b[i : i+n]), i + n, nil
}
