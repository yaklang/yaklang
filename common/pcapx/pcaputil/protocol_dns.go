package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

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
			nm, err := name(len(w))
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
			if section == 0 {
				row["QU"] = class&0x8000 != 0
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
				var txt []string
				for pos < end {
					n := int(w[pos])
					pos++
					if n > end-pos {
						return nil, fmt.Errorf("DNS TXT length")
					}
					if len(txt) >= limit {
						return nil, protocolError(ErrResourceExceeded, "DNS TXT count")
					}
					txt = append(txt, string(w[pos:pos+n]))
					pos += n
				}
				row["Text"] = txt
			case 41:
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

type dnsPending struct {
	id uint64
	ts time.Time
}
type dnsCorrelation struct {
	pending map[string]dnsPending
	clock   time.Time
}

func dnsQuestionKey(info map[string]any) string {
	var b strings.Builder
	fmt.Fprint(&b, info["ID"])
	for _, q := range info["Questions"].([]map[string]any) {
		fmt.Fprintf(&b, "/%s/%v/%v", strings.ToLower(q["Name"].(string)), q["Type"], q["Class Base"])
	}
	return b.String()
}
func (a *binParser) dnsEvent(e *ProtocolEvent, w []byte) error {
	info, err := DecodeDNSMessage(w, a.budget.MaxCollectionElements)
	if err != nil {
		return err
	}
	return a.dnsEventDecoded(e, info)
}
func (a *binParser) dnsEventDecoded(e *ProtocolEvent, info map[string]any) error {
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
		s.pending = map[string]dnsPending{}
	}
	if e.Timestamp.After(s.clock) {
		s.clock = e.Timestamp
	}
	for k, v := range s.pending {
		if s.clock.Sub(v.ts) > 30*time.Second {
			a.buffered.Add(-int64(len(k) + 128))
			delete(s.pending, k)
		}
	}
	src, dst := e.Source, e.Destination
	if info["Response"] == true {
		src, dst = dst, src
	}
	key := fmt.Sprintf("%v/%s/%s/%s/%s", e.Domain, e.Transport, src, dst, dnsQuestionKey(info))
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	if info["Response"] == true {
		if p, ok := s.pending[key]; ok {
			e.ResponseTo, e.TransactionID = p.id, p.id
			e.Session["Association Status"] = "matched"
			if qs, ok := info["Questions"].([]map[string]any); ok && len(qs) > 0 {
				e.Session["Matched Request"] = qs[0]["Name"]
			}
			e.Session["Latency"] = e.Timestamp.Sub(p.ts).Seconds()
			e.Completeness = "transaction"
			a.buffered.Add(-int64(len(key) + 128))
			delete(s.pending, key)
		} else {
			e.Session["Association"] = "missing-request"
			e.Session["Association Status"] = "missing-request"
			e.Session["Unmatched"] = true
		}
	} else {
		if p, ok := s.pending[key]; ok {
			e.TransactionID = p.id
			e.Session["Retransmission"] = true
		} else if len(s.pending) < a.budget.MaxCollectionElements && a.reserveEvidence(int64(len(key)+128)) {
			e.TransactionID = e.ID
			s.pending[key] = dnsPending{e.ID, e.Timestamp}
		} else {
			e.Session["Association"] = "limited"
		}
	}
	return nil
}
