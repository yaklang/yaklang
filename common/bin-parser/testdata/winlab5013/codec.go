package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type hdr struct{ K, V string }

func httpRaw(start string, headers []hdr, body []byte) []byte {
	var b strings.Builder
	b.WriteString(start)
	if !strings.HasSuffix(start, "\r\n") {
		b.WriteString("\r\n")
	}
	hasCL := false
	hasConn := false
	for _, h := range headers {
		fmt.Fprintf(&b, "%s: %s\r\n", h.K, h.V)
		if strings.EqualFold(h.K, "Content-Length") {
			hasCL = true
		}
		if strings.EqualFold(h.K, "Connection") {
			hasConn = true
		}
	}
	if !hasCL {
		fmt.Fprintf(&b, "Content-Length: %d\r\n", len(body))
	}
	if !hasConn {
		b.WriteString("Connection: keep-alive\r\n")
	}
	b.WriteString("\r\n")
	b.Write(body)
	return []byte(b.String())
}

type httpMessage struct {
	Start   string
	Headers map[string]string
	Body    []byte
}

func readAllHTTP(buf []byte) ([]httpMessage, error) {
	var out []httpMessage
	for len(buf) > 0 {
		msg, rest, err := readOneHTTP(buf)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
		buf = rest
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no http message")
	}
	return out, nil
}

func readOneHTTP(buf []byte) (httpMessage, []byte, error) {
	idx := strings.Index(string(buf), "\r\n\r\n")
	if idx < 0 {
		return httpMessage{}, nil, fmt.Errorf("http header not terminated")
	}
	head := string(buf[:idx])
	rest := buf[idx+4:]
	lines := strings.Split(head, "\r\n")
	if len(lines) == 0 || lines[0] == "" {
		return httpMessage{}, nil, fmt.Errorf("empty http start")
	}
	h := map[string]string{}
	for _, ln := range lines[1:] {
		k, v, ok := strings.Cut(ln, ":")
		if !ok {
			return httpMessage{}, nil, fmt.Errorf("bad http header %q", ln)
		}
		h[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	n, err := strconv.Atoi(h["content-length"])
	if err != nil {
		return httpMessage{}, nil, fmt.Errorf("missing content-length")
	}
	if n < 0 || n > len(rest) {
		return httpMessage{}, nil, fmt.Errorf("http body truncated")
	}
	return httpMessage{Start: lines[0], Headers: h, Body: append([]byte{}, rest[:n]...)}, rest[n:], nil
}

func headerValue(msgs []httpMessage, key, contains string) string {
	key = strings.ToLower(key)
	for _, m := range msgs {
		if strings.Contains(strings.ToLower(m.Headers[key]), strings.ToLower(contains)) || m.Headers[key] == contains {
			return m.Headers[key]
		}
	}
	return ""
}

func dnsEncodeName(name string) []byte {
	var b []byte
	if name != "" && name != "." {
		for _, p := range strings.Split(name, ".") {
			b = append(b, byte(len(p)))
			b = append(b, p...)
		}
	}
	return append(b, 0)
}

func dnsQuery(id uint16, name string, qtype uint16) []byte {
	q := dnsEncodeName(name)
	b := make([]byte, 12+len(q)+4)
	binary.BigEndian.PutUint16(b[0:2], id)
	binary.BigEndian.PutUint16(b[2:4], 0x0100)
	binary.BigEndian.PutUint16(b[4:6], 1)
	copy(b[12:], q)
	off := 12 + len(q)
	binary.BigEndian.PutUint16(b[off:off+2], qtype)
	binary.BigEndian.PutUint16(b[off+2:off+4], 1)
	return b
}

func dnsResponse(id uint16, name string, qtype uint16, rtype uint16, rdata []byte) []byte {
	q := dnsEncodeName(name)
	an := dnsEncodeName(name)
	b := make([]byte, 12+len(q)+4+len(an)+10+len(rdata))
	binary.BigEndian.PutUint16(b[0:2], id)
	binary.BigEndian.PutUint16(b[2:4], 0x8180)
	binary.BigEndian.PutUint16(b[4:6], 1)
	binary.BigEndian.PutUint16(b[6:8], 1)
	copy(b[12:], q)
	off := 12 + len(q)
	binary.BigEndian.PutUint16(b[off:off+2], qtype)
	binary.BigEndian.PutUint16(b[off+2:off+4], 1)
	off += 4
	copy(b[off:], an)
	off += len(an)
	binary.BigEndian.PutUint16(b[off:off+2], rtype)
	binary.BigEndian.PutUint16(b[off+2:off+4], 1)
	binary.BigEndian.PutUint32(b[off+4:off+8], 60)
	binary.BigEndian.PutUint16(b[off+8:off+10], uint16(len(rdata)))
	copy(b[off+10:], rdata)
	return b
}

func dnsTXT(s string) []byte { return append([]byte{byte(len(s))}, s...) }

type dnsRR struct {
	Name  string
	Type  uint16
	Rdata []byte
}

type dnsMsg struct {
	ID int
	Q  []dnsRR
	A  []dnsRR
}

func parseDNS(p []byte) (dnsMsg, error) {
	if len(p) < 12 {
		return dnsMsg{}, fmt.Errorf("short dns")
	}
	var m dnsMsg
	m.ID = int(binary.BigEndian.Uint16(p[0:2]))
	qd := int(binary.BigEndian.Uint16(p[4:6]))
	an := int(binary.BigEndian.Uint16(p[6:8]))
	off := 12
	var err error
	for i := 0; i < qd; i++ {
		var name string
		name, off, err = dnsReadName(p, off)
		if err != nil {
			return dnsMsg{}, err
		}
		if off+4 > len(p) {
			return dnsMsg{}, fmt.Errorf("short dns question")
		}
		typ := binary.BigEndian.Uint16(p[off : off+2])
		off += 4
		m.Q = append(m.Q, dnsRR{Name: name, Type: typ})
	}
	for i := 0; i < an; i++ {
		var name string
		name, off, err = dnsReadName(p, off)
		if err != nil {
			return dnsMsg{}, err
		}
		if off+10 > len(p) {
			return dnsMsg{}, fmt.Errorf("short dns rr")
		}
		typ := binary.BigEndian.Uint16(p[off : off+2])
		rdlen := int(binary.BigEndian.Uint16(p[off+8 : off+10]))
		off += 10
		if off+rdlen > len(p) {
			return dnsMsg{}, fmt.Errorf("dns rdata truncated")
		}
		m.A = append(m.A, dnsRR{Name: name, Type: typ, Rdata: append([]byte{}, p[off:off+rdlen]...)})
		off += rdlen
	}
	return m, nil
}

func dnsReadName(p []byte, off int) (string, int, error) {
	var labels []string
	hops := 0
	end := -1
	for {
		if off >= len(p) {
			return "", 0, fmt.Errorf("dns name truncated")
		}
		l := int(p[off])
		if l == 0 {
			off++
			break
		}
		if l&0xC0 == 0xC0 {
			if off+1 >= len(p) {
				return "", 0, fmt.Errorf("dns pointer truncated")
			}
			ptr := int(binary.BigEndian.Uint16(p[off:off+2]) & 0x3FFF)
			if end < 0 {
				end = off + 2
			}
			off = ptr
			hops++
			if hops > 8 {
				return "", 0, fmt.Errorf("dns pointer loop")
			}
			continue
		}
		if l&0xC0 != 0 {
			return "", 0, fmt.Errorf("bad dns label")
		}
		off++
		if off+l > len(p) {
			return "", 0, fmt.Errorf("dns label truncated")
		}
		labels = append(labels, string(p[off:off+l]))
		off += l
	}
	if end < 0 {
		end = off
	}
	return strings.Join(labels, "."), end, nil
}

func txtRdata(rdata []byte) (string, error) {
	if len(rdata) < 1 || int(rdata[0]) != len(rdata)-1 {
		return "", fmt.Errorf("bad txt rdata")
	}
	return string(rdata[1:]), nil
}

func put16(b []byte, v uint16) []byte {
	var x [2]byte
	binary.BigEndian.PutUint16(x[:], v)
	return append(b, x[:]...)
}

func put32(b []byte, v uint32) []byte {
	var x [4]byte
	binary.BigEndian.PutUint32(x[:], v)
	return append(b, x[:]...)
}

func u16(b []byte) uint16 { return binary.BigEndian.Uint16(b) }
func u32(b []byte) uint32 { return binary.BigEndian.Uint32(b) }

func appendU16(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

func chVarint(v uint64) []byte {
	var b []byte
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func chString(s string) []byte {
	return append(chVarint(uint64(len(s))), s...)
}

type rd struct {
	b []byte
	i int
}

func (r *rd) remain() int { return len(r.b) - r.i }

func (r *rd) bytes(n int) ([]byte, error) {
	if n < 0 || r.i+n > len(r.b) {
		return nil, fmt.Errorf("short read")
	}
	p := r.b[r.i : r.i+n]
	r.i += n
	return p, nil
}

func (r *rd) u8() (byte, error) {
	p, err := r.bytes(1)
	if err != nil {
		return 0, err
	}
	return p[0], nil
}

func (r *rd) be16() (uint16, error) {
	p, err := r.bytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(p), nil
}

func (r *rd) be32() (uint32, error) {
	p, err := r.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(p), nil
}

func (r *rd) be64() (uint64, error) {
	p, err := r.bytes(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(p), nil
}

func (r *rd) varint() (uint64, error) {
	var x uint64
	var s uint
	for {
		b, err := r.u8()
		if err != nil {
			return 0, err
		}
		x |= uint64(b&0x7f) << s
		if b&0x80 == 0 {
			return x, nil
		}
		s += 7
		if s > 63 {
			return 0, fmt.Errorf("varint overflow")
		}
	}
}

func (r *rd) chString() (string, error) {
	n, err := r.varint()
	if err != nil {
		return "", err
	}
	if n > uint64(r.remain()) {
		return "", fmt.Errorf("ch string truncated")
	}
	p, err := r.bytes(int(n))
	if err != nil {
		return "", err
	}
	return string(p), nil
}

func (r *rd) zkBytes() ([]byte, error) {
	n, err := r.be32()
	if err != nil {
		return nil, err
	}
	if n > 1<<20 {
		return nil, fmt.Errorf("zk bytes too long")
	}
	return r.bytes(int(n))
}

func (r *rd) zkString() (string, error) {
	b, err := r.zkBytes()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func zkBytes(s string) []byte {
	b := make([]byte, 4+len(s))
	binary.BigEndian.PutUint32(b, uint32(len(s)))
	copy(b[4:], s)
	return b
}

func bencStr(s string) string { return strconv.Itoa(len(s)) + ":" + s }
func bencInt(n int) string    { return "i" + strconv.Itoa(n) + "e" }

func bencDict(kv [][2]string) string {
	// kv values are already bencoded. Keys are sorted by caller.
	var b strings.Builder
	b.WriteByte('d')
	for _, p := range kv {
		b.WriteString(bencStr(p[0]))
		b.WriteString(p[1])
	}
	b.WriteByte('e')
	return b.String()
}

func bdecode(s string) (any, string, error) {
	if s == "" {
		return nil, "", fmt.Errorf("empty bencode")
	}
	switch s[0] {
	case 'i':
		end := strings.IndexByte(s, 'e')
		if end < 0 {
			return nil, "", fmt.Errorf("bad bencode int")
		}
		n, err := strconv.Atoi(s[1:end])
		if err != nil {
			return nil, "", err
		}
		return n, s[end+1:], nil
	case 'l':
		rest := s[1:]
		var list []any
		for len(rest) > 0 && rest[0] != 'e' {
			var v any
			var err error
			v, rest, err = bdecode(rest)
			if err != nil {
				return nil, "", err
			}
			list = append(list, v)
		}
		if len(rest) == 0 || rest[0] != 'e' {
			return nil, "", fmt.Errorf("bad bencode list")
		}
		return list, rest[1:], nil
	case 'd':
		rest := s[1:]
		m := map[string]any{}
		var order []string
		for len(rest) > 0 && rest[0] != 'e' {
			k, next, err := bdecode(rest)
			if err != nil {
				return nil, "", err
			}
			ks, ok := k.(string)
			if !ok {
				return nil, "", fmt.Errorf("bencode key not string")
			}
			var v any
			v, rest, err = bdecode(next)
			if err != nil {
				return nil, "", err
			}
			m[ks] = v
			order = append(order, ks)
			_ = order
		}
		if len(rest) == 0 || rest[0] != 'e' {
			return nil, "", fmt.Errorf("bad bencode dict")
		}
		return m, rest[1:], nil
	default:
		colon := strings.IndexByte(s, ':')
		if colon < 0 {
			return nil, "", fmt.Errorf("bad bencode string")
		}
		n, err := strconv.Atoi(s[:colon])
		if err != nil || n < 0 || colon+1+n > len(s) {
			return nil, "", fmt.Errorf("bad bencode string length")
		}
		return s[colon+1 : colon+1+n], s[colon+1+n:], nil
	}
}

func mpFixStr(s string) []byte {
	if len(s) > 31 {
		b := []byte{0xd9, byte(len(s))}
		return append(b, s...)
	}
	return append([]byte{0xa0 | byte(len(s))}, s...)
}

func mpArray(n int, parts ...[]byte) []byte {
	b := []byte{0x90 | byte(n)}
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

type mpDec struct {
	b []byte
	i int
}

func (m *mpDec) val() (any, error) {
	if m.i >= len(m.b) {
		return nil, fmt.Errorf("msgpack truncated")
	}
	c := m.b[m.i]
	m.i++
	switch {
	case c <= 0x7f:
		return int(c), nil
	case c&0xf0 == 0x80:
		return nil, fmt.Errorf("unexpected fixmap")
	case c&0xf0 == 0x90:
		n := int(c & 0x0f)
		arr := make([]any, n)
		for i := 0; i < n; i++ {
			v, err := m.val()
			if err != nil {
				return nil, err
			}
			arr[i] = v
		}
		return arr, nil
	case c&0xe0 == 0xa0:
		n := int(c & 0x1f)
		if m.i+n > len(m.b) {
			return nil, fmt.Errorf("msgpack str truncated")
		}
		s := string(m.b[m.i : m.i+n])
		m.i += n
		return s, nil
	case c == 0xc0:
		return nil, nil
	case c == 0xc2:
		return false, nil
	case c == 0xc3:
		return true, nil
	case c == 0xd9:
		if m.i >= len(m.b) {
			return nil, fmt.Errorf("msgpack str8 truncated")
		}
		n := int(m.b[m.i])
		m.i++
		if m.i+n > len(m.b) {
			return nil, fmt.Errorf("msgpack str8 truncated")
		}
		s := string(m.b[m.i : m.i+n])
		m.i += n
		return s, nil
	default:
		return nil, fmt.Errorf("unsupported msgpack %02x", c)
	}
}

func mpAll(b []byte) ([]any, error) {
	d := mpDec{b: b}
	var out []any
	for d.i < len(d.b) {
		v, err := d.val()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func f32be(v float32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(v))
	return b
}

func f32le(v float32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, math.Float32bits(v))
	return b
}

func crc16CCITT(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func xmlText(body []byte, tag string) (string, error) {
	s := string(body)
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(s, open)
	if i < 0 {
		// allow optional prefix inside tag name with namespace already in tag argument
		return "", fmt.Errorf("missing <%s>", tag)
	}
	j := strings.Index(s[i+len(open):], close)
	if j < 0 {
		return "", fmt.Errorf("unclosed <%s>", tag)
	}
	return s[i+len(open) : i+len(open)+j], nil
}

func mustContain(s, sub, what string) error {
	if !strings.Contains(s, sub) {
		return fmt.Errorf("%s missing %q", what, sub)
	}
	return nil
}
