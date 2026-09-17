package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binQUIC is the M0 session state for RFC 9000 QUIC v1 transport. Port 443
// is never consulted. Protected payloads without keys are Encrypted; frames
// are parsed only from plaintext (constructed or caller-decrypted) input.
type binQUIC struct {
	dcid, scid  []byte
	initialDCID []byte
	spaces      [3]quicPNSpace
	streams     map[uint64]*quicStream
	crypto      [3][]quicRange
	state       string
	closed      bool
	keys        *quicKeyring
}

type quicPNSpace struct {
	largest uint64
	seen    []uint64
	init    bool
}

type quicStream struct {
	id     uint64
	maxOff uint64
	fin    bool
	reset  bool
	stop   bool
}

type quicRange struct {
	off, end uint64
}

const (
	quicSpaceInitial = iota
	quicSpaceHandshake
	quicSpaceApplication
)

func probeQUIC(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0]&0x80 == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 5 {
		return ProbeResult{Verdict: ProbeReject}
	}
	ver := binary.BigEndian.Uint32(w[1:5])
	if ver != 0 && ver != 1 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if ver == 1 && w[0]&0x40 == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 6 {
		return probeNeed("quic", "1", len(w), 6)
	}
	h, err := quicParseHeader(w, false)
	if err != nil {
		if quicNeedMore(err) {
			want := 6
			if h.size > want {
				want = h.size
			}
			return probeNeed("quic", "1", len(w), want)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	if ver == 1 && len(h.dcid) < 8 && len(h.scid) < 8 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("quic", "1", 90)
}

func (f *binFlow) frameQUIC(w []byte) (int, *binSpec, error) {
	q := f.quic
	if q == nil {
		return 0, nil, sessionContext("QUIC session was not observed")
	}
	if err := f.reserveSession(512 + int64(len(q.streams))*64); err != nil {
		return 0, nil, err
	}
	if len(w) == 0 {
		return 0, nil, nil
	}
	h, err := quicParseHeader(w, true)
	if err != nil {
		if quicNeedMore(err) {
			if h.size > 0 {
				if h.size > f.a.config.MaxMessageBytes {
					return f.a.config.MaxMessageBytes + 1, nil, nil
				}
				return h.size, nil, nil
			}
			return 0, nil, nil
		}
		return 0, nil, err
	}
	if h.size > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if h.size > len(w) {
		return h.size, nil, nil
	}
	return h.size, f.a.specs["application-layer.quic/QUIC"], nil
}

func (q *binQUIC) consume(dir int, raw []byte, max int) (map[string]any, error) {
	if max <= 0 {
		max = 64
	}
	h, err := quicParseHeader(raw, true)
	if err != nil {
		return nil, err
	}
	if q.state == "" {
		q.state = "initial"
	}
	if h.space == quicSpaceInitial && dir == 0 && len(q.initialDCID) == 0 && len(h.dcid) > 0 {
		q.initialDCID = append([]byte(nil), h.dcid...)
	}
	if len(h.dcid) > 0 {
		q.dcid = append([]byte(nil), h.dcid...)
	}
	if len(h.scid) > 0 {
		q.scid = append([]byte(nil), h.scid...)
	}
	info := map[string]any{
		"Packet Name":         h.typeName,
		"Version":             h.version,
		"DCID":                append([]byte(nil), h.dcid...),
		"SCID":                append([]byte(nil), h.scid...),
		"Packet Number Space": quicSpaceName(h.space),
		"Connection State":    q.state,
		"Header Form":         "long",
	}
	if !h.long {
		info["Header Form"] = "short"
	}
	if h.vn {
		info["Supported Versions"] = h.versions
		q.state = "version-negotiation"
		info["Connection State"] = q.state
		return info, nil
	}
	if h.retry {
		info["Retry"] = true
		q.state = "retry"
		info["Connection State"] = q.state
		return info, nil
	}
	payload := raw[h.payloadOff:h.size]
	if h.long && h.pnLen > 0 && h.pnLen <= len(payload) {
		frames, ferr := quicParseFrames(payload[h.pnLen:], max)
		if ferr == nil {
			truncated := quicTruncatedPN(payload[:h.pnLen])
			space := q.space(h.space)
			pn := quicDecodePN(space.largest, truncated, h.pnLen)
			info["Packet Number"] = pn
			info["Packet Number Length"] = h.pnLen
			info["Plaintext Input"] = true
			q.observePN(space, pn, info)
			return q.finishFrames(h.space, frames, info, max)
		}
	}
	tk, reason := q.keysFor(dir, h)
	if tk == nil {
		info["Encrypted"] = true
		info["Protected Payload"] = true
		info["Missing Keys"] = true
		info["Key Reason"] = reason
		return info, protocolError(ErrEncrypted, "QUIC payload is protected; keys were not provided")
	}
	var phaseKeys [2]*quicTrafficKeys
	if q.keys != nil && !h.long {
		phaseKeys = q.keys.app[dir]
	}
	plain, pn, pnLen, first, phase, uerr := quicUnprotect(raw, h, tk, phaseKeys, q.space(h.space).largest)
	if uerr != nil {
		info["Encrypted"] = true
		info["Protected Payload"] = true
		info["Header Protection"] = "failed"
		return info, protocolError(ErrEncrypted, "QUIC payload is protected; keys were not provided")
	}
	info["Decrypted"] = true
	info["Header Protection"] = "removed"
	info["Packet Number"] = pn
	info["Packet Number Length"] = pnLen
	info["Cipher"] = tk.aead
	if !h.long {
		info["Key Phase"] = phase
		_ = first
	}
	space := q.space(h.space)
	q.observePN(space, pn, info)
	frames, ferr := quicParseFrames(plain, max)
	if ferr != nil {
		info["Encrypted"] = true
		return info, protocolError(ErrEncrypted, "QUIC decrypted payload is not a valid frame sequence")
	}
	return q.finishFrames(h.space, frames, info, max)
}

func (q *binQUIC) observePN(space *quicPNSpace, pn uint64, info map[string]any) {
	if space.init {
		if quicSeen(space, pn) {
			info["Duplicate"] = true
		} else if pn > space.largest+1 {
			info["Gap"] = true
			info["Missing Count"] = uint64(pn - space.largest - 1)
		}
	}
	quicRemember(space, pn)
}

func (q *binQUIC) finishFrames(space int, frames []map[string]any, info map[string]any, max int) (map[string]any, error) {
	info["Frames"] = frames
	if err := q.applyFrames(space, frames, info, max); err != nil {
		return info, err
	}
	q.advanceState(space, frames)
	info["Connection State"] = q.state
	if q.closed {
		info["Connection Closed"] = true
	}
	return info, nil
}

func (q *binQUIC) space(id int) *quicPNSpace {
	if id < 0 || id > 2 {
		id = quicSpaceApplication
	}
	return &q.spaces[id]
}

func (q *binQUIC) applyFrames(space int, frames []map[string]any, info map[string]any, max int) error {
	for _, fr := range frames {
		name, _ := fr["Frame Type"].(string)
		switch name {
		case "CRYPTO":
			off, _ := fr["Offset"].(uint64)
			data, _ := fr["Crypto Data"].([]byte)
			end := off + uint64(len(data))
			if quicOverlap(q.crypto[space], off, end) {
				fr["Retransmission"] = true
				info["Retransmission"] = true
			}
			q.crypto[space] = append(q.crypto[space], quicRange{off, end})
			if len(q.crypto[space]) > max {
				return protocolError(ErrResourceExceeded, "QUIC CRYPTO range budget exceeded")
			}
		case "STREAM":
			sid, _ := fr["Stream ID"].(uint64)
			off, _ := fr["Offset"].(uint64)
			data, _ := fr["Stream Data"].([]byte)
			fin, _ := fr["FIN"].(bool)
			st := q.stream(sid, max)
			if st == nil {
				return protocolError(ErrResourceExceeded, "QUIC stream budget exceeded")
			}
			end := off + uint64(len(data))
			if end > st.maxOff {
				st.maxOff = end
			}
			if fin {
				st.fin = true
				fr["Stream FIN"] = true
			}
		case "RESET_STREAM":
			sid, _ := fr["Stream ID"].(uint64)
			st := q.stream(sid, max)
			if st != nil {
				st.reset = true
			}
		case "STOP_SENDING":
			sid, _ := fr["Stream ID"].(uint64)
			st := q.stream(sid, max)
			if st != nil {
				st.stop = true
			}
		case "CONNECTION_CLOSE", "CONNECTION_CLOSE_APP":
			q.closed = true
			q.state = "closed"
			info["Connection Closed"] = true
		case "HANDSHAKE_DONE":
			q.state = "established"
		case "ACK":
			largest, _ := fr["Largest Acknowledged"].(uint64)
			fr["ACK Space"] = quicSpaceName(space)
			_ = largest
		}
	}
	return nil
}

func (q *binQUIC) stream(id uint64, max int) *quicStream {
	if q.streams == nil {
		q.streams = map[uint64]*quicStream{}
	}
	st := q.streams[id]
	if st == nil {
		if len(q.streams) >= max {
			return nil
		}
		st = &quicStream{id: id}
		q.streams[id] = st
	}
	return st
}

func (q *binQUIC) advanceState(space int, frames []map[string]any) {
	if q.closed {
		q.state = "closed"
		return
	}
	switch space {
	case quicSpaceInitial:
		if q.state == "" || q.state == "initial" {
			q.state = "initial"
		}
	case quicSpaceHandshake:
		if q.state != "established" && q.state != "closed" {
			q.state = "handshake"
		}
	case quicSpaceApplication:
		if q.state != "closed" {
			for _, fr := range frames {
				if fr["Frame Type"] == "HANDSHAKE_DONE" {
					q.state = "established"
					return
				}
			}
			if q.state == "handshake" || q.state == "initial" {
				q.state = "application"
			}
		}
	}
}

type quicHdr struct {
	first      byte
	long       bool
	vn, retry  bool
	version    uint32
	typeName   string
	space      int
	dcid, scid []byte
	token      []byte
	pnLen      int
	payloadOff int
	size       int
	versions   []uint32
}

type quicMoreError struct{ n int }

func (e quicMoreError) Error() string { return "quic: need more bytes" }

func quicNeedMore(err error) bool {
	_, ok := err.(quicMoreError)
	return ok
}

func quicParseHeader(w []byte, forFrame bool) (quicHdr, error) {
	var h quicHdr
	if len(w) == 0 {
		return h, quicMoreError{1}
	}
	h.first = w[0]
	if h.first&0x80 == 0 {
		if h.first&0x40 == 0 {
			return h, fmt.Errorf("quic: short header missing fixed bit")
		}
		h.long = false
		h.typeName = "1-RTT"
		h.space = quicSpaceApplication
		h.pnLen = int(h.first&0x03) + 1
		h.payloadOff = 1
		h.size = len(w)
		if h.size < 1+h.pnLen {
			return h, quicMoreError{1 + h.pnLen}
		}
		return h, nil
	}
	h.long = true
	if len(w) < 6 {
		h.size = 6
		return h, quicMoreError{6}
	}
	h.version = binary.BigEndian.Uint32(w[1:5])
	if h.version == 1 && h.first&0x40 == 0 {
		return h, fmt.Errorf("quic: long header missing fixed bit")
	}
	off := 5
	dcil := int(w[off])
	off++
	if dcil > 20 {
		return h, fmt.Errorf("quic: DCID longer than 20")
	}
	if len(w) < off+dcil+1 {
		h.size = off + dcil + 1
		return h, quicMoreError{h.size}
	}
	h.dcid = w[off : off+dcil]
	off += dcil
	scil := int(w[off])
	off++
	if scil > 20 {
		return h, fmt.Errorf("quic: SCID longer than 20")
	}
	if len(w) < off+scil {
		h.size = off + scil
		return h, quicMoreError{h.size}
	}
	h.scid = w[off : off+scil]
	off += scil
	if h.version == 0 {
		h.vn = true
		h.typeName = "Version Negotiation"
		if (len(w)-off)%4 != 0 || len(w)-off < 4 {
			if forFrame && len(w)-off < 4 {
				h.size = off + 4
				return h, quicMoreError{h.size}
			}
			if (len(w)-off)%4 != 0 {
				h.size = off + ((len(w)-off)/4+1)*4
				return h, quicMoreError{h.size}
			}
		}
		for p := off; p+4 <= len(w); p += 4 {
			h.versions = append(h.versions, binary.BigEndian.Uint32(w[p:p+4]))
		}
		h.size = len(w)
		h.payloadOff = off
		return h, nil
	}
	if h.version != 1 {
		return h, protocolError(ErrUnsupportedVersion, "QUIC version 0x%08x is not v1", h.version)
	}
	pktType := int((h.first >> 4) & 0x03)
	h.pnLen = int(h.first&0x03) + 1
	switch pktType {
	case 0:
		h.typeName, h.space = "Initial", quicSpaceInitial
		tlen, n, err := quicVarint(w[off:])
		if err != nil {
			h.size = off + n
			return h, err
		}
		off += n
		if tlen > 20<<10 {
			return h, fmt.Errorf("quic: token too large")
		}
		if uint64(len(w)-off) < tlen {
			h.size = off + int(tlen)
			return h, quicMoreError{h.size}
		}
		h.token = w[off : off+int(tlen)]
		off += int(tlen)
	case 1:
		h.typeName, h.space = "0-RTT", quicSpaceApplication
	case 2:
		h.typeName, h.space = "Handshake", quicSpaceHandshake
	case 3:
		h.retry = true
		h.typeName = "Retry"
		if len(w)-off < 16 {
			h.size = off + 16
			return h, quicMoreError{h.size}
		}
		h.size = len(w)
		h.payloadOff = off
		return h, nil
	}
	length, n, err := quicVarint(w[off:])
	if err != nil {
		h.size = off + n
		return h, err
	}
	off += n
	h.payloadOff = off
	if length < 1 || length > 1<<20 {
		return h, fmt.Errorf("quic: invalid length")
	}
	h.size = off + int(length)
	return h, nil
}

func quicVarint(b []byte) (uint64, int, error) {
	if len(b) == 0 {
		return 0, 1, quicMoreError{1}
	}
	n := 1 << (b[0] >> 6)
	if len(b) < n {
		return 0, n, quicMoreError{n}
	}
	var v uint64
	switch n {
	case 1:
		v = uint64(b[0] & 0x3f)
	case 2:
		v = uint64(binary.BigEndian.Uint16(b[:2]) & 0x3fff)
	case 4:
		v = uint64(binary.BigEndian.Uint32(b[:4]) & 0x3fffffff)
	default:
		v = binary.BigEndian.Uint64(b[:8]) & 0x3fffffffffffffff
	}
	return v, n, nil
}

func quicPutVarint(v uint64) []byte {
	switch {
	case v <= 63:
		return []byte{byte(v)}
	case v <= 16383:
		b := []byte{0, 0}
		binary.BigEndian.PutUint16(b, uint16(v))
		b[0] |= 0x40
		return b
	case v <= 1073741823:
		b := []byte{0, 0, 0, 0}
		binary.BigEndian.PutUint32(b, uint32(v))
		b[0] |= 0x80
		return b
	default:
		b := []byte{0, 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint64(b, v)
		b[0] |= 0xc0
		return b
	}
}

func quicTruncatedPN(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}

func quicDecodePN(largest, truncated uint64, pnLen int) uint64 {
	expected := largest + 1
	win := uint64(1) << (8 * pnLen)
	hwin := win / 2
	mask := win - 1
	candidate := (expected &^ mask) | truncated
	if candidate+hwin <= expected && candidate+win < 1<<62 {
		candidate += win
	} else if candidate > expected+hwin && candidate >= win {
		candidate -= win
	}
	return candidate
}

func quicSeen(s *quicPNSpace, pn uint64) bool {
	for _, v := range s.seen {
		if v == pn {
			return true
		}
	}
	return false
}

func quicRemember(s *quicPNSpace, pn uint64) {
	if !s.init || pn > s.largest {
		s.largest = pn
	}
	s.init = true
	s.seen = append(s.seen, pn)
	if len(s.seen) > 128 {
		s.seen = s.seen[len(s.seen)-128:]
	}
}

func quicOverlap(rs []quicRange, off, end uint64) bool {
	for _, r := range rs {
		if off < r.end && end > r.off {
			return true
		}
	}
	return false
}

func quicSpaceName(id int) string {
	switch id {
	case quicSpaceInitial:
		return "initial"
	case quicSpaceHandshake:
		return "handshake"
	default:
		return "application"
	}
}

func quicParseFrames(payload []byte, max int) ([]map[string]any, error) {
	var frames []map[string]any
	off := 0
	for off < len(payload) {
		if payload[off] == 0x00 {
			start := off
			for off < len(payload) && payload[off] == 0x00 {
				off++
			}
			frames = append(frames, map[string]any{"Frame Type": "PADDING", "Length": off - start})
			continue
		}
		if len(frames) >= max {
			return nil, protocolError(ErrResourceExceeded, "QUIC frame budget exceeded")
		}
		fr, n, err := quicParseFrame(payload[off:])
		if err != nil {
			return nil, err
		}
		frames = append(frames, fr)
		off += n
	}
	return frames, nil
}

func quicParseFrame(b []byte) (map[string]any, int, error) {
	if len(b) == 0 {
		return nil, 0, fmt.Errorf("quic: truncated frame")
	}
	t := b[0]
	off := 1
	take := func() (uint64, error) {
		v, n, err := quicVarint(b[off:])
		if err != nil {
			return 0, fmt.Errorf("quic: truncated varint")
		}
		off += n
		return v, nil
	}
	bytesN := func(n uint64) ([]byte, error) {
		if n > uint64(len(b)-off) {
			return nil, fmt.Errorf("quic: truncated frame data")
		}
		p := append([]byte(nil), b[off:off+int(n)]...)
		off += int(n)
		return p, nil
	}
	switch {
	case t == 0x01:
		return map[string]any{"Frame Type": "PING"}, 1, nil
	case t == 0x02 || t == 0x03:
		largest, err := take()
		if err != nil {
			return nil, 0, err
		}
		delay, err := take()
		if err != nil {
			return nil, 0, err
		}
		count, err := take()
		if err != nil {
			return nil, 0, err
		}
		first, err := take()
		if err != nil {
			return nil, 0, err
		}
		ranges := []map[string]any{{"Gap": uint64(0), "ACK Range": first}}
		for i := uint64(0); i < count; i++ {
			gap, err := take()
			if err != nil {
				return nil, 0, err
			}
			rng, err := take()
			if err != nil {
				return nil, 0, err
			}
			ranges = append(ranges, map[string]any{"Gap": gap, "ACK Range": rng})
		}
		if t == 0x03 {
			if _, err = take(); err != nil {
				return nil, 0, err
			}
			if _, err = take(); err != nil {
				return nil, 0, err
			}
			if _, err = take(); err != nil {
				return nil, 0, err
			}
		}
		return map[string]any{
			"Frame Type":           "ACK",
			"Largest Acknowledged": largest,
			"ACK Delay":            delay,
			"ACK Range Count":      count,
			"ACK Ranges":           ranges,
		}, off, nil
	case t == 0x04:
		sid, err := take()
		if err != nil {
			return nil, 0, err
		}
		app, err := take()
		if err != nil {
			return nil, 0, err
		}
		final, err := take()
		if err != nil {
			return nil, 0, err
		}
		return map[string]any{"Frame Type": "RESET_STREAM", "Stream ID": sid, "Application Error": app, "Final Size": final}, off, nil
	case t == 0x05:
		sid, err := take()
		if err != nil {
			return nil, 0, err
		}
		app, err := take()
		if err != nil {
			return nil, 0, err
		}
		return map[string]any{"Frame Type": "STOP_SENDING", "Stream ID": sid, "Application Error": app}, off, nil
	case t == 0x06:
		offset, err := take()
		if err != nil {
			return nil, 0, err
		}
		n, err := take()
		if err != nil {
			return nil, 0, err
		}
		data, err := bytesN(n)
		if err != nil {
			return nil, 0, err
		}
		return map[string]any{"Frame Type": "CRYPTO", "Offset": offset, "Length": n, "Crypto Data": data}, off, nil
	case t >= 0x08 && t <= 0x0f:
		sid, err := take()
		if err != nil {
			return nil, 0, err
		}
		var offset uint64
		if t&0x04 != 0 {
			offset, err = take()
			if err != nil {
				return nil, 0, err
			}
		}
		var data []byte
		if t&0x02 != 0 {
			n, err := take()
			if err != nil {
				return nil, 0, err
			}
			data, err = bytesN(n)
			if err != nil {
				return nil, 0, err
			}
		} else {
			data = append([]byte(nil), b[off:]...)
			off = len(b)
		}
		return map[string]any{
			"Frame Type":  "STREAM",
			"Stream ID":   sid,
			"Offset":      offset,
			"FIN":         t&0x01 != 0,
			"Length":      uint64(len(data)),
			"Stream Data": data,
		}, off, nil
	case t == 0x1c:
		code, err := take()
		if err != nil {
			return nil, 0, err
		}
		ft, err := take()
		if err != nil {
			return nil, 0, err
		}
		n, err := take()
		if err != nil {
			return nil, 0, err
		}
		reason, err := bytesN(n)
		if err != nil {
			return nil, 0, err
		}
		return map[string]any{"Frame Type": "CONNECTION_CLOSE", "Error Code": code, "Offending Frame": ft, "Reason": string(reason)}, off, nil
	case t == 0x1d:
		code, err := take()
		if err != nil {
			return nil, 0, err
		}
		n, err := take()
		if err != nil {
			return nil, 0, err
		}
		reason, err := bytesN(n)
		if err != nil {
			return nil, 0, err
		}
		return map[string]any{"Frame Type": "CONNECTION_CLOSE_APP", "Error Code": code, "Reason": string(reason)}, off, nil
	case t == 0x1e:
		return map[string]any{"Frame Type": "HANDSHAKE_DONE"}, 1, nil
	default:
		return nil, 0, fmt.Errorf("quic: unknown frame type 0x%02x", t)
	}
}

func quicLongPacket(pktType, pnLen int, dcid, scid []byte, token []byte, pn uint64, payload []byte) []byte {
	if pnLen < 1 {
		pnLen = 1
	}
	if pnLen > 4 {
		pnLen = 4
	}
	first := byte(0xc0 | (pktType << 4) | (pnLen - 1))
	b := []byte{first, 0, 0, 0, 1, byte(len(dcid))}
	b = append(b, dcid...)
	b = append(b, byte(len(scid)))
	b = append(b, scid...)
	if pktType == 0 {
		b = append(b, quicPutVarint(uint64(len(token)))...)
		b = append(b, token...)
	}
	pnBytes := make([]byte, pnLen)
	for i := pnLen - 1; i >= 0; i-- {
		pnBytes[i] = byte(pn)
		pn >>= 8
	}
	body := append(pnBytes, payload...)
	b = append(b, quicPutVarint(uint64(len(body)))...)
	return append(b, body...)
}

func quicCryptoFrame(offset uint64, data []byte) []byte {
	b := []byte{0x06}
	b = append(b, quicPutVarint(offset)...)
	b = append(b, quicPutVarint(uint64(len(data)))...)
	return append(b, data...)
}

func quicStreamFrame(id, offset uint64, fin bool, data []byte) []byte {
	t := byte(0x0e) // STREAM with OFF and LEN
	if fin {
		t |= 0x01
	}
	b := []byte{t}
	b = append(b, quicPutVarint(id)...)
	b = append(b, quicPutVarint(offset)...)
	b = append(b, quicPutVarint(uint64(len(data)))...)
	return append(b, data...)
}

func quicAckFrame(largest uint64) []byte {
	b := []byte{0x02}
	b = append(b, quicPutVarint(largest)...)
	b = append(b, quicPutVarint(0)...)
	b = append(b, quicPutVarint(0)...)
	b = append(b, quicPutVarint(0)...)
	return b
}

func quicResetStream(id, code, final uint64) []byte {
	b := []byte{0x04}
	b = append(b, quicPutVarint(id)...)
	b = append(b, quicPutVarint(code)...)
	b = append(b, quicPutVarint(final)...)
	return b
}

func quicStopSending(id, code uint64) []byte {
	b := []byte{0x05}
	b = append(b, quicPutVarint(id)...)
	b = append(b, quicPutVarint(code)...)
	return b
}

func quicConnectionClose(code uint64, reason string) []byte {
	b := []byte{0x1c}
	b = append(b, quicPutVarint(code)...)
	b = append(b, quicPutVarint(0)...)
	b = append(b, quicPutVarint(uint64(len(reason)))...)
	return append(b, reason...)
}

func quicVersionNegotiation(dcid, scid []byte, versions ...uint32) []byte {
	b := []byte{0xc0, 0, 0, 0, 0, byte(len(dcid))}
	b = append(b, dcid...)
	b = append(b, byte(len(scid)))
	b = append(b, scid...)
	for _, v := range versions {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], v)
		b = append(b, n[:]...)
	}
	return b
}
