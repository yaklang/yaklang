// Package main builds and checks the Windows lab captures stacked on yaklang #5013.
package main

import (
	"encoding/binary"
	"fmt"
	"sort"
)

var (
	clientIP  = [4]byte{192, 0, 2, 10}
	serverIP  = [4]byte{192, 0, 2, 20}
	clientMAC = [6]byte{0x02, 0x00, 0x00, 0x5e, 0x00, 0x0a}
	serverMAC = [6]byte{0x02, 0x00, 0x00, 0x5e, 0x00, 0x14}
)

const (
	flagFIN = 0x01
	flagSYN = 0x02
	flagRST = 0x04
	flagPSH = 0x08
	flagACK = 0x10
)

type lab struct {
	frames [][]byte
	ipid   uint16
	ts     uint64
}

func newLab() *lab {
	return &lab{ipid: 1, ts: 1_758_585_600_000_000_000} // 2026-09-23T00:00:00Z
}

func (l *lab) pcapng() []byte { return writePcapng(l.frames, l.tsBase()) }

func (l *lab) tsBase() uint64 { return 1_758_585_600_000_000_000 }

func (l *lab) stamp() uint64 {
	l.ts += 1_000_000 // 1 ms
	return l.ts
}

type tcpConn struct {
	l          *lab
	cport      uint16
	sport      uint16
	cseq, sseq uint32
	split      int
}

func (l *lab) tcp(cport, sport uint16) *tcpConn {
	c := &tcpConn{l: l, cport: cport, sport: sport, cseq: 1000, sseq: 5000, split: 24}
	c.emit(true, flagSYN, nil)
	c.cseq++
	c.emit(false, flagSYN|flagACK, nil)
	c.sseq++
	c.emit(true, flagACK, nil)
	return c
}

func (c *tcpConn) client(p []byte) { c.send(true, p) }
func (c *tcpConn) server(p []byte) { c.send(false, p) }

func (c *tcpConn) send(fromClient bool, p []byte) {
	if len(p) == 0 {
		return
	}
	split := c.split
	if split <= 0 {
		split = len(p)
	}
	for len(p) > 0 {
		n := split
		if n > len(p) {
			n = len(p)
		}
		c.emit(fromClient, flagPSH|flagACK, p[:n])
		if fromClient {
			c.cseq += uint32(n)
		} else {
			c.sseq += uint32(n)
		}
		p = p[n:]
	}
}

func (c *tcpConn) close() {
	c.emit(true, flagFIN|flagACK, nil)
	c.cseq++
	c.emit(false, flagACK, nil)
	c.emit(false, flagFIN|flagACK, nil)
	c.sseq++
	c.emit(true, flagACK, nil)
}

func (l *lab) udp(cport, sport uint16, fromClient bool, payload []byte) {
	srcIP, dstIP := clientIP, serverIP
	sp, dp := cport, sport
	smac, dmac := clientMAC, serverMAC
	if !fromClient {
		srcIP, dstIP = serverIP, clientIP
		sp, dp = sport, cport
		smac, dmac = serverMAC, clientMAC
	}
	u := udpPacket(srcIP, dstIP, sp, dp, payload)
	l3 := ipv4Packet(srcIP, dstIP, 17, l.ipid, u)
	l.ipid++
	l.frames = append(l.frames, ethernet(smac, dmac, 0x0800, l3))
}

func (l *lab) ethernet(dst, src [6]byte, ethertype uint16, payload []byte) {
	l.frames = append(l.frames, ethernet(src, dst, ethertype, payload))
}

func ethernet(src, dst [6]byte, ethertype uint16, payload []byte) []byte {
	f := make([]byte, 14+len(payload))
	copy(f[0:6], dst[:])
	copy(f[6:12], src[:])
	binary.BigEndian.PutUint16(f[12:14], ethertype)
	copy(f[14:], payload)
	return f
}

func ipv4Packet(src, dst [4]byte, proto uint8, id uint16, payload []byte) []byte {
	h := make([]byte, 20+len(payload))
	h[0] = 0x45
	h[1] = 0
	binary.BigEndian.PutUint16(h[2:4], uint16(len(h)))
	binary.BigEndian.PutUint16(h[4:6], id)
	binary.BigEndian.PutUint16(h[6:8], 0x4000) // DF
	h[8] = 128
	h[9] = proto
	copy(h[12:16], src[:])
	copy(h[16:20], dst[:])
	binary.BigEndian.PutUint16(h[10:12], ipChecksum(h[:20]))
	copy(h[20:], payload)
	return h
}

func tcpSegment(sport, dport uint16, seq, ack uint32, flags byte, payload []byte) []byte {
	h := make([]byte, 20+len(payload))
	binary.BigEndian.PutUint16(h[0:2], sport)
	binary.BigEndian.PutUint16(h[2:4], dport)
	binary.BigEndian.PutUint32(h[4:8], seq)
	binary.BigEndian.PutUint32(h[8:12], ack)
	h[12] = 5 << 4
	h[13] = flags
	binary.BigEndian.PutUint16(h[14:16], 65535)
	copy(h[20:], payload)
	return h
}

func udpPacket(src, dst [4]byte, sport, dport uint16, payload []byte) []byte {
	h := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint16(h[0:2], sport)
	binary.BigEndian.PutUint16(h[2:4], dport)
	binary.BigEndian.PutUint16(h[4:6], uint16(len(h)))
	copy(h[8:], payload)
	csum := transportChecksum(src, dst, 17, h)
	if csum == 0 {
		csum = 0xffff
	}
	binary.BigEndian.PutUint16(h[6:8], csum)
	return h
}

func ipChecksum(h []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(h); i += 2 {
		if i == 10 {
			continue
		}
		sum += uint32(binary.BigEndian.Uint16(h[i:]))
	}
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return ^uint16(sum)
}

func transportChecksum(src, dst [4]byte, proto uint8, segment []byte) uint16 {
	sum := uint32(binary.BigEndian.Uint16(src[0:2])) + uint32(binary.BigEndian.Uint16(src[2:4]))
	sum += uint32(binary.BigEndian.Uint16(dst[0:2])) + uint32(binary.BigEndian.Uint16(dst[2:4]))
	sum += uint32(proto)
	sum += uint32(len(segment))
	tmp := segment
	if len(tmp)%2 == 1 {
		tmp = append(append([]byte{}, segment...), 0)
	}
	for i := 0; i+1 < len(tmp); i += 2 {
		if proto == 6 && i == 16 {
			continue // TCP checksum field
		}
		if proto == 17 && i == 6 {
			continue
		}
		sum += uint32(binary.BigEndian.Uint16(tmp[i:]))
	}
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return ^uint16(sum)
}

func finishTCP(src, dst [4]byte, seg []byte) {
	binary.BigEndian.PutUint16(seg[16:18], 0)
	binary.BigEndian.PutUint16(seg[16:18], transportChecksum(src, dst, 6, seg))
}

func (c *tcpConn) emit(fromClient bool, flags byte, payload []byte) {
	seq, ack := c.cseq, c.sseq
	srcIP, dstIP := clientIP, serverIP
	sport, dport := c.cport, c.sport
	smac, dmac := clientMAC, serverMAC
	if !fromClient {
		seq, ack = c.sseq, c.cseq
		srcIP, dstIP = serverIP, clientIP
		sport, dport = c.sport, c.cport
		smac, dmac = serverMAC, clientMAC
	}
	if flags&flagACK == 0 {
		ack = 0
	}
	seg := tcpSegment(sport, dport, seq, ack, flags, payload)
	finishTCP(srcIP, dstIP, seg)
	l3 := ipv4Packet(srcIP, dstIP, 6, c.l.ipid, seg)
	c.l.ipid++
	c.l.frames = append(c.l.frames, ethernet(smac, dmac, 0x0800, l3))
}

func writePcapng(frames [][]byte, ts0 uint64) []byte {
	var b []byte
	b = appendBlock(b, 0x0A0D0D0A, shbBody())
	b = appendBlock(b, 0x00000001, idbBody())
	ts := ts0
	for _, f := range frames {
		ts += 1_000_000
		b = appendBlock(b, 0x00000006, epbBody(ts, f))
	}
	return b
}

func appendBlock(dst []byte, typ uint32, body []byte) []byte {
	for len(body)%4 != 0 {
		body = append(body, 0)
	}
	total := uint32(12 + len(body))
	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[0:4], typ)
	binary.LittleEndian.PutUint32(hdr[4:8], total)
	dst = append(dst, hdr[:]...)
	dst = append(dst, body...)
	var tail [4]byte
	binary.LittleEndian.PutUint32(tail[:], total)
	return append(dst, tail[:]...)
}

func shbBody() []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b[0:4], 0x1A2B3C4D)
	binary.LittleEndian.PutUint16(b[4:6], 1)
	binary.LittleEndian.PutUint16(b[6:8], 0)
	binary.LittleEndian.PutUint64(b[8:16], 0xFFFFFFFFFFFFFFFF)
	// shb_userappl option 4
	opt := []byte("yaklang-winlab5013")
	b = append(b, optPair(4, opt)...)
	b = append(b, 0, 0, 0, 0)
	return b
}

func idbBody() []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint16(b[0:2], 1) // Ethernet
	binary.LittleEndian.PutUint16(b[2:4], 0)
	binary.LittleEndian.PutUint32(b[4:8], 65535)
	b = append(b, optPair(2, []byte("winlab"))...) // if_name
	b = append(b, optPair(9, []byte{9})...)        // nanoseconds
	b = append(b, 0, 0, 0, 0)
	return b
}

func optPair(code uint16, val []byte) []byte {
	b := make([]byte, 4+len(val))
	binary.LittleEndian.PutUint16(b[0:2], code)
	binary.LittleEndian.PutUint16(b[2:4], uint16(len(val)))
	copy(b[4:], val)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

func epbBody(ts uint64, frame []byte) []byte {
	b := make([]byte, 20+len(frame))
	binary.LittleEndian.PutUint32(b[0:4], 0)
	binary.LittleEndian.PutUint32(b[4:8], uint32(ts>>32))
	binary.LittleEndian.PutUint32(b[8:12], uint32(ts))
	binary.LittleEndian.PutUint32(b[12:16], uint32(len(frame)))
	binary.LittleEndian.PutUint32(b[16:20], uint32(len(frame)))
	copy(b[20:], frame)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	b = append(b, 0, 0, 0, 0) // end of options
	return b
}

// Frame is one decoded Ethernet frame from a pcapng section.
type Frame struct {
	EtherType uint16
	L3        []byte
	SrcMAC    [6]byte
	DstMAC    [6]byte
	IPProto   uint8
	SrcIP     [4]byte
	DstIP     [4]byte
	SrcPort   uint16
	DstPort   uint16
	TCPSeq    uint32
	TCPFlags  uint8
	L4        []byte
}

func parsePcapng(data []byte) ([]Frame, error) {
	if len(data) < 28 {
		return nil, fmt.Errorf("pcapng too short")
	}
	var out []Frame
	off := 0
	sawIDB := false
	for off < len(data) {
		if len(data)-off < 8 {
			return nil, fmt.Errorf("truncated block header")
		}
		typ := binary.LittleEndian.Uint32(data[off:])
		total := binary.LittleEndian.Uint32(data[off+4:])
		if total < 12 || int(total) > len(data)-off {
			return nil, fmt.Errorf("truncated block type %08x", typ)
		}
		body := data[off+8 : off+int(total)-4]
		tail := binary.LittleEndian.Uint32(data[off+int(total)-4:])
		if tail != total {
			return nil, fmt.Errorf("block length mismatch")
		}
		switch typ {
		case 0x0A0D0D0A:
			if len(body) < 16 || binary.LittleEndian.Uint32(body) != 0x1A2B3C4D {
				return nil, fmt.Errorf("not a little-endian pcapng")
			}
		case 0x00000001:
			if len(body) < 8 || binary.LittleEndian.Uint16(body) != 1 {
				return nil, fmt.Errorf("unsupported linktype")
			}
			sawIDB = true
		case 0x00000006:
			if !sawIDB {
				return nil, fmt.Errorf("packet before interface")
			}
			fr, err := parseEPB(body)
			if err != nil {
				return nil, err
			}
			out = append(out, fr)
		default:
			return nil, fmt.Errorf("unexpected block %08x", typ)
		}
		off += int(total)
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("not a multi-packet capture")
	}
	return out, nil
}

func parseEPB(body []byte) (Frame, error) {
	if len(body) < 20 {
		return Frame{}, fmt.Errorf("short epb")
	}
	caplen := binary.LittleEndian.Uint32(body[12:16])
	if int(caplen) > len(body)-20 {
		return Frame{}, fmt.Errorf("epb caplen past block")
	}
	raw := body[20 : 20+caplen]
	if len(raw) < 14 {
		return Frame{}, fmt.Errorf("short ethernet")
	}
	var fr Frame
	copy(fr.DstMAC[:], raw[0:6])
	copy(fr.SrcMAC[:], raw[6:12])
	fr.EtherType = binary.BigEndian.Uint16(raw[12:14])
	fr.L3 = append([]byte{}, raw[14:]...)
	if fr.EtherType != 0x0800 {
		return fr, nil
	}
	if len(fr.L3) < 20 || fr.L3[0]>>4 != 4 {
		return Frame{}, fmt.Errorf("not ipv4")
	}
	ihl := int(fr.L3[0]&0x0f) * 4
	if ihl < 20 || len(fr.L3) < ihl {
		return Frame{}, fmt.Errorf("bad ihl")
	}
	total := int(binary.BigEndian.Uint16(fr.L3[2:4]))
	if total < ihl || total > len(fr.L3) {
		return Frame{}, fmt.Errorf("bad ip length")
	}
	got := binary.BigEndian.Uint16(fr.L3[10:12])
	if ipChecksum(fr.L3[:ihl]) != got && !checksumOK(fr.L3[:ihl]) {
		return Frame{}, fmt.Errorf("bad ip checksum")
	}
	copy(fr.SrcIP[:], fr.L3[12:16])
	copy(fr.DstIP[:], fr.L3[16:20])
	fr.IPProto = fr.L3[9]
	l4 := fr.L3[ihl:total]
	switch fr.IPProto {
	case 6:
		if len(l4) < 20 {
			return Frame{}, fmt.Errorf("short tcp")
		}
		fr.SrcPort = binary.BigEndian.Uint16(l4[0:2])
		fr.DstPort = binary.BigEndian.Uint16(l4[2:4])
		fr.TCPSeq = binary.BigEndian.Uint32(l4[4:8])
		fr.TCPFlags = l4[13]
		doff := int(l4[12]>>4) * 4
		if doff < 20 || doff > len(l4) {
			return Frame{}, fmt.Errorf("bad tcp offset")
		}
		want := transportChecksum(fr.SrcIP, fr.DstIP, 6, l4)
		if binary.BigEndian.Uint16(l4[16:18]) != want {
			return Frame{}, fmt.Errorf("bad tcp checksum")
		}
		fr.L4 = append([]byte{}, l4[doff:]...)
	case 17:
		if len(l4) < 8 {
			return Frame{}, fmt.Errorf("short udp")
		}
		fr.SrcPort = binary.BigEndian.Uint16(l4[0:2])
		fr.DstPort = binary.BigEndian.Uint16(l4[2:4])
		ulen := int(binary.BigEndian.Uint16(l4[4:6]))
		if ulen < 8 || ulen > len(l4) {
			return Frame{}, fmt.Errorf("bad udp length")
		}
		got := binary.BigEndian.Uint16(l4[6:8])
		want := transportChecksum(fr.SrcIP, fr.DstIP, 17, l4[:ulen])
		if want == 0 {
			want = 0xffff
		}
		if got != want {
			return Frame{}, fmt.Errorf("bad udp checksum")
		}
		fr.L4 = append([]byte{}, l4[8:ulen]...)
	default:
		return Frame{}, fmt.Errorf("unsupported ip proto %d", fr.IPProto)
	}
	return fr, nil
}

func checksumOK(h []byte) bool {
	var sum uint32
	for i := 0; i+1 < len(h); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(h[i:]))
	}
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return sum == 0xffff
}

type chunk struct {
	seq  uint32
	data []byte
}

type conn struct {
	cport uint16
	c2s   []byte
	s2c   []byte
}

func tcpConns(frames []Frame, serverPort uint16) ([]conn, error) {
	type acc struct {
		cport uint16
		ctoS  []chunk
		stoC  []chunk
	}
	by := map[uint16]*acc{}
	var order []uint16
	for _, fr := range frames {
		if fr.IPProto != 6 {
			continue
		}
		var clientPort uint16
		var fromClient bool
		switch {
		case fr.DstPort == serverPort && fr.SrcPort != serverPort:
			clientPort = fr.SrcPort
			fromClient = true
		case fr.SrcPort == serverPort && fr.DstPort != serverPort:
			clientPort = fr.DstPort
			fromClient = false
		default:
			continue
		}
		a := by[clientPort]
		if a == nil {
			a = &acc{cport: clientPort}
			by[clientPort] = a
			order = append(order, clientPort)
		}
		if len(fr.L4) == 0 {
			continue
		}
		ch := chunk{seq: fr.TCPSeq, data: append([]byte{}, fr.L4...)}
		if fromClient {
			a.ctoS = append(a.ctoS, ch)
		} else {
			a.stoC = append(a.stoC, ch)
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	var out []conn
	for _, p := range order {
		a := by[p]
		c2s, err := reassemble(a.ctoS)
		if err != nil {
			return nil, err
		}
		s2c, err := reassemble(a.stoC)
		if err != nil {
			return nil, err
		}
		out = append(out, conn{cport: p, c2s: c2s, s2c: s2c})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no tcp conversation on port %d", serverPort)
	}
	return out, nil
}

func reassemble(chunks []chunk) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	sort.Slice(chunks, func(i, j int) bool {
		if chunks[i].seq == chunks[j].seq {
			return len(chunks[i].data) < len(chunks[j].data)
		}
		return chunks[i].seq < chunks[j].seq
	})
	next := chunks[0].seq
	var out []byte
	for _, c := range chunks {
		if len(c.data) == 0 {
			continue
		}
		end := c.seq + uint32(len(c.data))
		if c.seq > next {
			return nil, fmt.Errorf("tcp gap at %d", next)
		}
		if end <= next {
			continue
		}
		out = append(out, c.data[next-c.seq:]...)
		next = end
	}
	return out, nil
}

func udpByPort(frames []Frame, serverPort uint16) (c2s, s2c [][]byte, err error) {
	for _, fr := range frames {
		if fr.IPProto != 17 {
			continue
		}
		switch {
		case fr.DstPort == serverPort && fr.SrcPort != serverPort:
			c2s = append(c2s, append([]byte{}, fr.L4...))
		case fr.SrcPort == serverPort && fr.DstPort != serverPort:
			s2c = append(s2c, append([]byte{}, fr.L4...))
		}
	}
	if len(c2s) == 0 && len(s2c) == 0 {
		return nil, nil, fmt.Errorf("no udp on port %d", serverPort)
	}
	return c2s, s2c, nil
}
