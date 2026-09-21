package pcaputil

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// binDoT is the M0 session state for RFC 7858 DNS-over-TLS on TLS
// plaintext. Framing is the DNS-over-TCP 2-byte length prefix (RFC 1035
// 4.2.2 / RFC 7766). Ports 853/53 are never consulted.
type binDoT struct {
	pending map[uint16]string
}

func probeDoT(w []byte, limit int) ProbeResult {
	if len(w) < 2 {
		if len(w) == 1 && w[0] == 0 {
			return probeNeed("dot", "rfc7858", 1, 14)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	n := int(binary.BigEndian.Uint16(w[:2]))
	if n < 12 {
		return ProbeResult{Verdict: ProbeReject}
	}
	// DNS-over-TCP length is almost always < 256 for the first query.
	// Larger prefixes (AMQP "AM", NFS 0x80, RDP TPKT 0x03, TLS 0x16) must Reject.
	if len(w) < 14 {
		if w[0] != 0 {
			return ProbeResult{Verdict: ProbeReject}
		}
		return probeNeed("dot", "rfc7858", len(w), 14)
	}
	if n > 4096 {
		return ProbeResult{Verdict: ProbeReject}
	}
	h := w[2:]
	if !dnsHeader(h) {
		return ProbeResult{Verdict: ProbeReject}
	}
	qd := binary.BigEndian.Uint16(h[4:6])
	if qd < 1 || qd > 8 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(h) > 12 {
		lab := h[12]
		if lab > 63 && lab < 0xc0 {
			return ProbeResult{Verdict: ProbeReject}
		}
	}
	_ = limit
	return probeAccept("dot", "rfc7858", 90)
}

func (f *binFlow) frameDoT(w []byte) (int, *binSpec, error) {
	d := f.dot
	if d == nil {
		return 0, nil, sessionContext("DoT session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(d.pending))*16); err != nil {
		return 0, nil, err
	}
	if len(w) < 2 {
		return 0, nil, nil
	}
	n := 2 + int(binary.BigEndian.Uint16(w[:2]))
	if n < 14 {
		return 0, nil, fmt.Errorf("dot: DNS length %d is shorter than the 12-byte header", n-2)
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("dns", "DNS"), nil
}

func (d *binDoT) consume(raw []byte, max int) (map[string]any, error) {
	if len(raw) < 14 {
		return nil, fmt.Errorf("dot: truncated DNS message")
	}
	n := 2 + int(binary.BigEndian.Uint16(raw[:2]))
	if n != len(raw) {
		return nil, fmt.Errorf("dot: length prefix disagrees with framed message")
	}
	msg := raw[2:]
	id := binary.BigEndian.Uint16(msg[0:2])
	flags := binary.BigEndian.Uint16(msg[2:4])
	qd := binary.BigEndian.Uint16(msg[4:6])
	an := binary.BigEndian.Uint16(msg[6:8])
	qr := flags>>15 != 0
	opcode := (flags >> 11) & 0xf
	rcode := flags & 0xf
	qname, qtype, err := dnsQuestion(msg)
	if err != nil {
		return nil, err
	}
	info := map[string]any{
		"Transaction ID": id,
		"QR":             qr,
		"Opcode":         opcode,
		"RCODE":          rcode,
		"Questions":      qd,
		"Answer RRs":     an,
		"QNAME":          qname,
		"QTYPE":          qtype,
		"QTYPE Name":     dnsTypeName(qtype),
		"Context Level":  "observed",
	}
	if !qr {
		info["Packet Name"] = "Query"
		if max <= 0 {
			max = 4096
		}
		if len(d.pending) >= max {
			return info, protocolError(ErrResourceExceeded, "DoT outstanding transaction IDs exceed budget")
		}
		d.pending[id] = qname
		info["Outstanding"] = true
		return info, nil
	}
	info["Packet Name"] = "Response"
	if want, ok := d.pending[id]; ok {
		delete(d.pending, id)
		info["Matched Request"] = want
		info["Association Status"] = "matched"
	} else {
		info["Unmatched"] = true
		info["Association Status"] = "missing-request"
		info["Context Level"] = "partial"
	}
	if addrs := dnsARecords(msg); len(addrs) > 0 {
		info["A Records"] = addrs
	}
	return info, nil
}

func dnsQuestion(msg []byte) (name string, qtype uint16, err error) {
	if len(msg) < 12 {
		return "", 0, fmt.Errorf("dot: truncated DNS header")
	}
	name, at, err := dnsParseName(msg, 12)
	if err != nil {
		return "", 0, err
	}
	if at+4 > len(msg) {
		return "", 0, fmt.Errorf("dot: truncated question")
	}
	return name, binary.BigEndian.Uint16(msg[at : at+2]), nil
}

func dnsParseName(msg []byte, at int) (string, int, error) {
	var labels []string
	next := at
	jumped := false
	var targets [128]int // cycles require a repeated compression-pointer target
	expanded := 1        // terminal root label counts toward RFC 1035's 255 octets
	jumps := 0
	for {
		if at < 0 || at >= len(msg) {
			return "", 0, fmt.Errorf("dot: truncated QNAME")
		}
		l := int(msg[at])
		if l == 0 {
			if !jumped {
				next = at + 1
			}
			return strings.Join(labels, "."), next, nil
		}
		if l >= 0xc0 {
			if jumps == len(targets) {
				return "", 0, protocolError(ErrResourceExceeded, "DNS pointer budget")
			}
			if at+1 >= len(msg) {
				return "", 0, fmt.Errorf("dot: truncated name pointer")
			}
			if !jumped {
				next = at + 2
				jumped = true
			}
			at = (l&0x3f)<<8 | int(msg[at+1])
			for _, target := range targets[:jumps] {
				if target == at {
					return "", 0, fmt.Errorf("dot: DNS name pointer loop")
				}
			}
			targets[jumps] = at
			jumps++
			continue
		}
		if l > 63 {
			return "", 0, fmt.Errorf("dot: invalid DNS label length")
		}
		at++
		if at+l > len(msg) {
			return "", 0, fmt.Errorf("dot: truncated DNS label")
		}
		expanded += l + 1
		if expanded > 255 {
			return "", 0, fmt.Errorf("dot: DNS name exceeds 255 octets")
		}
		labels = append(labels, string(msg[at:at+l]))
		at += l
	}
}

func dnsARecords(msg []byte) []string {
	if len(msg) < 12 {
		return nil
	}
	at := 12
	qd := int(binary.BigEndian.Uint16(msg[4:6]))
	an := int(binary.BigEndian.Uint16(msg[6:8]))
	for i := 0; i < qd; i++ {
		_, next, err := dnsParseName(msg, at)
		if err != nil || next+4 > len(msg) {
			return nil
		}
		at = next + 4
	}
	var addrs []string
	for i := 0; i < an && at+10 <= len(msg); i++ {
		_, next, err := dnsParseName(msg, at)
		if err != nil || next+10 > len(msg) {
			return addrs
		}
		typ := binary.BigEndian.Uint16(msg[next : next+2])
		rdlen := int(binary.BigEndian.Uint16(msg[next+8 : next+10]))
		at = next + 10
		if at+rdlen > len(msg) {
			return addrs
		}
		if typ == 1 && rdlen == 4 {
			addrs = append(addrs, fmt.Sprintf("%d.%d.%d.%d", msg[at], msg[at+1], msg[at+2], msg[at+3]))
		}
		at += rdlen
	}
	return addrs
}

func dnsTypeName(t uint16) string {
	switch t {
	case 1:
		return "A"
	case 2:
		return "NS"
	case 5:
		return "CNAME"
	case 12:
		return "PTR"
	case 16:
		return "TXT"
	case 28:
		return "AAAA"
	}
	return fmt.Sprintf("TYPE%d", t)
}
