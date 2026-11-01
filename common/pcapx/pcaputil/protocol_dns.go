package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// DNS display text is not a transaction identity. Keep the expanded wire and
// RFC-style escaped Name separately; do not replace invalid octets by U+FFFD.
func dnsDisplayName(wire []byte) string {
	var out strings.Builder
	for at := 0; at < len(wire); {
		n := int(wire[at])
		at++
		if n == 0 {
			break
		}
		if n > 63 || n > len(wire)-at {
			return ""
		}
		if out.Len() > 0 {
			out.WriteByte('.')
		}
		label := wire[at : at+n]
		for len(label) > 0 {
			r, size := utf8.DecodeRune(label)
			if (r == utf8.RuneError && size == 1) || !unicode.IsGraphic(r) {
				for _, c := range label[:size] {
					fmt.Fprintf(&out, "\\%03d", c)
				}
			} else {
				if r == '.' || r == '\\' {
					out.WriteByte('\\')
				}
				out.Write(label[:size])
			}
			label = label[size:]
		}
		at += n
	}
	return out.String()
}

// DecodeDNSMessage is the shared semantic DNS codec. Compression targets refer
// to the entire message, but the encoded RDATA cursor cannot leave its RR.
// DNSSEC records remain raw evidence; this API does not validate signatures.
func DecodeDNSMessage(w []byte, limit int) (map[string]any, error) {
	if len(w) < 12 {
		return nil, protocolError(ErrMalformedMessage, "DNS header truncated")
	}
	if limit <= 0 {
		limit = 4096
	}
	flags := binary.BigEndian.Uint16(w[2:])
	out := map[string]any{"ID": binary.BigEndian.Uint16(w), "Flags": flags, "Response": flags&0x8000 != 0, "Opcode": (flags >> 11) & 15, "RCODE": flags & 15, "Truncated": flags&0x200 != 0}
	pos, total := 12, 0
	optSeen := false
	name := func(end int) (string, error) {
		n, next, err := dnsParseName(w, pos)
		if err != nil {
			return "", err
		}
		if next > end {
			return "", fmt.Errorf("DNS name escapes RDATA window")
		}
		pos = next
		return n, nil
	}
	for section, key := range []string{"Questions", "Answers", "Authority", "Additional"} {
		count := int(binary.BigEndian.Uint16(w[4+2*section:]))
		total += count
		if total > limit {
			return nil, protocolError(ErrResourceExceeded, "DNS record budget")
		}
		rows := make([]map[string]any, 0, min(count, 16))
		for j := 0; j < count; j++ {
			nameAt := pos
			nm, next, nameWire, err := dnsParseNameWire(w, pos)
			pos = next
			if err != nil {
				return nil, err
			}
			need := 4
			if section > 0 {
				need = 10
			}
			if len(w)-pos < need {
				return nil, fmt.Errorf("DNS record truncated")
			}
			typ, class := binary.BigEndian.Uint16(w[pos:]), binary.BigEndian.Uint16(w[pos+2:])
			pos += 4
			row := map[string]any{"Name": nm, "Type": typ, "Class": class, "Class Base": class & 0x7fff}
			if display := dnsDisplayName(nameWire); display != nm {
				row["Display Name"], row["Name Wire"] = display, nameWire
			}
			if section == 0 {
				row["QU"] = class&0x8000 != 0
				row["Name Wire"] = nameWire
				rows = append(rows, row)
				continue
			}
			ttl, n := binary.BigEndian.Uint32(w[pos:]), int(binary.BigEndian.Uint16(w[pos+4:]))
			pos += 6
			if n > len(w)-pos {
				return nil, fmt.Errorf("DNS RDATA truncated")
			}
			start, end := pos, pos+n
			row["TTL"], row["Cache Flush"], row["Withdrawn"] = ttl, class&0x8000 != 0, ttl == 0
			row["RData"] = bytes.Clone(w[pos:end])
			switch typ {
			case 1, 28:
				size := 4
				if typ == 28 {
					size = 16
				}
				if n != size {
					return nil, fmt.Errorf("DNS address length")
				}
				row["Address"] = net.IP(w[pos:end]).String()
				pos = end
			case 2, 5, 12:
				row["Target"], err = name(end)
			case 15, 33:
				size := 2
				if typ == 33 {
					size = 6
				}
				if n < size {
					return nil, fmt.Errorf("DNS MX/SRV length")
				}
				row["Priority"] = binary.BigEndian.Uint16(w[pos:])
				if typ == 33 {
					row["Weight"], row["Port"] = binary.BigEndian.Uint16(w[pos+2:]), binary.BigEndian.Uint16(w[pos+4:])
				}
				pos += size
				row["Target"], err = name(end)
			case 6:
				row["MNAME"], err = name(end)
				if err == nil {
					row["RNAME"], err = name(end)
				}
				if err == nil {
					if end-pos != 20 {
						return nil, fmt.Errorf("DNS SOA length")
					}
					for _, k := range []string{"Serial", "Refresh", "Retry", "Expire", "Minimum"} {
						row[k] = binary.BigEndian.Uint32(w[pos:])
						pos += 4
					}
				}
			case 16:
				if n == 0 {
					return nil, fmt.Errorf("DNS TXT requires at least one character-string")
				}
				var txt []string
				for pos < end {
					n := int(w[pos])
					pos++
					if n > end-pos {
						return nil, fmt.Errorf("DNS TXT length")
					}
					if len(txt) >= min(limit, 4096) {
						return nil, protocolError(ErrResourceExceeded, "DNS TXT count")
					}
					txt = append(txt, string(w[pos:pos+n]))
					pos += n
				}
				row["Text"] = txt
			case 257:
				// RFC 8659 section 4.1. Preserve unknown tags, reserved flag
				// bits, and binary property values; no CA policy is evaluated.
				if n < 3 {
					return nil, fmt.Errorf("DNS CAA length")
				}
				tagLength := int(w[pos+1])
				if tagLength == 0 || tagLength > n-2 {
					return nil, fmt.Errorf("DNS CAA tag length")
				}
				tag := w[pos+2 : pos+2+tagLength]
				for _, c := range tag {
					if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
						return nil, fmt.Errorf("DNS CAA tag character")
					}
				}
				row["CAA Flags"], row["Issuer Critical"] = w[pos], w[pos]&0x80 != 0
				row["Tag"], row["Value"] = string(tag), bytes.Clone(w[pos+2+tagLength:end])
				pos = end
			case 41:
				if section != 3 || optSeen || nm != "" || w[nameAt] != 0 || next != nameAt+1 {
					return nil, protocolError(ErrMalformedMessage, "DNS OPT must be unique, in Additional, with an uncompressed root name")
				}
				optSeen = true
				out["RCODE"] = flags&15 | uint16(ttl>>24)<<4
				var options []map[string]any
				for pos < end {
					if end-pos < 4 {
						return nil, fmt.Errorf("DNS OPT header")
					}
					code, l := binary.BigEndian.Uint16(w[pos:]), int(binary.BigEndian.Uint16(w[pos+2:]))
					pos += 4
					if l > end-pos {
						return nil, fmt.Errorf("DNS OPT length")
					}
					if len(options) >= limit {
						return nil, protocolError(ErrResourceExceeded, "DNS OPT count")
					}
					options = append(options, map[string]any{"Code": code, "Value": bytes.Clone(w[pos : pos+l])})
					pos += l
				}
				row["Options"], row["UDP Size"], row["EDNS Version"] = options, class, uint8(ttl>>16)
				row["EDNS Flags"], row["DNSSEC OK"] = uint16(ttl), ttl&0x8000 != 0
			case 64, 65:
				if n < 3 {
					return nil, fmt.Errorf("DNS SVCB length")
				}
				row["Priority"] = binary.BigEndian.Uint16(w[pos:])
				pos += 2
				row["Target"], err = name(end)
				var params []map[string]any
				last := -1
				for err == nil && pos < end {
					if end-pos < 4 {
						return nil, fmt.Errorf("DNS SVCB parameter")
					}
					k, l := int(binary.BigEndian.Uint16(w[pos:])), int(binary.BigEndian.Uint16(w[pos+2:]))
					pos += 4
					if k <= last || l > end-pos {
						return nil, fmt.Errorf("DNS SVCB order/length")
					}
					last = k
					if len(params) >= limit {
						return nil, protocolError(ErrResourceExceeded, "DNS SVCB count")
					}
					params = append(params, map[string]any{"Key": uint16(k), "Value": bytes.Clone(w[pos : pos+l])})
					pos += l
				}
				row["Parameters"] = params
			default:
				pos = end
				row["RData Completeness"] = "opaque"
			}
			if err != nil {
				return nil, err
			}
			if pos != end {
				return nil, fmt.Errorf("DNS RDATA trailing bytes at %d", start)
			}
			rows = append(rows, row)
		}
		out[key] = rows
	}
	if pos != len(w) {
		return nil, fmt.Errorf("DNS trailing bytes")
	}
	return out, nil
}

// DHCP keeps its established storage and accounting; DNS identity history is separate.
type dnsPending struct {
	completed, ambiguous, refused                          bool
	phaseConflict, serverChosen, clientChosen, rapidCommit bool
	queryType                                              byte
	id, requestID, discoverID                              uint64
	ts                                                     time.Time
	cost                                                   int64
	discoverSeconds                                        int64
	discoverNanos                                          int32
	queryHash, serverHash, clientHash                      [32]byte
}

type dnsCorrelation struct {
	untracked4, untracked6 bool
	pending                map[string]dnsPending
	clock                  time.Time
}

type dnsObservation struct {
	identity           string
	id                 uint64
	ts, clock, touched time.Time
	flow               uint64
	cost               int64
	owner              string
	digest             [32]byte
	active, ambiguous  bool
	responders         map[string]struct{}
}
type dnsObservationCorrelation struct {
	// Completed, orphaned and expired identities remain bounded, charged history.
	pending             map[string]dnsObservation
	untracked           bool
	untrackedTCPThrough uint64
}

// A fresh TCP FlowID cannot reuse an unretained identity from an older
// connection. UDP has no such generation boundary and remains conservative.
func (s *dnsObservationCorrelation) untrackedInput(flow uint64) {
	if flow == 0 {
		s.untracked = true
	} else if flow > s.untrackedTCPThrough {
		s.untrackedTCPThrough = flow
	}
}
func (s *dnsObservationCorrelation) newOwnerAllowed(flow uint64) bool {
	if flow == 0 {
		return !s.untracked
	}
	return flow > s.untrackedTCPThrough
}

func dnsOwner(e *ProtocolEvent, proto string, flow uint64, src, server string) string {
	return fmt.Sprintf("%v/%s/%s/%d/%s/%s", e.Domain, proto, e.Transport, flow, src, server)
}
func dnsRefusalKey(owner, identity string) string { return "refused/" + owner + "/" + identity }
func (a *binParser) retireDNS(p *dnsObservation) {
	for peer := range p.responders {
		cost := int64(len(peer) + 32)
		a.buffered.Add(-cost)
		p.cost -= cost
	}
	p.responders = nil
	p.active = false
	p.ambiguous = true
	p.id = 0
}

func dnsQuestionKey(info map[string]any) string {
	var b strings.Builder
	fmt.Fprint(&b, info["ID"], "/", info["Opcode"])
	for _, q := range info["Questions"].([]map[string]any) {
		// Canonicalize only ASCII case; label lengths and all other octets remain
		// part of the identity. Display escaping must not define association.
		wire := bytes.Clone(q["Name Wire"].([]byte))
		for i, c := range wire {
			if c >= 'A' && c <= 'Z' {
				wire[i] = c + ('a' - 'A')
			}
		}
		fmt.Fprintf(&b, "/%s/%v/%v", hex.EncodeToString(wire), q["Type"], q["Class"])
	}
	return b.String()
}

func (a *binParser) closeDNSFlow(id uint64) {
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	for k, p := range a.dns.pending {
		if p.flow == id && id != 0 {
			a.buffered.Add(-p.cost)
			delete(a.dns.pending, k)
		}
	}
}

func (a *binParser) dnsEvent(e *ProtocolEvent, w []byte) error {
	info, err := DecodeDNSMessage(w, a.budget.MaxCollectionElements)
	if err != nil {
		return err
	}
	return a.dnsEventDecoded(e, info, w)
}
func (a *binParser) dnsEventDecoded(e *ProtocolEvent, info map[string]any, wire ...[]byte) error {
	if e.Session == nil {
		e.Session = map[string]any{}
	}
	// Transport framers must not expose their older ID-only association as a
	// second, contradictory result alongside the canonical question-aware one.
	delete(e.Session, "Matched Request")
	delete(e.Session, "Unmatched")
	delete(e.Session, "Association Status")
	e.Session["DNS"] = info
	e.Completeness = "message"
	if e.Protocol == "mdns" {
		e.Session["Association"] = "multicast-observation"
		return nil
	}
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	s := &a.dns
	if s.pending == nil {
		s.pending = map[string]dnsObservation{}
	}
	src, dst := e.Source, e.Destination
	if info["Response"] == true {
		src, dst = dst, src
	}
	flow := uint64(0)
	if e.Transport == "tcp" {
		flow = e.FlowID
		if flow == 0 {
			e.Session["Association Status"] = "missing-connection"
			return nil
		}
	}
	ownerFor := func(server string) string {
		return dnsOwner(e, e.Protocol, flow, src, server)
	}
	keyFor := func(server string) string { return ownerFor(server) + "/" + dnsQuestionKey(info) }
	key := keyFor(dst)
	multicast := false
	if e.Protocol == "llmnr" && e.Transport == "udp" {
		host, port, _ := net.SplitHostPort(dst)
		multicast = port == "5355" && (host == "224.0.0.252" || host == "ff02::1:3")
		if multicast {
			key = keyFor("multicast")
		}
		if info["Response"] == true {
			_, responderPort, _ := net.SplitHostPort(e.Source)
			if _, ok := s.pending[key]; !ok && responderPort == "5355" {
				key = keyFor("multicast")
				multicast = true
			}
		}
	}
	owner := ownerFor(dst)
	if multicast {
		owner = ownerFor("multicast")
	}
	clock := e.Timestamp
	for _, p := range s.pending {
		if p.owner == owner && p.clock.After(clock) {
			clock = p.clock
		}
	}
	for k, p := range s.pending {
		if p.owner != owner {
			continue
		}
		p.clock = clock
		if p.active && clock.Sub(p.touched) > 30*time.Second {
			a.retireDNS(&p)
		}
		s.pending[k] = p
	}
	identity := fmt.Sprint(info["ID"], "/", info["Opcode"])
	if _, refused := s.pending[dnsRefusalKey(owner, identity)]; refused {
		e.Session["Association"], e.Session["Association Status"] = "ambiguous-context", "ambiguous"
		if info["Response"] == true {
			e.Session["Unmatched"] = true
		}
		return nil
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	if info["Response"] == true {
		if p, ok := s.pending[key]; ok && p.active && !p.ambiguous {
			e.ResponseTo, e.TransactionID = p.id, p.id
			e.Session["Association Status"] = "matched"
			if qs, ok := info["Questions"].([]map[string]any); ok && len(qs) > 0 {
				e.Session["Matched Request"] = qs[0]["Name"]
			}
			e.Session["Latency"] = e.Timestamp.Sub(p.ts).Seconds()
			e.Completeness = "transaction"
			if multicast {
				if p.responders == nil {
					p.responders = map[string]struct{}{}
				}
				if _, seen := p.responders[e.Source]; seen {
					e.Session["Retransmission"] = true
				} else {
					cost := int64(len(e.Source) + 32)
					if len(p.responders) >= a.budget.MaxCollectionElements || !a.reserveEvidence(cost) {
						e.ResponseTo, e.TransactionID = 0, 0
						e.Completeness = "message"
						delete(e.Session, "Matched Request")
						e.Session["Association Status"] = "limited"
						return nil
					}
					p.responders[e.Source] = struct{}{}
					p.cost += cost
				}
				e.Session["Responder Count"] = len(p.responders)
			} else {
				p.active = false
				p.id = 0
			}
			s.pending[key] = p
		} else {
			e.Session["Association"], e.Session["Association Status"], e.Session["Unmatched"] = "missing-request", "missing-request", true
			if _, ok := s.pending[key]; ok {
				e.Session["Association"], e.Session["Association Status"] = "ambiguous-context", "ambiguous"
			} else {
				cost := int64(len(key) + len(owner) + 256)
				if len(s.pending)+len(a.lan.pending) < a.budget.MaxCollectionElements && a.reserveEvidence(cost) {
					s.pending[key] = dnsObservation{identity: identity, owner: owner, clock: clock, touched: clock, flow: flow, cost: cost, ambiguous: true}
				} else {
					s.untrackedInput(flow)
					e.Session["Association Status"] = "limited"
				}
			}
		}
	} else {
		raw := e.Raw
		if len(wire) > 0 {
			raw = wire[0]
		}
		if len(raw) == 0 {
			raw, _ = json.Marshal(info)
		}
		digest := sha256.Sum256(raw)
		if p, ok := s.pending[key]; ok {
			if p.active && !p.ambiguous && p.digest == digest {
				e.TransactionID = p.id
				e.Session["Retransmission"] = true
				p.touched = clock
			} else {
				p.ambiguous = true
				p.active = false
				p.id = 0
				e.Session["Association"], e.Session["Association Status"] = "ambiguous-context", "ambiguous"
			}
			s.pending[key] = p
		} else {
			cost := int64(len(key) + len(owner) + 256)
			if s.newOwnerAllowed(flow) && len(s.pending)+len(a.lan.pending) < a.budget.MaxCollectionElements && a.reserveEvidence(cost) {
				e.TransactionID = e.ID
				s.pending[key] = dnsObservation{identity: identity, id: e.ID, ts: e.Timestamp, touched: clock, clock: clock, flow: flow, cost: cost, owner: owner, digest: digest, active: true}
			} else {
				s.untrackedInput(flow)
				e.Session["Association"], e.Session["Association Status"] = "limited", "limited"
			}
		}
	}
	return nil
}
