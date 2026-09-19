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
type rtspRequest struct {
	method, session string
	hash            [32]byte
}
type rtspChannel struct {
	session string
	rtcp    bool
}
type binRTSP struct {
	pending  map[rtspKey]rtspRequest
	sessions map[string]string
	channels map[byte]rtspChannel
	media    *binRTP
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
	mediaBytes := int64(0)
	if s.media != nil {
		mediaBytes = int64(len(s.media.sources)+1) * 512
	}
	if err := f.reserveSession(1024 + int64(len(s.pending)+1)*512 + int64(len(s.sessions)+1)*320 + int64(len(s.channels)+2)*128 + mediaBytes); err != nil {
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
		s.pending = map[rtspKey]rtspRequest{}
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
		if ch.rtcp != rtpIsRTCP(w[4:]) {
			return nil, fmt.Errorf("rtsp: RTP/RTCP channel mismatch")
		}
		media, err := s.media.consume(w[4:], ts, max)
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
		if old, ok := s.pending[key]; ok {
			if old.hash != sha256.Sum256(w) {
				return nil, protocolError(ErrDesynchronized, "RTSP CSeq reused")
			}
			out["Retransmission"] = true
		} else if len(s.pending) >= max {
			return nil, protocolError(ErrResourceExceeded, "RTSP pending requests")
		}
		s.pending[key] = rtspRequest{strings.Clone(parts[0]), strings.Clone(sid), sha256.Sum256(w)}
		out["Packet Name"] = parts[0]
		out["URI"] = parts[1]
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
					for _, part := range strings.Split(h.Get("Transport"), ";") {
						kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
						if len(kv) != 2 || strings.ToLower(kv[0]) != "interleaved" {
							continue
						}
						pair := strings.Split(kv[1], "-")
						if len(pair) != 2 {
							return nil, fmt.Errorf("rtsp: interleaved channel pair")
						}
						a, e1 := strconv.ParseUint(pair[0], 10, 8)
						b, e2 := strconv.ParseUint(pair[1], 10, 8)
						if e1 != nil || e2 != nil || a == b {
							return nil, fmt.Errorf("rtsp: channel range")
						}
						extra := 0
						for _, channel := range []byte{byte(a), byte(b)} {
							if old, exists := s.channels[channel]; exists {
								if old.session != sid {
									return nil, protocolError(ErrDesynchronized, "RTSP channel already assigned")
								}
							} else {
								extra++
							}
						}
						if len(s.channels)+extra > max {
							return nil, protocolError(ErrResourceExceeded, "RTSP channels")
						}
						s.channels[byte(a)] = rtspChannel{sid, false}
						s.channels[byte(b)] = rtspChannel{sid, true}
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
