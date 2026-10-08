package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// TCP basic text get/gets/set and binary GET/SET only. Data blocks are binary
// octets delimited by declared byte lengths, never by searching their contents.
// References: upstream memcached doc/protocol.txt and protocols/binary/.
type memcachedPending struct {
	command string
	keys    []string
	key     []byte
	opcode  byte
}
type memcachedOpaquePending struct {
	opaque  uint32
	request memcachedPending
}
type binMemcached struct {
	clientDir     int
	format        string
	maxPending    int
	reserveMemory func(int64) error
	text          []memcachedPending
	binary        []memcachedOpaquePending
	closed        bool
}

func newBinMemcached(clientDir int, format string, maxPending int, reserveMemory func(int64) error) *binMemcached {
	return &binMemcached{clientDir: clientDir, format: format, maxPending: sessionCollectionLimit(maxPending), reserveMemory: reserveMemory}
}
func (s *binMemcached) sessionBytes() int64 {
	if s.closed {
		return 0
	}
	// Account retained slice capacity after replies, plus all owned key bytes.
	// 80 bytes conservatively covers each pending record on supported targets.
	n := int64(256 + cap(s.text)*80 + cap(s.binary)*80)
	for _, p := range s.text {
		n += int64(cap(p.keys) * 16)
		for _, k := range p.keys {
			n += int64(len(k))
		}
	}
	for _, p := range s.binary {
		n += int64(cap(p.request.key))
	}
	return n
}
func (s *binMemcached) outstanding() int { return len(s.text) + len(s.binary) }
func (s *binMemcached) close() {
	s.text, s.binary, s.reserveMemory, s.closed = nil, nil, nil, true
}
func (s *binMemcached) reserve(n int64) error {
	if s.reserveMemory != nil {
		return s.reserveMemory(n)
	}
	return nil
}
func memcachedMalformed(why string) error {
	return protocolError(ErrMalformedMessage, "Memcached: "+why)
}
func memcachedKey(k []byte) bool {
	if len(k) == 0 || len(k) > 250 {
		return false
	}
	for _, b := range k {
		if b <= 32 || b == 127 {
			return false
		}
	}
	return true
}
func memcachedUint(w []byte, bits int) (uint64, error) {
	if len(w) == 0 {
		return 0, memcachedMalformed("empty integer")
	}
	for _, b := range w {
		if b < '0' || b > '9' {
			return 0, memcachedMalformed("invalid decimal integer")
		}
	}
	n, err := strconv.ParseUint(string(w), 10, bits)
	if err != nil {
		return 0, memcachedMalformed("integer overflow")
	}
	return n, nil
}

type memcachedLine struct {
	command         string
	keys            []string
	flags           uint32
	expiration      int64
	size            int
	cas             uint64
	hasCAS, noreply bool
}

func memcachedParseLine(line []byte, maxMessage, maxElements int) (memcachedLine, error) {
	var p memcachedLine
	for _, b := range line {
		if b == 0 || b == '\r' || b == '\n' || b == 127 {
			return p, memcachedMalformed("control in command line")
		}
	}
	f := bytes.Fields(line)
	if len(f) == 0 {
		return p, memcachedMalformed("empty line")
	}
	p.command = string(f[0])
	switch p.command {
	case "get", "gets":
		if len(f) < 2 {
			return p, memcachedMalformed("retrieval requires keys")
		}
		if len(f)-1 > maxElements {
			return p, protocolError(ErrResourceExceeded, "Memcached key collection limit")
		}
		for _, k := range f[1:] {
			if !memcachedKey(k) {
				return p, memcachedMalformed("invalid text key")
			}
			p.keys = append(p.keys, string(k))
		}
	case "set", "VALUE":
		want := 5
		if p.command == "VALUE" {
			want = 4
		}
		if len(f) != want && len(f) != want+1 {
			return p, memcachedMalformed("storage/value header arity")
		}
		if !memcachedKey(f[1]) {
			return p, memcachedMalformed("invalid text key")
		}
		p.keys = []string{string(f[1])}
		flags, err := memcachedUint(f[2], 32)
		if err != nil {
			return p, err
		}
		p.flags = uint32(flags)
		sizeIdx := 3
		if p.command == "set" {
			ex, err := strconv.ParseInt(string(f[3]), 10, 64)
			if err != nil || ex < -2147483648 || ex > 4294967295 {
				return p, memcachedMalformed("expiration range")
			}
			p.expiration = ex
			sizeIdx = 4
		}
		size, err := memcachedUint(f[sizeIdx], 64)
		if err != nil {
			return p, err
		}
		if size > uint64(maxMessage) || size+uint64(len(line))+4 > uint64(maxMessage) {
			return p, protocolError(ErrResourceExceeded, "Memcached data block exceeds message limit")
		}
		p.size = int(size)
		if len(f) == want+1 {
			if p.command == "set" {
				if string(f[want]) != "noreply" {
					return p, memcachedMalformed("unsupported set suffix")
				}
				p.noreply = true
			} else {
				p.cas, err = memcachedUint(f[want], 64)
				if err != nil {
					return p, err
				}
				p.hasCAS = true
			}
		}
	case "END", "STORED", "NOT_STORED", "ERROR":
		if len(f) != 1 {
			return p, memcachedMalformed("response suffix")
		}
	case "CLIENT_ERROR", "SERVER_ERROR":
		if len(f) < 2 {
			return p, memcachedMalformed("empty error response")
		}
	default:
		return p, protocolError(ErrUnsupportedFeature, "Memcached command is outside get/gets/set profile")
	}
	return p, nil
}
func memcachedBinarySize(w []byte, maxMessage int) (int, error) {
	if len(w) < 24 {
		return 0, nil
	}
	if w[0] != 0x80 && w[0] != 0x81 {
		return 0, memcachedMalformed("binary magic")
	}
	if w[1] != 0 && w[1] != 1 {
		return 0, protocolError(ErrUnsupportedFeature, "Memcached binary opcode is outside GET/SET profile")
	}
	if w[5] != 0 {
		return 0, memcachedMalformed("binary data type")
	}
	key, extra, body := uint64(binary.BigEndian.Uint16(w[2:4])), uint64(w[4]), uint64(binary.BigEndian.Uint32(w[8:12]))
	if body+24 > uint64(maxMessage) {
		return 0, protocolError(ErrResourceExceeded, "Memcached binary body exceeds message limit")
	}
	if key > 250 || extra+key > body {
		return 0, memcachedMalformed("binary extras/key/body lengths")
	}
	request := w[0] == 0x80
	status := binary.BigEndian.Uint16(w[6:8])
	if request {
		if key == 0 {
			return 0, memcachedMalformed("binary request missing key")
		}
		if w[1] == 0 && (extra != 0 || body != key) {
			return 0, memcachedMalformed("GET request must contain only key")
		}
		if w[1] == 1 && extra != 8 {
			return 0, memcachedMalformed("SET request extras must contain flags/expiry")
		}
	} else if status != 0 {
		if key != 0 || extra != 0 {
			return 0, memcachedMalformed("binary error layout")
		}
	} else if w[1] == 0 {
		if extra != 4 || key != 0 {
			return 0, memcachedMalformed("GET response flags layout")
		}
	} else if extra != 0 || key != 0 || body != 0 {
		return 0, memcachedMalformed("SET success response must be empty")
	}
	return int(body) + 24, nil
}
func memcachedFrameSize(w []byte, maxMessage, maxElements int) (int, error) {
	if len(w) == 0 {
		return 0, nil
	}
	if w[0] == 0x80 || w[0] == 0x81 {
		return memcachedBinarySize(w, maxMessage)
	}
	pos, values := 0, 0
	for {
		i := bytes.Index(w[pos:], []byte("\r\n"))
		if i < 0 {
			if len(w) > maxMessage {
				return 0, protocolError(ErrResourceExceeded, "Memcached line exceeds message limit")
			}
			return 0, nil
		}
		line := w[pos : pos+i]
		p, err := memcachedParseLine(line, maxMessage, maxElements)
		if err != nil {
			return 0, err
		}
		end := pos + i + 2
		if p.command == "set" || p.command == "VALUE" {
			n := end + p.size + 2
			if n > maxMessage {
				return 0, protocolError(ErrResourceExceeded, "Memcached response exceeds message limit")
			}
			if n > len(w) {
				return n, nil
			}
			if !bytes.Equal(w[n-2:n], []byte("\r\n")) {
				return 0, memcachedMalformed("data block terminator")
			}
			if p.command == "set" {
				if values != 0 {
					return 0, memcachedMalformed("request embedded in retrieval response")
				}
				return n, nil
			}
			values++
			if values > maxElements {
				return 0, protocolError(ErrResourceExceeded, "Memcached returned value collection limit")
			}
			pos = n
			if pos == len(w) {
				return 0, nil
			}
			continue
		}
		if values > 0 && p.command != "END" {
			return 0, memcachedMalformed("retrieval values must end with END")
		}
		if end > maxMessage {
			return 0, protocolError(ErrResourceExceeded, "Memcached line exceeds message limit")
		}
		return end, nil
	}
}
func probeMemcached(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0] == 0x80 || w[0] == 0x81 {
		// Every observed discriminating byte must remain plausible. In
		// particular, 0x80 is also an RTP header and a valid Modbus TID byte;
		// it cannot make arbitrary short input an unconditional candidate.
		if len(w) >= 2 && w[1] != 0 && w[1] != 1 {
			return ProbeResult{Verdict: ProbeReject}
		}
		if len(w) >= 3 && w[2] != 0 {
			return ProbeResult{Verdict: ProbeReject} // key length already exceeds 250
		}
		if len(w) >= 4 {
			key := binary.BigEndian.Uint16(w[2:4])
			if key > 250 || w[0] == 0x80 && key == 0 || w[0] == 0x81 && key != 0 {
				return ProbeResult{Verdict: ProbeReject}
			}
		}
		if len(w) >= 5 && w[0] == 0x80 && (w[1] == 0 && w[4] != 0 || w[1] == 1 && w[4] != 8) {
			return ProbeResult{Verdict: ProbeReject}
		}
		if len(w) >= 5 && w[0] == 0x81 && (w[1] == 1 && w[4] != 0 || w[1] == 0 && w[4] != 0 && w[4] != 4) {
			return ProbeResult{Verdict: ProbeReject}
		}
		if len(w) >= 6 && w[5] != 0 {
			return ProbeResult{Verdict: ProbeReject}
		}
		if len(w) >= 9 && w[8] != 0 {
			return ProbeResult{Verdict: ProbeReject} // body cannot fit the bounded profile
		}
		if w[0] == 0x80 && len(w) >= 2 && w[1] == 0 {
			for i := 9; i < min(len(w), 11); i++ {
				if w[i] != 0 {
					return ProbeResult{Verdict: ProbeReject} // GET body is just its <=250-byte key
				}
			}
		}
		if len(w) >= 12 {
			// The complete layout is known before opaque/CAS arrive. Their
			// bytes are unconstrained; use zeros only for this shape check.
			var header [24]byte
			copy(header[:], w[:12])
			if _, err := memcachedBinarySize(header[:], 16<<20); err != nil {
				return ProbeResult{Verdict: ProbeReject}
			}
		}
		if len(w) < 24 {
			return probeNeed("memcached", "binary-get-set", len(w), 24)
		}
		if _, err := memcachedBinarySize(w, 16<<20); err != nil {
			return ProbeResult{Verdict: ProbeReject}
		}
		return probeAccept("memcached", "binary-get-set", 97)
	}
	// Reject HTTP's uppercase GET and admit only a validated, complete header.
	candidate := false
	for _, prefix := range []string{"get ", "gets ", "set ", "VALUE "} {
		if bytes.HasPrefix(w, []byte(prefix)) || bytes.HasPrefix([]byte(prefix), w) {
			candidate = true
			break
		}
	}
	if !candidate {
		return ProbeResult{Verdict: ProbeReject}
	}
	i := bytes.Index(w, []byte("\r\n"))
	if i < 0 {
		return probeNeed("memcached", "text-get-set", len(w), limit)
	}
	if _, err := memcachedParseLine(w[:i], 16<<20, 4096); err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("memcached", "text-get-set", 95)
}
func (s *binMemcached) consume(dir int, raw []byte) (map[string]any, error) {
	if s.closed {
		return nil, protocolError(ErrFatalSessionError, "Memcached session is closed")
	}
	if dir != 0 && dir != 1 {
		return nil, memcachedMalformed("invalid direction")
	}
	n, err := memcachedFrameSize(raw, len(raw), s.maxPending)
	if err != nil {
		return nil, err
	}
	if n != len(raw) || n == 0 {
		return nil, memcachedMalformed("message boundary")
	}
	binaryWire := raw[0] == 0x80 || raw[0] == 0x81
	if binaryWire != (s.format == "binary-get-set") {
		return nil, memcachedMalformed("wire format changed within connection")
	}
	if binaryWire {
		return s.consumeBinary(dir, raw)
	}
	return s.consumeText(dir, raw)
}
func (s *binMemcached) consumeBinary(dir int, raw []byte) (map[string]any, error) {
	request := raw[0] == 0x80
	if request != (dir == s.clientDir) {
		return nil, memcachedMalformed("binary request/response direction")
	}
	op := raw[1]
	name := "GET"
	if op == 1 {
		name = "SET"
	}
	opaque := binary.BigEndian.Uint32(raw[12:16])
	extra, key := int(raw[4]), int(binary.BigEndian.Uint16(raw[2:4]))
	body := raw[24:]
	value := body[extra+key:]
	// Retain the existing binary GET rule's field names and uint64 wire types.
	out := map[string]any{
		"Packet Name": name, "Command": name, "Wire Format": "binary",
		"Magic": uint64(raw[0]), "Opcode": uint64(op), "Key Length": uint64(key),
		"Extras Length": uint64(extra), "Data Type": uint64(raw[5]),
		"Total Body Length": uint64(len(body)), "Opaque": uint64(opaque),
		"CAS": binary.BigEndian.Uint64(raw[16:24]), "Body Length": len(body),
	}
	if request {
		if s.findOpaque(opaque) >= 0 {
			return nil, protocolError(ErrDesynchronized, "Memcached duplicate outstanding opaque")
		}
		if s.outstanding() >= s.maxPending {
			return nil, protocolError(ErrResourceExceeded, "Memcached pending request limit")
		}
		capacity := cap(s.binary)
		if len(s.binary) == capacity {
			capacity = min(max(1, capacity*2), s.maxPending)
		}
		if err := s.reserve(s.sessionBytes() + int64(capacity-cap(s.binary))*80 + int64(key)); err != nil {
			return nil, err
		}
		owned := make([]byte, key)
		copy(owned, body[extra:extra+key])
		if len(s.binary) == cap(s.binary) {
			grown := make([]memcachedOpaquePending, len(s.binary), capacity)
			copy(grown, s.binary)
			s.binary = grown
		}
		s.binary = append(s.binary, memcachedOpaquePending{opaque: opaque, request: memcachedPending{command: name, key: owned, opcode: op}})
		out["Role"], out["Key"] = "request", bytes.Clone(owned)
		out["VBucket ID"] = uint64(binary.BigEndian.Uint16(raw[6:8]))
		if op == 1 {
			out["Flags"] = binary.BigEndian.Uint32(body[:4])
			out["Expiration"] = binary.BigEndian.Uint32(body[4:8])
			out["Value"] = bytes.Clone(value)
		}
	} else {
		status := binary.BigEndian.Uint16(raw[6:8])
		out["Role"], out["Status"] = "response", int(status)
		idx := s.findOpaque(opaque)
		matched := idx >= 0
		var req memcachedPending
		if matched {
			req = s.binary[idx].request
		}
		if matched && req.opcode != op {
			return nil, memcachedMalformed("response opcode differs from outstanding opaque")
		}
		out["Matched"] = matched
		if matched {
			out["In Reply To"] = req.command
			out["Key"] = bytes.Clone(req.key)
			copy(s.binary[idx:], s.binary[idx+1:])
			s.binary[len(s.binary)-1] = memcachedOpaquePending{}
			s.binary = s.binary[:len(s.binary)-1]
		} else {
			out["Association"] = "missing-request"
		}
		if status != 0 {
			out["Error Message"] = bytes.Clone(value)
		} else if op == 0 {
			out["Flags"] = binary.BigEndian.Uint32(body[:4])
			out["Value"] = bytes.Clone(value)
		}
	}
	out["Pending"] = s.outstanding()
	return out, nil
}
func (s *binMemcached) consumeText(dir int, raw []byte) (map[string]any, error) {
	i := bytes.Index(raw, []byte("\r\n"))
	p, err := memcachedParseLine(raw[:i], len(raw), s.maxPending)
	if err != nil {
		return nil, err
	}
	request := p.command == "set" || p.command == "get" || p.command == "gets"
	if request != (dir == s.clientDir) {
		return nil, memcachedMalformed("text request/response direction")
	}
	out := map[string]any{"Packet Name": p.command, "Command": p.command, "Wire Format": "text"}
	if request {
		out["Role"], out["Keys"] = "request", append([]string(nil), p.keys...)
		if p.command == "set" {
			out["Key"], out["Flags"], out["Expiration"], out["Bytes"], out["No Reply"] = p.keys[0], p.flags, p.expiration, p.size, p.noreply
			out["Value"] = bytes.Clone(raw[i+2 : i+2+p.size])
		}
		if !p.noreply {
			if s.outstanding() >= s.maxPending {
				return nil, protocolError(ErrResourceExceeded, "Memcached pending request limit")
			}
			additional := int64(len(p.keys) * 16)
			for _, k := range p.keys {
				additional += int64(len(k))
			}
			capacity := cap(s.text)
			if len(s.text) == capacity {
				capacity = max(1, capacity*2)
				capacity = min(capacity, s.maxPending)
				additional += int64(capacity-cap(s.text)) * 80
			}
			if err := s.reserve(s.sessionBytes() + additional); err != nil {
				return nil, err
			}
			keys := make([]string, len(p.keys))
			for i, k := range p.keys {
				keys[i] = strings.Clone(k)
			}
			if len(s.text) == cap(s.text) {
				grown := make([]memcachedPending, len(s.text), capacity)
				copy(grown, s.text)
				s.text = grown
			}
			s.text = append(s.text, memcachedPending{command: p.command, keys: keys})
		}
	} else {
		out["Role"] = "response"
		matched := len(s.text) > 0
		out["Matched"] = matched
		var req memcachedPending
		if matched {
			req = s.text[0]
			out["In Reply To"] = req.command
		} else {
			out["Association"] = "missing-request"
		}
		if p.command == "STORED" || p.command == "NOT_STORED" {
			if matched && req.command != "set" {
				return nil, memcachedMalformed("storage reply does not match request")
			}
		} else if p.command == "END" || p.command == "VALUE" {
			if matched && req.command != "get" && req.command != "gets" {
				return nil, memcachedMalformed("retrieval reply does not match request")
			}
			values := []map[string]any{}
			seen := map[string]int{}
			requested := map[string]int{}
			for _, key := range req.keys {
				requested[key]++
			}
			pos := 0
			for pos < len(raw) {
				j := bytes.Index(raw[pos:], []byte("\r\n"))
				item, err := memcachedParseLine(raw[pos:pos+j], len(raw), s.maxPending)
				if err != nil {
					return nil, err
				}
				if item.command == "END" {
					break
				}
				seen[item.keys[0]]++
				if matched {
					// The server returns an item for each requested key token,
					// including repeated tokens. A reply cannot exceed the
					// multiplicity of its observed request.
					if seen[item.keys[0]] > requested[item.keys[0]] || item.hasCAS != (req.command == "gets") {
						return nil, memcachedMalformed("returned key/CAS differs from retrieval request")
					}
				}
				start := pos + j + 2
				v := map[string]any{"Key": item.keys[0], "Flags": item.flags, "Bytes": item.size, "Value": bytes.Clone(raw[start : start+item.size])}
				if item.hasCAS {
					v["CAS"] = item.cas
				}
				values = append(values, v)
				pos = start + item.size + 2
			}
			out["Packet Name"], out["Values"], out["Value Count"] = "Retrieval", values, len(values)
		} else {
			out["Error Message"] = string(raw[:i])
		}
		if matched {
			copy(s.text, s.text[1:])
			s.text[len(s.text)-1] = memcachedPending{}
			s.text = s.text[:len(s.text)-1]
		}
	}
	out["Pending"] = s.outstanding()
	return out, nil
}
func (s *binMemcached) findOpaque(opaque uint32) int {
	for i, p := range s.binary {
		if p.opaque == opaque {
			return i
		}
	}
	return -1
}
func (s *binMemcached) String() string {
	return fmt.Sprintf("Memcached %s pending=%d", s.format, s.outstanding())
}
