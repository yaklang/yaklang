package pcaputil

import (
	"encoding/binary"
)

// doqStreamState is RFC 9250 DNS-over-QUIC on a client-initiated
// bidirectional QUIC stream. Framing is the 2-byte DNS-over-TCP length
// prefix. Ports 853/443 are never consulted.
type doqStreamState struct {
	buf       [2][]byte
	parsed    [2]int
	queryID   uint16
	qname     string
	haveQuery bool
	fin       [2]bool
}

func (q *binQUIC) feedDoQ(dir int, sid, off uint64, data []byte, fin bool, max int, fr, info map[string]any) error {
	if sid&0x3 != 0 {
		return nil
	}
	st := q.stream(sid, max)
	if st == nil {
		return nil
	}
	if st.h3 != nil && st.h3.kind != "" && st.h3.kind != "unknown" {
		return nil
	}
	if dir != 0 && dir != 1 {
		dir = 0
	}
	if st.doq == nil {
		st.doq = &doqStreamState{}
	}
	d := st.doq
	if off != uint64(len(d.buf[dir])) {
		return nil
	}
	if len(d.buf[dir])+len(data) > 1<<20 {
		return protocolError(ErrResourceExceeded, "DoQ stream exceeds 1 MiB")
	}
	d.buf[dir] = append(d.buf[dir], data...)
	buf := d.buf[dir][d.parsed[dir]:]
	if q.native && fin {
		d.fin[dir] = true
	}
	if len(buf) < 2 {
		if q.native && fin {
			return protocolError(ErrNeedMore, "DoQ FIN before length")
		}
		return nil
	}
	n := int(binary.BigEndian.Uint16(buf[:2]))
	if n < 12 || n > 65535 {
		if q.native || d.haveQuery {
			return protocolError(ErrMalformedMessage, "doq: DNS length %d is invalid", n)
		}
		return nil
	}
	need := 2 + n
	if need > 1<<20 {
		return protocolError(ErrResourceExceeded, "DoQ message exceeds 1 MiB")
	}
	if len(buf) < need {
		if q.native && fin {
			return protocolError(ErrNeedMore, "DoQ FIN before complete DNS message")
		}
		return nil
	}
	if q.native {
		if d.parsed[dir] != 0 || len(buf) > need {
			return protocolError(ErrMalformedMessage, "DoQ permits one message per direction")
		}
		if !fin {
			info["DoQ Incomplete"] = true
			return nil
		}
	}
	msg := buf[2:need]
	if !dnsHeader(msg) {
		if q.native || d.haveQuery {
			return protocolError(ErrMalformedMessage, "doq: framed payload is not a DNS header")
		}
		return nil
	}
	id := binary.BigEndian.Uint16(msg[0:2])
	flags := binary.BigEndian.Uint16(msg[2:4])
	qd := binary.BigEndian.Uint16(msg[4:6])
	an := binary.BigEndian.Uint16(msg[6:8])
	qr := flags>>15 != 0
	if q.native && (id != 0 || qr != (dir == 1)) {
		return protocolError(ErrMalformedMessage, "DoQ DNS ID or direction invalid")
	}
	qname, qtype, err := dnsQuestion(msg)
	if err != nil {
		if q.native || d.haveQuery {
			return err
		}
		return nil
	}
	var semantic map[string]any
	if q.native {
		var err error
		semantic, err = DecodeDNSMessage(msg, max)
		if err != nil {
			return err
		}
		if qd != 1 {
			return protocolError(ErrMalformedMessage, "DoQ requires one question")
		}
	}
	d.parsed[dir] += need
	dns := map[string]any{
		"Transaction ID": id,
		"QR":             qr,
		"Opcode":         (flags >> 11) & 0xf,
		"RCODE":          flags & 0xf,
		"Questions":      qd,
		"Answer RRs":     an,
		"QNAME":          qname,
		"QTYPE":          qtype,
		"QTYPE Name":     dnsTypeName(qtype),
		"Context Level":  "observed",
	}
	if semantic != nil {
		dns["DNS"] = semantic
	}
	info["DoQ"] = true
	info["Protocol Transition"] = "quic->doq"
	info["DoQ Stream ID"] = sid
	fr["DoQ"] = true
	if !qr {
		dns["Packet Name"] = "Query"
		d.haveQuery = true
		d.queryID = id
		d.qname = qname
		dns["Outstanding"] = true
		if max <= 0 {
			max = 4096
		}
	} else {
		dns["Packet Name"] = "Response"
		if d.haveQuery && d.queryID == id && (!q.native || d.qname == qname) {
			dns["Matched Request"] = d.qname
			dns["Association Status"] = "matched"
		} else {
			dns["Unmatched"] = true
			dns["Association Status"] = "missing-request"
			dns["Context Level"] = "partial"
			info["DoQ"] = true
			info["DoQ Message"] = dns
			info["QNAME"] = qname
			info["Transaction ID"] = id
			info["Packet Name"] = "Response"
			info["Association Status"] = "missing-request"
			if fin {
				info["DoQ Stream State"] = "half-closed"
			}
			return protocolError(ErrContextRequired, "doq: response without a query on this stream")
		}
		if addrs := dnsARecords(msg); len(addrs) > 0 {
			dns["A Records"] = addrs
		}
	}
	info["DoQ Message"] = dns
	info["Packet Name"] = dns["Packet Name"]
	info["QNAME"] = qname
	info["Transaction ID"] = id
	info["QTYPE Name"] = dns["QTYPE Name"]
	if addrs, ok := dns["A Records"]; ok {
		info["A Records"] = addrs
	}
	if status, ok := dns["Association Status"]; ok {
		info["Association Status"] = status
		info["Matched Request"] = dns["Matched Request"]
	}
	if fin {
		state := "half-closed"
		if (!q.native && st.fin) || (q.native && d.fin[0] && d.fin[1]) {
			state = "closed"
		}
		info["DoQ Stream State"] = state
	}
	return nil
}
