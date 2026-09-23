package pcaputil

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

type rtspKey struct {
	dir int
	seq uint32
}
type rtspInterleavedOffer struct {
	valid     bool
	profile   string
	rtp, rtcp byte
}
type rtspRequest struct {
	method, session  string
	hash             [32]byte
	udpOffer         rtspUDPTransportOffer
	interleavedOffer rtspInterleavedOffer
	eventIDs         []uint64
	packetRefs       []PacketReference
	evidenceCost     int64
	owner            *binRTSP
}
type rtspChannel struct {
	session string
	rtcp    bool
}
type binRTSP struct {
	pending                map[rtspKey]*rtspRequest
	sessions               map[string]string
	channels               map[byte]rtspChannel
	media                  *binRTP
	requestEvidenceBytes   int64
	reserveRequestEvidence func(int64) error
}

const rtspInternalRequestEvidenceKey = "_rtsp_request_evidence"

func parseRTSPChannelPair(value string) (byte, byte, bool) {
	pair := strings.Split(value, "-")
	if len(pair) != 2 || pair[0] == "" || pair[1] == "" {
		return 0, 0, false
	}
	a, errA := strconv.ParseUint(pair[0], 10, 8)
	b, errB := strconv.ParseUint(pair[1], 10, 8)
	if errA != nil || errB != nil || a == b {
		return 0, 0, false
	}
	return byte(a), byte(b), true
}

func parseRTSPInterleavedOffer(values []string) rtspInterleavedOffer {
	fields, err := parseRTSPTransportFields(values)
	if err != nil || fields.profile != "RTP/AVP/TCP" || fields.flags["multicast"] || fields.flags["rtcp-mux"] {
		return rtspInterleavedOffer{}
	}
	value, ok := fields.params["interleaved"]
	if !ok {
		return rtspInterleavedOffer{}
	}
	rtp, rtcp, ok := parseRTSPChannelPair(value)
	if !ok {
		return rtspInterleavedOffer{}
	}
	return rtspInterleavedOffer{valid: true, profile: fields.profile, rtp: rtp, rtcp: rtcp}
}

func resolveRTSPInterleavedTransport(offer rtspInterleavedOffer, values []string) (string, byte, byte, bool) {
	if len(values) == 0 {
		return "transport-not-observed", 0, 0, false
	}
	fields, err := parseRTSPTransportFields(values)
	if err != nil {
		return "ambiguous-or-invalid-transport", 0, 0, false
	}
	if fields.profile != "RTP/AVP/TCP" {
		return "non-interleaved-transport", 0, 0, false
	}
	if !offer.valid {
		return "request-interleaved-offer-not-observed", 0, 0, false
	}
	if fields.flags["multicast"] || fields.flags["rtcp-mux"] {
		return "unsupported-interleaved-transport-mode", 0, 0, false
	}
	value, ok := fields.params["interleaved"]
	if !ok {
		return "interleaved-channels-not-observed", 0, 0, false
	}
	rtp, rtcp, ok := parseRTSPChannelPair(value)
	if !ok {
		return "invalid-interleaved-channel-pair", 0, 0, false
	}
	if fields.profile != offer.profile || rtp != offer.rtp || rtcp != offer.rtcp {
		return "interleaved-channel-offer-mismatch", 0, 0, false
	}
	return "matched", rtp, rtcp, true
}

func (s *binRTSP) recordRequestEvidence(req *rtspRequest, e *ProtocolEvent, max int) {
	if req == nil || e == nil {
		return
	}
	newIDs := make([]uint64, 0, 1)
	if e.ID != 0 {
		found := false
		for _, id := range req.eventIDs {
			found = found || id == e.ID
		}
		if !found {
			newIDs = append(newIDs, e.ID)
		}
	}
	newRefs := make([]PacketReference, 0, len(e.SourceBytes.PacketRefs))
	for _, ref := range e.SourceBytes.PacketRefs {
		found := false
		for _, existing := range req.packetRefs {
			if existing == ref {
				found = true
				break
			}
		}
		for _, candidate := range newRefs {
			if candidate == ref {
				found = true
				break
			}
		}
		if !found {
			newRefs = append(newRefs, ref)
		}
	}
	if max > 0 && (len(req.eventIDs)+len(newIDs) > max || len(req.packetRefs)+len(newRefs) > max) {
		e.Session["RTSP Request Evidence Status"] = "resource-limit"
		return
	}
	delta := int64(len(newIDs))*8 + int64(len(newRefs))*24
	if delta > 0 && req.owner != nil && req.owner.reserveRequestEvidence != nil {
		if err := req.owner.reserveRequestEvidence(delta); err != nil {
			e.Session["RTSP Request Evidence Status"] = "resource-limit"
			return
		}
		req.owner.requestEvidenceBytes += delta
	}
	if len(newIDs) > 0 {
		retained := make([]uint64, len(req.eventIDs)+len(newIDs))
		copy(retained, req.eventIDs)
		copy(retained[len(req.eventIDs):], newIDs)
		req.eventIDs = retained
	}
	if len(newRefs) > 0 {
		retained := make([]PacketReference, len(req.packetRefs)+len(newRefs))
		copy(retained, req.packetRefs)
		copy(retained[len(req.packetRefs):], newRefs)
		req.packetRefs = retained
	}
	req.evidenceCost += delta
	e.Session["RTSP Request Evidence Status"] = "retained"
}

var rtspMethods = []string{"OPTIONS", "DESCRIBE", "SETUP", "PLAY", "PAUSE", "TEARDOWN", "GET_PARAMETER", "SET_PARAMETER", "ANNOUNCE", "RECORD", "REDIRECT"}

func probeRTSP(w []byte, limit int) ProbeResult {
	if bytes.HasPrefix(w, []byte("RTSP/")) {
		return probeAccept("rtsp", "1.0", 99)
	}
	for _, m := range rtspMethods {
		prefix := m + " "
		if !bytes.HasPrefix(w, []byte(prefix)) {
			continue
		}
		rest := strings.ToLower(string(w[len(prefix):]))
		if strings.HasPrefix(rest, "rtsp://") || strings.HasPrefix(rest, "rtsps://") || strings.Contains(rest, " RTSP/1.0") || strings.Contains(rest, " rtsp/1.0") {
			return probeAccept("rtsp", "1.0", 98)
		}
		if strings.Contains(rest, "\r\n") || strings.HasPrefix(rest, "sip:") || strings.HasPrefix(rest, "sips:") || strings.Contains(rest, " http/") || strings.Contains(rest, " sip/") {
			return ProbeResult{Verdict: ProbeReject}
		}
		if len(w) < limit {
			return probeNeed("rtsp", "1.0", len(w), limit)
		}
	}
	return ProbeResult{Verdict: ProbeReject}
}
func rtspHeader(w []byte, max int) (string, textproto.MIMEHeader, int, int, error) {
	i := bytes.Index(w, []byte("\r\n\r\n"))
	if i < 0 {
		if len(w) > 32<<10 {
			return "", nil, 0, 0, protocolError(ErrResourceExceeded, "RTSP headers")
		}
		return "", nil, 0, 0, nil
	}
	if i > 32<<10 {
		return "", nil, 0, 0, protocolError(ErrResourceExceeded, "RTSP headers")
	}
	first := bytes.Index(w[:i+2], []byte("\r\n"))
	line := string(w[:first])
	h, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(w[first+2 : i+4]))).ReadMIMEHeader()
	if err != nil {
		return "", nil, 0, 0, err
	}
	count := 0
	for _, v := range h {
		count += len(v)
	}
	if count > max {
		return "", nil, 0, 0, protocolError(ErrResourceExceeded, "RTSP header count")
	}
	n := uint64(0)
	if v := h.Values("Content-Length"); len(v) > 0 {
		if len(v) != 1 {
			return "", nil, 0, 0, fmt.Errorf("rtsp: duplicate Content-Length")
		}
		n, err = strconv.ParseUint(strings.TrimSpace(v[0]), 10, 31)
		if err != nil {
			return "", nil, 0, 0, fmt.Errorf("rtsp: invalid Content-Length")
		}
	}
	if h.Get("Transfer-Encoding") != "" {
		return "", nil, 0, 0, protocolError(ErrUnsupportedFeature, "RTSP transfer encoding")
	}
	return line, h, i + 4, int(n), nil
}
func (f *binFlow) frameRTSP(w []byte) (int, *binSpec, error) {
	s := f.rtsp
	mediaBytes := int64(512)
	if s.media != nil {
		s.media.maxBufferedBytes = f.a.config.MaxBufferedBytes
		mediaBytes = s.media.retainedBytes()
	}
	rtspBase := int64(1024 + len(s.pending)*512 + len(s.sessions)*320 + len(s.channels)*128)
	s.reserveRequestEvidence = func(delta int64) error {
		return f.reserveSession(rtspBase + mediaBytes + s.requestEvidenceBytes + delta)
	}
	if s.media != nil {
		s.media.reserveSessionMemory = func(target int64) error {
			return f.reserveSession(rtspBase + s.requestEvidenceBytes + target)
		}
	}
	if err := f.reserveSession(rtspBase + 512 + 320 + 2*128 + s.requestEvidenceBytes + mediaBytes); err != nil {
		return 0, nil, err
	}
	if len(w) > 0 && w[0] == '$' {
		if len(w) < 4 {
			return 0, nil, nil
		}
		return 4 + int(binary.BigEndian.Uint16(w[2:])), f.a.specs["rtsp_session/Interleaved"], nil
	}
	_, _, hdr, n, err := rtspHeader(w, f.a.budget.MaxCollectionElements)
	if err != nil || hdr == 0 {
		return 0, nil, err
	}
	return hdr + n, f.a.specs["rtsp_session/RTSP"], nil
}
func (s *binRTSP) consume(dir int, w []byte, ts time.Time, max int) (map[string]any, error) {
	if s.pending == nil {
		s.pending = map[rtspKey]*rtspRequest{}
		s.sessions = map[string]string{}
		s.channels = map[byte]rtspChannel{}
		s.media = &binRTP{sources: map[uint32]*rtpSource{}}
	}
	if len(w) > 0 && w[0] == '$' {
		if len(w) < 4 || len(w) != 4+int(binary.BigEndian.Uint16(w[2:])) {
			return nil, fmt.Errorf("rtsp: interleaved length")
		}
		ch, ok := s.channels[w[1]]
		out := map[string]any{"Packet Name": "Interleaved", "Channel": w[1], "Matched": ok}
		if !ok {
			out["Context Missing"] = "SETUP channel was not observed"
			return out, nil
		}
		out["Session ID"] = ch.session
		if ch.rtcp {
			media, err := s.media.consumeRTCPCompound(w[4:], max)
			out["Media"] = media
			return out, err
		}
		_, pt, err := inspectRTPDatagramAs(w[4:], 1<<20, max, false)
		if err != nil {
			return nil, err
		}
		media, err := s.media.consumeRTP(w[4:], ts, max)
		if err == nil {
			media["Channel Payload Type"] = pt
		}
		out["Media"] = media
		return out, err
	}
	line, h, hdr, n, err := rtspHeader(w, max)
	if err != nil {
		return nil, err
	}
	if hdr == 0 || hdr+n != len(w) {
		return nil, fmt.Errorf("rtsp: body boundary")
	}
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("rtsp: first line")
	}
	response := strings.HasPrefix(parts[0], "RTSP/")
	version := parts[2]
	if response {
		version = parts[0]
	}
	if version != "RTSP/1.0" {
		return nil, protocolError(ErrUnsupportedVersion, "RTSP version")
	}
	seqs := h.Values("Cseq")
	if len(seqs) != 1 {
		return nil, fmt.Errorf("rtsp: missing/duplicate CSeq")
	}
	seq, err := strconv.ParseUint(seqs[0], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("rtsp: CSeq")
	}
	sid := strings.TrimSpace(strings.SplitN(h.Get("Session"), ";", 2)[0])
	if len(sid) > 256 {
		return nil, protocolError(ErrResourceExceeded, "RTSP session identifier")
	}
	sid = strings.Clone(sid)
	out := map[string]any{"CSeq": uint32(seq), "Session ID": sid, "Content Length": n, "Content Type": h.Get("Content-Type")}
	key := rtspKey{dir, uint32(seq)}
	if !response {
		known := false
		for _, m := range rtspMethods {
			if parts[0] == m {
				known = true
			}
		}
		if !known {
			return nil, protocolError(ErrUnsupportedFeature, "RTSP method")
		}
		var request *rtspRequest
		if old, ok := s.pending[key]; ok {
			if old.hash != sha256.Sum256(w) {
				return nil, protocolError(ErrDesynchronized, "RTSP CSeq reused")
			}
			request = old
			out["Retransmission"] = true
		} else if len(s.pending) >= max {
			return nil, protocolError(ErrResourceExceeded, "RTSP pending requests")
		}
		if request == nil {
			request = &rtspRequest{method: strings.Clone(parts[0]), session: strings.Clone(sid), hash: sha256.Sum256(w), owner: s}
			if parts[0] == "SETUP" {
				request.udpOffer = parseRTSPClientTransportOffer(h.Values("Transport"))
				request.interleavedOffer = parseRTSPInterleavedOffer(h.Values("Transport"))
			}
		}
		s.pending[key] = request
		out["Packet Name"] = parts[0]
		out["URI"] = parts[1]
		if request.method == "SETUP" {
			out[rtspInternalRequestEvidenceKey] = request
		}
	} else {
		status, err := strconv.Atoi(parts[1])
		if err != nil || status < 100 || status > 599 {
			return nil, fmt.Errorf("rtsp: status")
		}
		key.dir = 1 - dir
		req, ok := s.pending[key]
		out["Packet Name"] = "Response"
		out["Status"] = status
		out["Matched"] = ok
		if ok {
			out["In Reply To"] = req.method
			if sid == "" {
				sid = req.session
				out["Session ID"] = sid
			}
			if req.session != "" && sid != req.session {
				return nil, protocolError(ErrDesynchronized, "RTSP response Session differs")
			}
			if status >= 200 && status < 300 && sid != "" {
				if _, exists := s.sessions[sid]; !exists && len(s.sessions) >= max {
					return nil, protocolError(ErrResourceExceeded, "RTSP session budget")
				}
				switch req.method {
				case "SETUP":
					interleavedStatus, rtpChannel, rtcpChannel, matched := resolveRTSPInterleavedTransport(req.interleavedOffer, h.Values("Transport"))
					out["Interleaved Transport Status"] = interleavedStatus
					if matched {
						for _, channel := range []byte{rtpChannel, rtcpChannel} {
							if _, exists := s.channels[channel]; exists {
								return nil, protocolError(ErrDesynchronized, "RTSP channel already assigned")
							}
						}
						if len(s.channels)+2 > max {
							return nil, protocolError(ErrResourceExceeded, "RTSP channels")
						}
						s.channels[rtpChannel] = rtspChannel{sid, false}
						s.channels[rtcpChannel] = rtspChannel{sid, true}
					}
					out["RTSP SETUP Request Event IDs"] = append([]uint64(nil), req.eventIDs...)
					out["RTSP SETUP Request Packet References"] = append([]PacketReference(nil), req.packetRefs...)
					transportStatus, transport := resolveRTSPUDPTransport(req.udpOffer, h.Values("Transport"))
					out["UDP Media Transport Status"] = transportStatus
					if transport != nil {
						out["UDP Media Transport"] = transport.sessionValue()
					}
					s.sessions[strings.Clone(sid)] = "ready"
				case "PLAY":
					if _, exists := s.sessions[sid]; !exists {
						out["Context Missing"] = "SETUP was not observed"
					}
					s.sessions[strings.Clone(sid)] = "playing"
				case "PAUSE":
					s.sessions[strings.Clone(sid)] = "ready"
				case "TEARDOWN":
					out["RTSP Session Teardown"] = true
					delete(s.sessions, sid)
					for ch, v := range s.channels {
						if v.session == sid {
							delete(s.channels, ch)
						}
					}
				}
			}
			if status >= 200 {
				delete(s.pending, key)
				if req != nil && req.evidenceCost > 0 {
					s.requestEvidenceBytes -= req.evidenceCost
				}
			}
		}
	}
	if strings.EqualFold(h.Get("Content-Type"), "application/sdp") && n > 0 {
		lines := bytes.Split(w[hdr:], []byte("\n"))
		if len(lines) > max {
			return nil, protocolError(ErrResourceExceeded, "RTSP SDP lines")
		}
		values := []any{}
		for _, line := range lines {
			line = bytes.TrimSuffix(line, []byte("\r"))
			if len(line) == 0 {
				continue
			}
			if len(line) < 2 || line[1] != '=' {
				return nil, fmt.Errorf("rtsp: invalid SDP line")
			}
			values = append(values, map[string]any{"Type": string(line[:1]), "Value": string(line[2:])})
		}
		out["SDP"] = values
	}
	out["Phase"] = s.sessions[sid]
	out["Pending Requests"] = len(s.pending)
	return out, nil
}
