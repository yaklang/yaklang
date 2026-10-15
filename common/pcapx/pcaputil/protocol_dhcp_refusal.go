package pcaputil

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

func dhcp6DeniedKey(domain CaptureDomain, relay string, xid any) string {
	return fmt.Sprintf("dhcp6-refused/%v/%s/%v", domain, relay, xid)
}

// Only a fixed header/prefix is inspected on the rejected path. No complete
// oversized datagram, option graph or DUID is copied into association history.
func dhcpRefusalKey(e *ProtocolEvent, proto string, wire []byte) string {
	w := wire[:min(len(wire), 256)]
	if proto == "dhcp" {
		if len(w) < 240 || binary.BigEndian.Uint32(w[236:240]) != dhcpCookie || w[2] == 0 || w[2] > 16 {
			return ""
		}
		return fmt.Sprintf("dhcp/%v/%v/%x/%v", e.Domain, net.IP(w[24:28]).String(), w[28:28+int(w[2])], binary.BigEndian.Uint32(w[4:8]))
	}
	relay := ""
	for depth := 0; depth < 8; depth++ {
		if len(w) < 4 {
			return ""
		}
		if w[0] != 12 && w[0] != 13 {
			xid := uint32(w[1])<<16 | uint32(w[2])<<8 | uint32(w[3])
			marker := dhcp6DeniedKey(e.Domain, relay, xid)
			for p := 4; p+4 <= len(w); {
				code, n := binary.BigEndian.Uint16(w[p:]), int(binary.BigEndian.Uint16(w[p+2:]))
				p += 4
				if n > len(w)-p {
					break
				}
				if code == 1 && n >= 2 && n <= 128 {
					return fmt.Sprintf("dhcp6/%v/%s/%x/%v", e.Domain, relay, w[p:p+n], xid)
				}
				p += n
			}
			return marker
		}
		if len(w) < 34 {
			return ""
		}
		relay += fmt.Sprint(net.IP(w[2:18]).String(), "/", net.IP(w[18:34]).String(), "/")
		found := false
		for p := 34; p+4 <= len(w); {
			code, n := binary.BigEndian.Uint16(w[p:]), int(binary.BigEndian.Uint16(w[p+2:]))
			p += 4
			if code == 9 {
				w = w[p:min(len(w), p+n)]
				found = true
				break
			}
			if n > len(w)-p {
				break
			}
			p += n
		}
		if !found {
			return ""
		}
	}
	return ""
}
func (a *binParser) recordDHCPRefusal(e *ProtocolEvent, proto string, w []byte) {
	key := dhcpRefusalKey(e, proto, w)
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	s := &a.lan
	if s.pending == nil {
		s.pending = map[string]dnsPending{}
	}
	// An unavailable DUID in the bounded prefix is uncertainty for this
	// observed relay/transaction, not proof of any client. Retire matching
	// retained candidates even if the history marker itself cannot fit.
	if strings.HasPrefix(key, "dhcp6-refused/") {
		at := strings.LastIndex(key, "/")
		prefix := strings.Replace(key[:at+1], "dhcp6-refused/", "dhcp6/", 1)
		for k, p := range s.pending {
			if strings.HasPrefix(k, prefix) && strings.HasSuffix(k, key[at:]) {
				p.ambiguous, p.refused = true, true
				s.pending[k] = p
			}
		}
	} else if key == "" {
		prefix := fmt.Sprintf("dhcp/%v/", e.Domain)
		if proto == "dhcpv6" {
			prefix = fmt.Sprintf("dhcp6/%v/", e.Domain)
		}
		for k, p := range s.pending {
			if strings.HasPrefix(k, prefix) {
				p.ambiguous, p.refused = true, true
				s.pending[k] = p
			}
		}
	}
	if key != "" {
		if p, ok := s.pending[key]; ok {
			p.ambiguous, p.refused = true, true
			s.pending[key] = p
			return
		}
		cost := int64(len(key) + dhcpPhaseRowOverhead)
		if len(s.pending)+len(a.dns.pending) < a.budget.MaxCollectionElements && a.reserveEvidence(cost) {
			s.pending[key] = dnsPending{ts: e.Timestamp, cost: cost, ambiguous: true, refused: true}
			return
		}
	}
	// A missing identity/slot cannot become a completely observed new owner.
	// Already retained, separately identified owners remain usable and charged.
	if proto == "dhcp" {
		s.untracked4 = true
	} else {
		s.untracked6 = true
	}
}
func (a *binParser) refuseDHCPOversizeAssociation(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	proto := a.datagramDecodeAs[dst]
	if proto == "" {
		proto = a.datagramDecodeAs[src]
	}
	explicit := proto != ""
	if proto == "" {
		if len(w) >= 240 && probeDHCP(w[:240], 240).Verdict == ProbeAccept {
			proto = "dhcp"
		} else if (src == 546 || dst == 546 || src == 547 || dst == 547) && len(w) >= 4 && w[0] >= 1 && w[0] <= 13 {
			proto = "dhcpv6"
		}
	}
	if proto != "dhcp" && proto != "dhcpv6" {
		return false
	}
	a.recordDHCPRefusal(e, proto, w)
	e.nativeDHCPObservation = true
	e.Protocol, e.Profile, e.Admission = proto, proto+"-native", "selected-endpoint-refusal"
	if explicit {
		e.Admission = "explicit-decode-as"
	}
	preview := w[:min(len(w), a.config.MaxMessageBytes)]
	a.finishProtocolDatagram(e, preview, nil, protocolError(ErrResourceExceeded, "DHCP-family UDP datagram exceeds message byte budget"))
	e.ExpertCode = string(ErrResourceExceeded)
	a.messageBytes.Add(uint64(len(w) - len(preview)))
	a.limited.Add(uint64(len(w) - len(preview)))
	return true
}
