package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"net/netip"
	"time"
)

// RFC 8489/8656. Integrity is reported as unverified without credential keys.
// Transaction and relay state describes observations, not authenticated grants.
type stunKey struct {
	dir int
	id  [12]byte
}
type turnChannelKey struct {
	dir     int
	channel uint16
}
type turnPeerKey struct {
	dir int
	ip  netip.Addr
}
type stunRequest struct {
	method      uint16
	peer        netip.AddrPort
	peers       []netip.AddrPort
	hash        [32]byte
	hasLifetime bool
	channel     uint16
	lifetime    uint32
	expires     time.Time
}
type turnChannel struct {
	peer    netip.AddrPort
	expires time.Time
}
type binSTUN struct {
	pending     map[stunKey]stunRequest
	history     map[stunKey]bool
	blocked     bool
	channels    map[turnChannelKey]turnChannel
	permissions map[turnPeerKey]time.Time
	allocations [2]time.Time
	clock       time.Time
}

func probeSTUN(w []byte, _ int) ProbeResult {
	if len(w) >= 2 && len(w) < 8 && w[0]&0xc0 == 0 {
		method := stunMethod(binary.BigEndian.Uint16(w))
		known := method == 1 || method == 3 || method == 4 || method == 6 || method == 7 || method == 8 || method == 9
		cookie := []byte{0x21, 0x12, 0xa4, 0x42}
		if len(w) >= 4 && binary.BigEndian.Uint16(w[2:4])%4 != 0 {
			known = false
		}
		for i := 4; i < len(w); i++ {
			if w[i] != cookie[i-4] {
				known = false
			}
		}
		if known {
			return probeNeed("stun", "8489", len(w), 8)
		}
	}

	if len(w) < 8 || w[0]&0xc0 != 0 || binary.BigEndian.Uint32(w[4:8]) != 0x2112a442 || binary.BigEndian.Uint16(w[2:4])%4 != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("stun", "8489", 99)
}
func stunFrameLength(w []byte, stream bool) (int, error) {
	if len(w) < 4 {
		return 0, nil
	}
	if w[0]&0xc0 == 0x40 {
		channel := binary.BigEndian.Uint16(w[:2])
		if channel < 0x4000 || channel > 0x7fff {
			return 0, protocolError(ErrMalformedMessage, "turn: invalid channel number")
		}
		n := 4 + int(binary.BigEndian.Uint16(w[2:4]))
		if stream {
			n = (n + 3) &^ 3
		}
		return n, nil
	}
	if len(w) < 20 {
		return 0, nil
	}
	if w[0]&0xc0 != 0 || binary.BigEndian.Uint32(w[4:8]) != 0x2112a442 || binary.BigEndian.Uint16(w[2:4])%4 != 0 {
		return 0, fmt.Errorf("stun: invalid header")
	}
	return 20 + int(binary.BigEndian.Uint16(w[2:4])), nil
}
func (f *binFlow) frameSTUN(w []byte) (int, *binSpec, error) {
	s := f.stun
	if err := f.reserveSession(s.bytes() + int64(len(w))*8 + 256); err != nil {
		s.invalidate()
		return 0, nil, err
	}
	n, err := stunFrameLength(w, true)
	if err != nil {
		s.invalidate()
	}
	entry := "STUN"
	if len(w) > 0 && w[0]&0xc0 == 0x40 {
		entry = "ChannelData"
	}
	return n, f.a.specs["stun_session/"+entry], err
}
func (s *binSTUN) bytes() int64 {
	n := int64(512 + len(s.pending)*256 + len(s.history)*128 + len(s.channels)*96 + len(s.permissions)*80)
	for _, p := range s.pending {
		n += int64(cap(p.peers)) * 32
	}
	return n
}
func stunMethod(t uint16) uint16 { return t&15 | (t&0xe0)>>1 | (t&0x3e00)>>2 }
func stunName(m uint16) string {
	switch m {
	case 1:
		return "Binding"
	case 3:
		return "Allocate"
	case 4:
		return "Refresh"
	case 6:
		return "Send"
	case 7:
		return "Data"
	case 8:
		return "CreatePermission"
	case 9:
		return "ChannelBind"
	}
	return fmt.Sprintf("Method-%d", m)
}
func (s *binSTUN) expire(ts time.Time) {
	if ts.After(s.clock) {
		s.clock = ts
	}
	ts = s.clock
	for k, v := range s.pending {
		if !ts.Before(v.expires) {
			s.retire(k)
		}
	}
	for k, v := range s.channels {
		if !ts.Before(v.expires) {
			delete(s.channels, k)
		}
	}
	for k, v := range s.permissions {
		if !ts.Before(v) {
			delete(s.permissions, k)
		}
	}
	for dir, v := range s.allocations {
		if !v.IsZero() && !ts.Before(v) {
			s.clearAllocation(dir)
		}
	}
}

// Wire IDs are not new generations when a local timer fires. Retain only
// bounded hashes/tombstones; ambiguous observations never grant TURN state.
func (s *binSTUN) retire(k stunKey) {
	if _, ok := s.history[k]; ok {
		s.history[k] = true
	}
	delete(s.pending, k)
}
func (s *binSTUN) invalidate() {
	for k := range s.pending {
		s.retire(k)
	}
	s.blocked = true
	s.clearAllocation(0)
	s.clearAllocation(1)
}
func (s *binSTUN) clearAllocation(dir int) {
	s.allocations[dir] = time.Time{}
	for k := range s.channels {
		if k.dir == dir {
			delete(s.channels, k)
		}
	}
	for k := range s.permissions {
		if k.dir == dir {
			delete(s.permissions, k)
		}
	}
}
func stunAddress(v, w []byte, xor bool) (netip.AddrPort, error) {
	if len(v) < 4 || v[0] != 0 {
		return netip.AddrPort{}, fmt.Errorf("stun: invalid address attribute")
	}
	n := 4
	if v[1] == 2 {
		n = 16
	} else if v[1] != 1 {
		return netip.AddrPort{}, fmt.Errorf("stun: address family")
	}
	if len(v) != 4+n {
		return netip.AddrPort{}, fmt.Errorf("stun: address length")
	}
	port := binary.BigEndian.Uint16(v[2:4])
	b := append([]byte(nil), v[4:]...)
	if xor {
		port ^= 0x2112
		for i := range b {
			b[i] ^= w[4+i]
		}
	}
	ip, _ := netip.AddrFromSlice(b)
	return netip.AddrPortFrom(ip, port), nil
}
func (s *binSTUN) consume(dir int, ts time.Time, w []byte, max int, stream bool) (_ map[string]any, consumeErr error) {
	keepContext := false
	defer func() {
		var typed *ProtocolError
		if consumeErr != nil && !errors.As(consumeErr, &typed) {
			// Keep a semantic refusal typed through event queries; an unchecked
			// structural rule must not turn a failed fingerprint into success.
			consumeErr = protocolError(ErrMalformedMessage, "%s", consumeErr)
		}
		if consumeErr != nil && !keepContext {
			s.invalidate()
		}
	}()
	max = sessionCollectionLimit(max)
	s.expire(ts)
	if s.pending == nil {
		s.pending = map[stunKey]stunRequest{}
		s.history = map[stunKey]bool{}
		s.channels = map[turnChannelKey]turnChannel{}
		s.permissions = map[turnPeerKey]time.Time{}
	}
	n, err := stunFrameLength(w, stream)
	if err != nil {
		return nil, err
	}
	if n == 0 || len(w) < n {
		return nil, fmt.Errorf("stun: truncated message")
	}
	if w[0]&0xc0 == 0x40 {
		ln := int(binary.BigEndian.Uint16(w[2:4]))
		if len(w) != n && (stream || len(w) != (n+3)&^3) {
			return nil, fmt.Errorf("turn: channel length mismatch")
		}
		ch := binary.BigEndian.Uint16(w[:2])
		k := turnChannelKey{dir, ch}
		v, ok := s.channels[k]
		if !ok {
			v, ok = s.channels[turnChannelKey{1 - dir, ch}]
		}
		if other, exists := s.channels[turnChannelKey{1 - dir, ch}]; exists && ok && other.peer != v.peer {
			ok = false
		}
		out := map[string]any{"Packet Name": "ChannelData", "TURN": true, "Channel Number": ch, "Data Length": ln, "Matched": ok}
		if ok {
			out["Peer Address"] = v.peer.String()
		} else {
			out["Context Missing"] = "channel binding was not observed"
		}
		return out, nil
	}
	if len(w) != n {
		return nil, fmt.Errorf("stun: message length mismatch")
	}
	t := binary.BigEndian.Uint16(w[:2])
	method := stunMethod(t)
	class := (t>>4)&1 | (t>>7)&2
	var id [12]byte
	copy(id[:], w[8:20])
	req := stunRequest{method: method, hash: sha256.Sum256(w), expires: s.clock.Add(40 * time.Second)}
	out := map[string]any{"Packet Name": stunName(method), "Class": class, "Transaction ID": hex.EncodeToString(id[:]), "TURN": stunTURNMethod(method), "Integrity": "absent"}
	attrs := []any{}
	fingerprint := false
	for at := 20; at < len(w); {
		if len(attrs) >= max {
			return nil, protocolError(ErrResourceExceeded, "STUN attribute budget")
		}
		if at+4 > len(w) {
			return nil, fmt.Errorf("stun: truncated attribute")
		}
		typ := binary.BigEndian.Uint16(w[at:])
		ln := int(binary.BigEndian.Uint16(w[at+2:]))
		end := at + 4 + ln
		next := (end + 3) &^ 3
		if next > len(w) {
			return nil, fmt.Errorf("stun: attribute exceeds message")
		}
		if fingerprint {
			return nil, fmt.Errorf("stun: fingerprint must be last")
		}
		v := w[at+4 : end]
		a := map[string]any{"Type": typ, "Length": ln}
		switch typ {
		case 1, 0x20, 0x12, 0x16:
			addr, e := stunAddress(v, w, typ != 1)
			if e != nil {
				return nil, e
			}
			a["Address"] = addr.String()
			if typ == 0x12 {
				req.peer = addr
				req.peers = append(req.peers, addr)
			}
			if typ == 0x16 {
				out["Relayed Address"] = addr.String()
			}
			if typ == 0x20 || typ == 1 {
				out["Mapped Address"] = addr.String()
			}
		case 0xc:
			if ln != 4 || v[2] != 0 || v[3] != 0 {
				return nil, fmt.Errorf("turn: channel attribute")
			}
			req.channel = binary.BigEndian.Uint16(v)
			if req.channel < 0x4000 || req.channel > 0x7fff {
				return nil, fmt.Errorf("turn: channel number")
			}
			a["Channel Number"] = req.channel
		case 0xd:
			if ln != 4 {
				return nil, fmt.Errorf("turn: lifetime attribute")
			}
			req.hasLifetime = true
			req.lifetime = binary.BigEndian.Uint32(v)
			a["Lifetime"] = req.lifetime
		case 0x19:
			if ln != 4 || v[1] != 0 || v[2] != 0 || v[3] != 0 {
				return nil, fmt.Errorf("turn: requested transport")
			}
			a["Protocol"] = v[0]
		case 6, 0x14, 0x15, 0x8022:
			a["Text"] = string(v)
		case 9:
			if ln < 4 || v[2] < 3 || v[2] > 6 || v[3] > 99 {
				return nil, fmt.Errorf("stun: error code")
			}
			out["Error Code"] = int(v[2])*100 + int(v[3])
			a["Reason"] = string(v[4:])
		case 8, 0x1c:
			if typ == 8 && ln != 20 || typ == 0x1c && (ln < 16 || ln > 32 || ln%4 != 0) {
				return nil, fmt.Errorf("stun: integrity length")
			}
			out["Integrity"] = "unverified: credentials required"
		case 0x8028:
			if ln != 4 || binary.BigEndian.Uint32(v) != (crc32.ChecksumIEEE(w[:at])^0x5354554e) {
				return nil, fmt.Errorf("stun: fingerprint mismatch")
			}
			fingerprint = true
			out["Fingerprint Valid"] = true
		case 0x13:
			a["Data Length"] = ln
		default:
			a["Value"] = append([]byte(nil), v...)
		}
		attrs = append(attrs, a)
		at = next
	}
	out["Attributes"] = attrs
	key := stunKey{dir, id}
	if class == 0 {
		old, ok := s.pending[key]
		h, seen := s.history[key]
		if s.blocked || seen && h {
			out["Retransmission"] = false
			out["Association Status"] = "ambiguous-transaction-id"
			out["Pending Transactions"] = len(s.pending)
			return out, nil
		}
		if ok && old.hash != req.hash {
			s.retire(key)
			keepContext = true
			return nil, protocolError(ErrDesynchronized, "STUN transaction ID reused for different request bytes")
		}
		if !seen && len(s.history) >= max {
			// Fully decoded unseen identity cannot be retained, but unrelated
			// already tracked transactions and exact retries remain usable.
			keepContext = true
			return nil, protocolError(ErrResourceExceeded, "STUN identity history budget")
		}
		if (method == 8 || method == 9) && !req.peer.IsValid() {
			return nil, fmt.Errorf("turn: request lacks peer address")
		}
		if method == 9 && (req.channel == 0 || len(req.peers) != 1) {
			return nil, fmt.Errorf("turn: ChannelBind lacks channel")
		}
		s.history[key] = false
		s.pending[key] = req
		out["Retransmission"] = ok
	} else if class >= 2 {
		key.dir = 1 - dir
		old, ok := s.pending[key]
		out["Matched"] = !s.blocked && ok && old.method == method
		if s.blocked {
			out["Association Status"] = "unverified-after-refusal"
		}
		if !s.blocked && ok && old.method == method {
			if class == 2 {
				switch method {
				case 3, 4:
					if !req.hasLifetime {
						return nil, fmt.Errorf("turn: successful allocation/refresh lacks lifetime")
					}
					if req.lifetime == 0 {
						s.clearAllocation(key.dir)
					} else {
						s.allocations[key.dir] = s.clock.Add(time.Duration(req.lifetime) * time.Second)
					}
					out["Lifetime"] = req.lifetime
				case 8, 9:
					added := map[turnPeerKey]bool{}
					for _, peer := range old.peers {
						pk := turnPeerKey{key.dir, peer.Addr()}
						if _, exists := s.permissions[pk]; !exists {
							added[pk] = true
						}
					}
					if len(s.permissions)+len(added) > max {
						keepContext = true // Fully parsed matching response; no relay mutation occurred.
						return nil, protocolError(ErrResourceExceeded, "TURN permission budget")
					}
					ck := turnChannelKey{key.dir, old.channel}
					if method == 9 {
						if _, exists := s.channels[ck]; !exists && len(s.channels) >= max {
							keepContext = true
							return nil, protocolError(ErrResourceExceeded, "TURN channel budget")
						}
						if current, exists := s.channels[ck]; exists && current.peer != old.peer {
							return nil, protocolError(ErrDesynchronized, "TURN channel rebound before expiry")
						}
						for k, v := range s.channels {
							if k.dir == key.dir && k.channel != old.channel && v.peer == old.peer {
								return nil, protocolError(ErrDesynchronized, "TURN peer bound to another channel")
							}
						}
						s.channels[ck] = turnChannel{old.peer, s.clock.Add(600 * time.Second)}
					}
					for _, peer := range old.peers {
						s.permissions[turnPeerKey{key.dir, peer.Addr()}] = s.clock.Add(300 * time.Second)
					}
				}
			}
			s.retire(key)
			out["In Reply To"] = stunName(old.method)
		} else if !ok {
			if _, seen := s.history[key]; !seen {
				if len(s.history) < max {
					s.history[key] = true
				} else {
					s.invalidate()
				}
			}
		}
	}
	out["Pending Transactions"] = len(s.pending)
	return out, nil
}

// Only methods defined for TURN carry TURN semantics. Unknown STUN methods
// remain visible as STUN observations instead of being misclassified as TURN.
func stunTURNMethod(method uint16) bool {
	switch method {
	case 3, 4, 6, 7, 8, 9, 10, 11, 12: // Allocate through ConnectionAttempt.
		return true
	default:
		return false
	}
}
