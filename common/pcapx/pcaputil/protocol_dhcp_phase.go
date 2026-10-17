package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"net"
	"time"
)

// Fixed row plus map/string overhead. Only value fingerprints and IDs survive
// callbacks; caller-owned decoded option graphs are never association state.
const dhcpPhaseRowOverhead = 224

func dhcpPhaseFingerprint(w []byte, v6 bool) ([32]byte, bool) {
	h := sha256.New()
	if !v6 {
		if len(w) < 240 {
			return [32]byte{}, false
		}
		h.Write(w[:8])
		h.Write(w[10:])
		return *(*[32]byte)(h.Sum(nil)), true
	}
	if !dhcp6FingerprintWrite(h, w, 0) {
		return [32]byte{}, false
	}
	return *(*[32]byte)(h.Sum(nil)), true
}
func dhcp6FingerprintWrite(h hash.Hash, w []byte, depth int) bool {
	if depth > 8 || len(w) < 4 {
		return false
	}
	at := 4
	relay := w[0] == 12 || w[0] == 13
	if relay {
		at = 34
		if len(w) < at {
			return false
		}
	}
	h.Write(w[:at])
	for at < len(w) {
		if len(w)-at < 4 {
			return false
		}
		c, n := binary.BigEndian.Uint16(w[at:]), int(binary.BigEndian.Uint16(w[at+2:]))
		if n > len(w)-at-4 {
			return false
		}
		v := w[at+4 : at+4+n]
		if !relay && c == 8 && n == 2 {
			h.Write(w[at : at+4])
		} else if relay && c == 9 {
			h.Write(w[at : at+4])
			if !dhcp6FingerprintWrite(h, v, depth+1) {
				return false
			}
		} else {
			h.Write(w[at : at+4+n])
		}
		at += 4 + n
	}
	return true
}
func dhcp6PhaseOption(info map[string]any, code uint16) ([]byte, bool) {
	for _, o := range info["Options"].([]map[string]any) {
		if o["Code"] == code {
			w, ok := o["Raw"].([]byte)
			return w, ok
		}
	}
	return nil, false
}
func dhcpPhaseServer(info map[string]any, v6 bool) ([32]byte, bool) {
	if v6 {
		b, ok := dhcp6PhaseOption(info, 2)
		return sha256.Sum256(b), ok
	}
	s, ok := info["Server Identifier"].(string)
	if !ok {
		return [32]byte{}, false
	}
	return sha256.Sum256(net.ParseIP(s).To4()), true
}
func dhcpPhaseClient(info map[string]any, v6 bool) ([32]byte, bool) {
	if v6 {
		b, ok := dhcp6PhaseOption(info, 1)
		return sha256.Sum256(b), ok
	}
	b, ok := info["Client Identifier"].([]byte)
	return sha256.Sum256(b), ok
}
func dhcpPhaseAssociation(e *ProtocolEvent, p dnsPending) {
	s := "missing-request"
	if p.refused {
		s = "refused-client-xid"
	} else if p.phaseConflict || p.completed && p.ambiguous {
		s = "ambiguous-client-xid"
	}
	e.Session["Association"] = s
}
func (a *binParser) observeDHCPPhase(e *ProtocolEvent, info map[string]any, key, relay string, typ byte, v6 bool) {
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	s := &a.lan
	if s.pending == nil {
		s.pending = map[string]dnsPending{}
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	p, known := s.pending[key]

	if v6 {
		if _, denied := s.pending[dhcp6DeniedKey(e.Domain, relay, info["Transaction ID"])]; denied {
			if known {
				p.ambiguous, p.refused = true, true
				s.pending[key] = p
			} else {
				e.Session["Association"] = "refused-client-xid"
				return
			}
		}
	}
	response := typ == 2 || typ == 5 || typ == 6
	if v6 {
		response = typ == 2 || typ == 7
	}
	if !response {
		if known && e.Timestamp.Sub(p.ts) > 30*time.Second {
			p.ambiguous = true
			s.pending[key] = p
		}
		query := typ == 1 || typ == 3 || typ == 8
		if v6 {
			query = typ == 1 || typ == 3 || typ == 4 || typ == 5 || typ == 6 || typ == 8 || typ == 9 || typ == 11
		}
		if !query {
			return
		}
		digest, ok := dhcpPhaseFingerprint(e.Raw, v6)
		if !ok {
			return
		}
		client, clientChosen := dhcpPhaseClient(info, v6)
		server, serverChosen := dhcpPhaseServer(info, v6)
		rapid := false
		if v6 {
			_, rapid = dhcp6PhaseOption(info, 14)
		}
		if known {
			if p.completed || p.ambiguous {
				p.ambiguous = true
				s.pending[key] = p
				return
			}
			same := p.queryType == typ && p.queryHash == digest
			progress := !v6 && p.queryType == 1 && typ == 3 && p.clientChosen == clientChosen && (!clientChosen || p.clientHash == client)
			if !same && !progress {
				p.ambiguous, p.phaseConflict = true, true
				s.pending[key] = p
				return
			}
			if progress {
				p.queryType, p.queryHash = typ, digest
				p.requestID = e.ID
				p.serverChosen, p.serverHash = serverChosen, server
			}
			if e.Timestamp.After(p.ts) {
				p.ts = e.Timestamp
				if !v6 && typ == 1 {
					p.discoverSeconds, p.discoverNanos = e.Timestamp.Unix(), int32(e.Timestamp.Nanosecond())
				}
			}
			s.pending[key] = p
			e.TransactionID = p.id
			return
		}
		if v6 && s.untracked6 || !v6 && s.untracked4 {
			return
		}
		cost := int64(len(key) + dhcpPhaseRowOverhead)
		if len(s.pending)+len(a.dns.pending) >= a.budget.MaxCollectionElements || !a.reserveEvidence(cost) {
			return
		}
		p = dnsPending{id: e.ID, ts: e.Timestamp, cost: cost, queryType: typ, queryHash: digest, serverChosen: serverChosen, serverHash: server, clientChosen: clientChosen, clientHash: client, rapidCommit: rapid}
		if !v6 && typ == 1 {
			p.discoverID = e.ID
			p.discoverSeconds, p.discoverNanos = e.Timestamp.Unix(), int32(e.Timestamp.Nanosecond())
		} else {
			p.requestID = e.ID
		}
		s.pending[key] = p
		e.TransactionID = p.id
		return
	}
	if !known || p.ambiguous {
		dhcpPhaseAssociation(e, p)
		return
	}
	if p.serverChosen && !(!v6 && typ == 2) {
		server, ok := dhcpPhaseServer(info, v6)
		if !ok {
			e.Session["Association"] = "missing-server-identifier"
			return
		}
		if server != p.serverHash {
			e.Session["Association"] = "server-mismatch"
			return
		}
	}
	requestID := p.requestID
	terminal := true
	if !v6 && typ == 2 {
		requestID = p.discoverID
		terminal = false
	} else if !v6 {
		if p.queryType != 3 && !(typ == 5 && p.queryType == 8) {
			requestID = 0
		}
	} else if typ == 2 {
		if p.queryType != 1 {
			requestID = 0
		}
		terminal = false
	} else {
		switch p.queryType {
		case 1:
			_, rc := dhcp6PhaseOption(info, 14)
			if !p.rapidCommit || !rc {
				requestID = 0
			}
		case 3, 4, 5, 6, 8, 9, 11:
		default:
			requestID = 0
		}
	}
	if requestID == 0 {
		e.Session["Association"] = "missing-request-phase"
		p.ambiguous, p.phaseConflict = true, true
		s.pending[key] = p
		return
	}

	// A response is checked against its observed phase, but cannot advance
	// another phase/peer's clock. Only accepted client queries renew state.
	ageFrom := p.ts
	if !v6 && typ == 2 {
		ageFrom = time.Unix(p.discoverSeconds, int64(p.discoverNanos))
	}
	if e.Timestamp.Sub(ageFrom) > 30*time.Second {
		e.Session["Association"] = "missing-request"
		return
	}
	e.ResponseTo, e.TransactionID = requestID, p.id
	e.Session["Association"] = "observed-client-xid"
	if terminal {
		e.Completeness = "transaction"
		p.completed = true
		s.pending[key] = p
	} else if v6 {
		e.Completeness = "transaction"
	}
}
