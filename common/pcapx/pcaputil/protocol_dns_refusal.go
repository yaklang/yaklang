package pcaputil

import (
	"encoding/binary"
	"fmt"
	"net"
)

// A denied message is still an observed gap. Only bounded header identity and
// already selected endpoint/capture context are inspected; no oversized body
// parsing or unbounded history is permitted.
func (a *binParser) recordDNSRefusal(e *ProtocolEvent, proto string, w []byte) {
	if proto == "mdns" {
		return
	}
	if len(w) < 4 {
		return
	} // A complete denied datagram normally has this bounded header.
	src, dst := e.Source, e.Destination
	response := w[2]&128 != 0
	if response {
		src, dst = dst, src
	}
	owner := dnsOwner(e, proto, 0, src, dst)
	host, port, _ := net.SplitHostPort(dst)
	if proto == "llmnr" && !response && port == "5355" && (host == "224.0.0.252" || host == "ff02::1:3") {
		owner = dnsOwner(e, proto, 0, src, "multicast")
	}
	owners := []string{owner}
	if proto == "llmnr" && response {
		_, p, _ := net.SplitHostPort(e.Source)
		if p == "5355" {
			owners = append(owners, dnsOwner(e, proto, 0, src, "multicast"))
		}
	}
	identity := fmt.Sprint(binary.BigEndian.Uint16(w), "/", (binary.BigEndian.Uint16(w[2:])>>11)&15)
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	s := &a.dns
	if s.pending == nil {
		s.pending = map[string]dnsObservation{}
	}
	for _, owner := range owners {
		for k, p := range s.pending {
			if p.owner == owner && p.identity == identity {
				a.retireDNS(&p)
				s.pending[k] = p
			}
		}
		key := dnsRefusalKey(owner, identity)
		if _, ok := s.pending[key]; ok {
			continue
		}
		cost := int64(len(key) + len(owner) + 256)
		if len(s.pending)+len(a.lan.pending) < a.budget.MaxCollectionElements && a.reserveEvidence(cost) {
			s.pending[key] = dnsObservation{owner: owner, identity: identity, clock: e.Timestamp, touched: e.Timestamp, cost: cost, ambiguous: true}
		} else {
			s.untracked = true
		}
	}
}
func (a *binParser) refuseDNSOversizeAssociation(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	proto := a.datagramDecodeAs[dst]
	if proto == "" {
		proto = a.datagramDecodeAs[src]
	}
	explicit := proto != ""
	if proto == "" {
		if src == 53 || dst == 53 {
			proto = "dns"
		} else if src == 5355 || dst == 5355 {
			proto = "llmnr"
		}
	}
	if len(w) < 12 {
		return false
	}
	if !explicit && !dnsHeader(w[:12]) {
		return false
	}
	if proto == "" {
		// A previously admitted nonstandard-port DNS owner is also protected.
		a.dnsMu.Lock()
		for _, p := range a.dns.pending {
			for _, kind := range []string{"dns", "llmnr"} {
				if p.owner == dnsOwner(e, kind, 0, e.Source, e.Destination) || p.owner == dnsOwner(e, kind, 0, e.Destination, e.Source) {
					proto = kind
					break
				}
			}
			if proto != "" {
				break
			}
		}
		a.dnsMu.Unlock()
	}
	if proto != "dns" && proto != "llmnr" {
		return false
	}
	a.recordDNSRefusal(e, proto, w)
	e.nativeDNSObservation = true
	e.Protocol, e.Profile, e.Admission, e.Completeness = proto, proto+"-native", "selected-endpoint-refusal", "message"
	preview := w[:min(len(w), a.config.MaxMessageBytes)]
	a.finishProtocolDatagram(e, preview, nil, protocolError(ErrResourceExceeded, "DNS-family UDP datagram exceeds message byte budget"))
	e.ExpertCode = string(ErrResourceExceeded)
	// The preview is owned and bounded; accounting still describes all denied bytes.
	a.messageBytes.Add(uint64(len(w) - len(preview)))
	a.limited.Add(uint64(len(w) - len(preview)))
	return true
}
