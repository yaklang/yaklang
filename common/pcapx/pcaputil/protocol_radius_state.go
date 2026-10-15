package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

// Identifier matching observes a possible exchange. Without a shared secret it
// does not verify the Response Authenticator, peer identity or access decision.
type radiusKey struct {
	dir int
	id  byte
}
type radiusOwner struct {
	digest, replyDigest         [32]byte
	eventID                     uint64
	last                        time.Time
	retired, completed, replied bool
}
type binRADIUS struct {
	pending   map[radiusKey]radiusOwner
	untracked bool
}

const radiusObserverTTL = 5 * time.Minute

func (s *binRADIUS) storage() int64 { return 256 + 192*int64(len(s.pending)) }
func (s *binRADIUS) invalidate() {
	for k, v := range s.pending {
		v.retired = true
		s.pending[k] = v
	}
	s.untracked = true
}
func (s *binRADIUS) outstanding() int {
	n := 0
	for _, v := range s.pending {
		if !v.retired && !v.completed {
			n++
		}
	}
	return n
}
func radiusFields(raw []byte, max int) (map[string]any, error) {
	if len(raw) < 20 {
		return nil, fmt.Errorf("radius: truncated header")
	}
	n := int(binary.BigEndian.Uint16(raw[2:4]))
	if n < 20 || n > 4096 || n > len(raw) {
		return nil, fmt.Errorf("radius: invalid declared length")
	}
	padding := len(raw) - n
	raw = raw[:n]
	code := raw[0]
	if code != 1 && code != 2 && code != 3 && code != 11 {
		return nil, fmt.Errorf("radius: unsupported code %d", code)
	}
	out := map[string]any{"Packet Name": radiusCodeName(code), "Code": int(code), "Identifier": int(raw[1]), "Authenticator": bytes.Clone(raw[4:20]), "Packet Length": n, "Padding Bytes": padding, "Authentication": "not-verified"}
	role := "response"
	if code == 1 {
		role = "request"
	}
	out["Role"] = role
	attrs := make([]map[string]any, 0)
	seen := map[byte]bool{}
	for at := 20; at < n; {
		if n-at < 2 {
			return nil, fmt.Errorf("radius: truncated attribute header")
		}
		if raw[at+1] < 2 {
			return nil, fmt.Errorf("radius: attribute length smaller than 2")
		}
		if int(raw[at+1]) > n-at {
			return nil, fmt.Errorf("radius: invalid attribute length")
		}
		if len(attrs) >= max {
			return nil, protocolError(ErrResourceExceeded, "RADIUS attribute budget exceeded")
		}
		typ, sz := raw[at], int(raw[at+1])
		v := raw[at+2 : at+sz]
		at += sz
		switch typ {
		case 1, 2, 4, 32:
			if seen[typ] {
				return nil, fmt.Errorf("radius: repeated singleton attribute %d", typ)
			}
			seen[typ] = true
		}
		switch typ {
		case 1:
			if len(v) < 1 {
				return nil, fmt.Errorf("radius: empty User-Name")
			}
			out["User-Name"] = string(v)
		case 2:
			if len(v) < 16 || len(v) > 128 || len(v)%16 != 0 {
				return nil, fmt.Errorf("radius: User-Password width")
			}
			out["Encrypted"], out["User-Password"], out["User-Password Bytes"] = true, "opaque", len(v)
		case 4:
			if len(v) != 4 {
				return nil, fmt.Errorf("radius: NAS-IP-Address width")
			}
			out["NAS-IP-Address"] = fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
		case 32:
			if len(v) < 1 {
				return nil, fmt.Errorf("radius: empty NAS-Identifier")
			}
			out["NAS-Identifier"] = string(v)
		}
		attrs = append(attrs, map[string]any{"Type": typ, "Raw": bytes.Clone(v)})
	}
	out["Attributes"], out["Attribute Count"] = attrs, len(attrs)
	return out, nil
}
func (s *binRADIUS) observe(dir int, ts time.Time, wire []byte, id uint64, out map[string]any, limit int) (reply, transaction uint64) {
	n := int(binary.BigEndian.Uint16(wire[2:4]))
	wire = wire[:n]
	key := radiusKey{dir, wire[1]}
	response := wire[0] != 1
	if response {
		key.dir = 1 - dir
	}
	digest := sha256.Sum256(wire)
	if s.pending == nil {
		s.pending = map[radiusKey]radiusOwner{}
	}
	p, known := s.pending[key]
	if response {
		status := "missing-request"
		if known {
			status = "ambiguous-identifier"
			if !p.retired && ts.Sub(p.last) <= radiusObserverTTL {
				if !p.completed || p.replied && p.replyDigest == digest {
					out["In Reply To"] = "Access-Request"
					p.completed, p.replied, p.replyDigest = true, true, digest
					s.pending[key] = p
					return p.eventID, p.eventID
				}
			}
			p.retired = true
			s.pending[key] = p
		}
		if !known {
			s.pending[key] = radiusOwner{retired: true}
		}
		out["Association"] = status
		return 0, 0
	}
	if known {
		if p.retired || p.completed || p.digest != digest || ts.Sub(p.last) > radiusObserverTTL {
			p.retired = true
			s.pending[key] = p
			out["Association"] = "ambiguous-identifier"
			return 0, 0
		}
		if ts.After(p.last) {
			p.last = ts
			s.pending[key] = p
		}
		return 0, p.eventID
	}
	if s.untracked || len(s.pending) >= limit {
		s.untracked = true
		out["Association"] = "untracked-history"
		return 0, 0
	}
	s.pending[key] = radiusOwner{digest: digest, eventID: id, last: ts}
	return 0, id
}

// Compatibility for structural codec callers; event paths supply real direction,
// observed time and owned IDs through consumeRADIUS instead.
func (s *binRADIUS) consume(raw []byte, max int) (map[string]any, error) {
	out, err := radiusFields(raw, max)
	if err != nil {
		return nil, err
	}
	dir := 0
	if raw[0] != 1 {
		dir = 1
	}
	s.observe(dir, time.Time{}, raw, 0, out, max)
	return out, nil
}

func (s *binRADIUS) retire(dir int, w []byte, canAdd bool) {
	if len(w) < 2 {
		s.invalidate()
		return
	}
	key := radiusKey{dir, w[1]}
	if w[0] != 1 {
		key.dir = 1 - dir
	}
	p, known := s.pending[key]
	if !known && !canAdd {
		s.untracked = true
		return
	}
	if s.pending == nil {
		s.pending = map[radiusKey]radiusOwner{}
	}
	p.retired = true
	s.pending[key] = p
}
