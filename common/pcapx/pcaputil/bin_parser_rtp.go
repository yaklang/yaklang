package pcaputil

import (
	"encoding/binary"
	"fmt"
	"time"
)

// binRTP is the M0 session state for RFC 3550 RTP/RTCP. Ports 5004/5005
// are never consulted. A sequence gap is capture-missing, not network loss.
type binRTP struct {
	sources map[uint32]*rtpSource
	payload map[uint8]string
}

type rtpSource struct {
	ssrc     uint32
	maxSeq   uint16
	cycles   uint32
	received uint32
	lastTS   uint32
	lastArr  time.Time
	jitter   float64
	recent   []uint32
	init     bool
}

func probeRTP(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0]>>6 != 2 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if rtpIsRTCP(w) {
		if len(w) < 8 {
			return probeNeed("rtp", "3550", len(w), 8)
		}
		n := rtpRTCPSize(w)
		if n < 8 {
			return ProbeResult{Verdict: ProbeReject}
		}
		_ = limit
		return probeAccept("rtp", "3550", 90)
	}
	if len(w) < 12 {
		return probeNeed("rtp", "3550", len(w), 12)
	}
	if _, err := rtpHeaderSize(w); err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	// ONC RPC CALL/REPLY put 0 or 1 at this offset; RTP SSRC is not a 0/1 enum.
	if binary.BigEndian.Uint32(w[8:12]) <= 1 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("rtp", "3550", 88)
}

func rtpIsRTCP(w []byte) bool {
	if len(w) < 2 {
		return false
	}
	pt := w[1]
	return pt >= 200 && pt <= 204
}

func rtpRTCPSize(w []byte) int {
	if len(w) < 4 {
		return 0
	}
	return (int(binary.BigEndian.Uint16(w[2:4])) + 1) * 4
}

func rtpHeaderSize(w []byte) (int, error) {
	if len(w) < 12 {
		return 12, fmt.Errorf("rtp: truncated header")
	}
	if w[0]>>6 != 2 {
		return 0, fmt.Errorf("rtp: version must be 2")
	}
	cc := int(w[0] & 0x0f)
	n := 12 + 4*cc
	if w[0]&0x10 != 0 {
		if len(w) < n+4 {
			return n + 4, fmt.Errorf("rtp: truncated extension")
		}
		el := int(binary.BigEndian.Uint16(w[n+2 : n+4]))
		n += 4 + 4*el
	}
	return n, nil
}

func (f *binFlow) frameRTP(w []byte) (int, *binSpec, error) {
	r := f.rtp
	if r == nil {
		return 0, nil, sessionContext("RTP session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(r.sources))*96); err != nil {
		return 0, nil, err
	}
	if len(w) == 0 {
		return 0, nil, nil
	}
	if w[0]>>6 != 2 {
		return 0, nil, fmt.Errorf("rtp: version must be 2")
	}
	if rtpIsRTCP(w) {
		if len(w) < 4 {
			return 0, nil, nil
		}
		n := rtpRTCPSize(w)
		if n < 8 {
			return 0, nil, fmt.Errorf("rtcp: length too small")
		}
		if n > f.a.config.MaxMessageBytes {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		if n > len(w) {
			return n, nil, nil
		}
		return n, f.a.specs["rtp/RTCP"], nil
	}
	if len(w) < 12 {
		return 0, nil, nil
	}
	n, err := rtpHeaderSize(w)
	if err != nil {
		if len(w) < n {
			return 0, nil, nil
		}
		return 0, nil, err
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.a.specs["rtp/RTP"], nil
}

func (r *binRTP) consume(raw []byte, arr time.Time, max int) (map[string]any, error) {
	if len(raw) == 0 || raw[0]>>6 != 2 {
		return nil, protocolError(ErrUnsupportedVersion, "RTP/RTCP version must be 2")
	}
	if rtpIsRTCP(raw) {
		return r.consumeRTCP(raw)
	}
	return r.consumeRTP(raw, arr, max)
}

func (r *binRTP) consumeRTP(raw []byte, arr time.Time, max int) (map[string]any, error) {
	n, err := rtpHeaderSize(raw)
	if err != nil || len(raw) < n {
		return nil, fmt.Errorf("rtp: truncated packet")
	}
	pt := raw[1] & 0x7f
	seq := binary.BigEndian.Uint16(raw[2:4])
	ts := binary.BigEndian.Uint32(raw[4:8])
	ssrc := binary.BigEndian.Uint32(raw[8:12])
	info := map[string]any{
		"Packet Name":       "RTP",
		"Version":           2,
		"Marker":            raw[1]&0x80 != 0,
		"Payload Type":      pt,
		"Payload Type Name": rtpPayloadName(pt, r.payload),
		"Sequence":          seq,
		"Timestamp":         ts,
		"SSRC":              ssrc,
		"CSRC Count":        int(raw[0] & 0x0f),
		"Context Level":     "observed",
		"Network Loss":      false,
		"Loss Kind":         "none",
	}
	if max <= 0 {
		max = 4096
	}
	src := r.source(ssrc, max)
	if src == nil {
		return info, protocolError(ErrResourceExceeded, "RTP SSRC count exceeds budget")
	}
	for k, v := range src.observe(seq, ts, arr, rtpClock(pt)) {
		info[k] = v
	}
	info["Jitter"] = src.jitter
	info["Received"] = src.received
	return info, nil
}

func (r *binRTP) consumeRTCP(raw []byte) (map[string]any, error) {
	n := rtpRTCPSize(raw)
	if n < 8 || len(raw) < n {
		return nil, fmt.Errorf("rtcp: truncated packet")
	}
	pt := raw[1]
	ssrc := binary.BigEndian.Uint32(raw[4:8])
	rc := int(raw[0] & 0x1f)
	info := map[string]any{
		"Packet Name":   rtpRTCPName(pt),
		"Version":       2,
		"Packet Type":   pt,
		"SSRC":          ssrc,
		"Report Count":  rc,
		"Context Level": "observed",
		"Network Loss":  false,
		"Loss Kind":     "none",
	}
	if pt == 200 && n >= 28 {
		info["NTP MSW"] = binary.BigEndian.Uint32(raw[8:12])
		info["NTP LSW"] = binary.BigEndian.Uint32(raw[12:16])
		info["RTP Timestamp"] = binary.BigEndian.Uint32(raw[16:20])
		info["Packet Count"] = binary.BigEndian.Uint32(raw[20:24])
		info["Octet Count"] = binary.BigEndian.Uint32(raw[24:28])
	}
	var reports []map[string]any
	off := 8
	if pt == 200 {
		off = 28
	}
	for i := 0; i < rc && off+24 <= n; i++ {
		block := raw[off : off+24]
		src := binary.BigEndian.Uint32(block[0:4])
		frac := block[4]
		cum := uint32(block[5])<<16 | uint32(block[6])<<8 | uint32(block[7])
		rep := map[string]any{
			"Source SSRC":               src,
			"Fraction Lost":             frac,
			"Cumulative Packets Lost":   cum,
			"Extended Highest Sequence": binary.BigEndian.Uint32(block[8:12]),
			"Interarrival Jitter":       binary.BigEndian.Uint32(block[12:16]),
			"Last SR":                   binary.BigEndian.Uint32(block[16:20]),
			"Delay Since Last SR":       binary.BigEndian.Uint32(block[20:24]),
			"Associated RTP":            r.sources[src] != nil,
			"Reported Network Loss":     frac > 0 || cum > 0,
		}
		reports = append(reports, rep)
		off += 24
	}
	if len(reports) > 0 {
		info["Report Blocks"] = reports
	}
	if _, ok := r.sources[ssrc]; ok {
		info["Associated RTP"] = true
	}
	return info, nil
}

func (r *binRTP) source(ssrc uint32, max int) *rtpSource {
	if r.sources == nil {
		r.sources = map[uint32]*rtpSource{}
	}
	if s := r.sources[ssrc]; s != nil {
		return s
	}
	if len(r.sources) >= max {
		return nil
	}
	s := &rtpSource{ssrc: ssrc}
	r.sources[ssrc] = s
	return s
}

func (s *rtpSource) observe(seq uint16, ts uint32, arr time.Time, clock int) map[string]any {
	info := map[string]any{}
	if !s.init {
		s.init = true
		s.maxSeq = seq
		s.lastTS = ts
		s.lastArr = arr
		s.received = 1
		s.recent = []uint32{uint32(seq)}
		return info
	}
	ext := s.cycles | uint32(seq)
	if seq < s.maxSeq && uint16(s.maxSeq-seq) > 0x8000 {
		s.cycles += 0x10000
		ext = s.cycles | uint32(seq)
	} else if seq > s.maxSeq && uint16(seq-s.maxSeq) > 0x8000 {
		ext = s.cycles - 0x10000 | uint32(seq)
	}
	for _, prev := range s.recent {
		if prev == ext {
			info["Duplicate"] = true
			s.received++
			return info
		}
	}
	maxExt := s.cycles | uint32(s.maxSeq)
	if ext+0x10000 == maxExt+1 && seq < s.maxSeq {
		maxExt = s.cycles - 0x10000 | uint32(s.maxSeq)
	}
	if ext > maxExt+1 {
		info["Gap"] = true
		info["Missing Count"] = ext - maxExt - 1
		info["Capture Missing"] = true
		info["Network Loss"] = false
		info["Loss Kind"] = "capture-missing"
	} else if ext < maxExt {
		info["Out of Order"] = true
	}
	if ext >= maxExt {
		if seq < s.maxSeq && uint16(s.maxSeq-seq) > 0x8000 {
			// wrap already accounted
		} else if seq < s.maxSeq && ext > maxExt {
			s.cycles += 0x10000
		}
		if ext >= (s.cycles | uint32(s.maxSeq)) {
			s.maxSeq = seq
		}
	}
	if clock > 0 && !s.lastArr.IsZero() {
		arrivalDelta := arr.Sub(s.lastArr).Seconds()
		tsDelta := float64(int32(ts-s.lastTS)) / float64(clock)
		d := arrivalDelta - tsDelta
		if d < 0 {
			d = -d
		}
		s.jitter += (d - s.jitter) / 16
	}
	s.lastTS = ts
	s.lastArr = arr
	s.received++
	s.recent = append(s.recent, ext)
	if len(s.recent) > 128 {
		s.recent = s.recent[len(s.recent)-128:]
	}
	return info
}

func rtpPayloadName(pt uint8, dynamic map[uint8]string) string {
	if dynamic != nil {
		if n, ok := dynamic[pt]; ok {
			return n
		}
	}
	switch pt {
	case 0:
		return "PCMU"
	case 3:
		return "GSM"
	case 4:
		return "G723"
	case 8:
		return "PCMA"
	case 9:
		return "G722"
	case 18:
		return "G729"
	case 26:
		return "JPEG"
	case 31:
		return "H261"
	case 32:
		return "MPV"
	case 33:
		return "MP2T"
	case 34:
		return "H263"
	}
	if pt >= 96 {
		return fmt.Sprintf("dynamic-%d", pt)
	}
	return fmt.Sprintf("PT-%d", pt)
}

func rtpClock(pt uint8) int {
	switch pt {
	case 26, 31, 32, 33, 34:
		return 90000
	}
	if pt >= 96 {
		return 90000
	}
	return 8000
}

func rtpRTCPName(pt byte) string {
	switch pt {
	case 200:
		return "SR"
	case 201:
		return "RR"
	case 202:
		return "SDES"
	case 203:
		return "BYE"
	case 204:
		return "APP"
	}
	return fmt.Sprintf("RTCP-%d", pt)
}

func rtpApplySDPMap(r *binRTP, maps map[uint8]string) {
	if r.payload == nil {
		r.payload = map[uint8]string{}
	}
	for k, v := range maps {
		r.payload[k] = v
	}
}
