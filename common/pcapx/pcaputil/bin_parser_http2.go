package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type binH2Stream struct {
	headers, ended [2]bool
	window         [2]int64
	method         string
	grpc           [2][]byte
	grpcEnabled    [2]bool
	doh            [2]bool
	dohBuf         [2][]byte
}

type binH2Settings struct {
	table, frame uint32
	window       int64
	push         bool
}
type binHTTP2 struct {
	grpcBuffered        int64
	scanEnd, scanFrames [2]int
	client              int
	initial             [2]bool
	decoder             [2]*stream_parser.HTTP2HeaderDecoder
	settings            [2]binH2Settings // advertised receiving constraints
	acked               [2]binH2Settings
	pending             [2][]binH2Settings
	streams             map[uint32]*binH2Stream
	last                [2]uint32
	goaway              [2]bool
	lastAllowed         [2]uint32
	window              [2]int64
}

func newBinHTTP2(client int) *binHTTP2 {
	h := &binHTTP2{client: client, streams: make(map[uint32]*binH2Stream), window: [2]int64{65535, 65535}}
	for i := 0; i < 2; i++ {
		h.decoder[i] = stream_parser.NewHTTP2HeaderDecoder()
		h.settings[i] = binH2Settings{4096, 16384, 65535, true}
		h.acked[i] = h.settings[i]
	}
	return h
}

func h2FrameLength(w []byte) int { return 9 + (int(w[0]) << 16) + (int(w[1]) << 8) + int(w[2]) }

func (f *binFlow) frameHTTP2(dir int, w []byte) (int, *binSpec, error) {
	h := f.h2
	if h == nil {
		return 0, nil, sessionContext("HTTP/2 client preface was not observed")
	}
	// HPACK's RFC table size excludes the decoder's slices and lookup maps.
	// Account for that Go storage too, including spare slice/map capacity.
	if err := f.reserveSession(h.sessionStorageBytes()); err != nil {
		return 0, nil, err
	}
	at := 0
	entry := "HTTP2FrameSequenceFields"
	if !h.initial[dir] && dir == h.client {
		if len(w) < 24 {
			return 0, nil, nil
		}
		if string(w[:24]) != binH2Preface {
			return 0, nil, fmt.Errorf("http2: invalid client preface")
		}
		at = 24
		entry = "HTTP2InitialClientStream"
	}
	if len(w)-at < 9 {
		return 0, nil, nil
	}
	n := h2FrameLength(w[at:])
	if n > f.a.budget.MaxFrameBytes {
		return 0, nil, protocolError(ErrResourceExceeded, "HTTP/2 frame exceeds local frame budget")
	}
	if n-9 > int(h.effectiveSettings(1-dir).frame) {
		return 0, nil, fmt.Errorf("http2: frame exceeds advertised receive limit")
	}
	if !h.initial[dir] && (w[at+3] != 4 || w[at+4]&1 != 0) {
		return 0, nil, fmt.Errorf("http2: initial direction requires SETTINGS")
	}
	if !h.initial[dir] && dir != h.client {
		entry = "HTTP2InitialServerStream"
	}
	end := at + n
	if end > min(f.a.config.MaxMessageBytes, 1<<20) || end > len(w) {
		if end > 1<<20 {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		return end, f.spec("http2_fields", entry), nil
	}
	// A complete field block is one event. This keeps fragmented HPACK bytes
	// in the existing bounded/accounted stream buffer, never a second queue.
	typ, flags := w[at+3], w[at+4]
	if typ == 1 || typ == 5 {
		stream := binary.BigEndian.Uint32(w[at+5:]) & 0x7fffffff
		count := 1
		if h.scanEnd[dir] > 0 {
			end = h.scanEnd[dir]
			count = h.scanFrames[dir]
		}
		for ; flags&4 == 0; count++ {
			if count >= 1024 {
				return 0, nil, sessionContext("HTTP/2 continuation limit")
			}
			if len(w)-end < 9 {
				return 0, nil, nil
			}
			if w[end+3] != 9 || binary.BigEndian.Uint32(w[end+5:])&0x7fffffff != stream {
				return 0, nil, fmt.Errorf("http2: expected same-stream CONTINUATION")
			}
			flags = w[end+4]
			n = h2FrameLength(w[end:])
			if n > f.a.budget.MaxFrameBytes {
				return 0, nil, protocolError(ErrResourceExceeded, "HTTP/2 continuation exceeds local frame budget")
			}
			if n-9 > int(h.effectiveSettings(1-dir).frame) {
				return 0, nil, fmt.Errorf("http2: continuation exceeds frame limit")
			}
			end += n
			if end > min(f.a.config.MaxMessageBytes, 1<<20) || end > len(w) {
				if end > 1<<20 {
					return f.a.config.MaxMessageBytes + 1, nil, nil
				}
				return end, f.spec("http2_fields", entry), nil
			}
			if flags&4 == 0 {
				h.scanEnd[dir], h.scanFrames[dir] = end, count+1
			}
		}
	}
	if end > 1<<20 {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	return end, f.spec("http2_fields", entry), nil
}

func (h *binHTTP2) consume(dir int, wire []byte) (map[string]any, error) {
	h.scanEnd[dir], h.scanFrames[dir] = 0, 0
	if !h.initial[dir] && dir == h.client {
		wire = wire[24:]
	}
	f, err := stream_parser.InspectHTTP2Frame(wire[:h2FrameLength(wire)])
	if err != nil {
		return nil, err
	}
	info := map[string]any{"Client Direction": h.client, "Stream ID": f.Stream, "Frame Type": f.Type, "Payload Decrypted": false, "HTTP Semantics Validated": false}
	if !h.initial[dir] {
		h.initial[dir] = true
	}
	if f.Type == 4 {
		if f.Flags&1 != 0 {
			peer := 1 - dir
			if len(h.pending[peer]) == 0 {
				return nil, fmt.Errorf("http2: SETTINGS ACK without observed SETTINGS")
			}
			before := h.effectiveSettings(peer)
			applied := h.pending[peer][0]
			h.pending[peer] = h.pending[peer][1:]
			h.acked[peer] = applied
			if err := h.applyLimits(peer, before); err != nil {
				return nil, err
			}
			h.decoder[dir].RequireTableReduction(applied.table)
			info["Settings Acknowledged"] = true
			return info, nil
		}
		if len(h.pending[dir]) >= 128 {
			return nil, sessionContext("HTTP/2 unacknowledged SETTINGS limit")
		}
		s := h.settings[dir]
		for at := 9; at < len(wire); at += 6 {
			id, v := binary.BigEndian.Uint16(wire[at:]), binary.BigEndian.Uint32(wire[at+2:])
			switch id {
			case 1:
				if v > 65536 {
					return nil, sessionContext("HTTP/2 HPACK table above 64 KiB local limit")
				}
				s.table = v
			case 2:
				if dir != h.client {
					return nil, fmt.Errorf("http2: server sent ENABLE_PUSH")
				}
				s.push = v != 0
			case 4:
				s.window = int64(v)
			case 5:
				s.frame = v
			}
		}
		before := h.effectiveSettings(dir)
		h.settings[dir] = s
		h.pending[dir] = append(h.pending[dir], s)
		if err := h.applyLimits(dir, before); err != nil {
			return nil, err
		}
		info["Header Table Size"], info["Maximum Frame Size"] = s.table, s.frame
		return info, nil
	}
	if !h.initial[1-dir] && dir != h.client {
		return nil, sessionContext("HTTP/2 missing client SETTINGS")
	}
	stream := h.streams[f.Stream]
	if f.Type == 7 {
		h.goaway[dir] = true
		h.lastAllowed[dir] = binary.BigEndian.Uint32(wire[9:]) & 0x7fffffff
		info["Last Stream ID"] = h.lastAllowed[dir]
		return info, nil
	}
	if f.Type == 8 {
		n := int64(binary.BigEndian.Uint32(wire[9:]) & 0x7fffffff)
		if f.Stream == 0 {
			h.window[1-dir] += n
			if h.window[1-dir] > 0x7fffffff {
				return nil, fmt.Errorf("http2: connection window overflow")
			}
		} else if stream != nil {
			stream.window[1-dir] += n
			if stream.window[1-dir] > 0x7fffffff {
				return nil, fmt.Errorf("http2: stream window overflow")
			}
		}
		return info, nil
	}
	if f.Type == 9 {
		return nil, fmt.Errorf("http2: orphan CONTINUATION")
	}
	if f.Type == 5 {
		return nil, sessionContext("HTTP/2 PUSH_PROMISE requires a push profile")
	}
	if f.Type == 1 {
		if stream == nil {
			if dir != h.client {
				return nil, sessionContext("HTTP/2 response has no observed request")
			}
			if f.Stream%2 != 1 || f.Stream <= h.last[dir] {
				return nil, fmt.Errorf("http2: reused or invalid client stream ID")
			}
			if h.goaway[1-dir] {
				return nil, sessionContext("HTTP/2 new stream after GOAWAY")
			}
			if len(h.streams) >= 1024 {
				return nil, sessionContext("HTTP/2 active stream limit")
			}
			h.last[dir] = f.Stream
			stream = &binH2Stream{window: [2]int64{h.effectiveSettings(1).window, h.effectiveSettings(0).window}}
			h.streams[f.Stream] = stream
		}
		if stream.ended[dir] {
			return nil, fmt.Errorf("http2: HEADERS on closed stream direction")
		}
		block := bytes.Clone(f.Fragment)
		for at := h2FrameLength(wire); at < len(wire); {
			n := h2FrameLength(wire[at:])
			next, e := stream_parser.InspectHTTP2Frame(wire[at : at+n])
			if e != nil {
				return nil, e
			}
			block = append(block, next.Fragment...)
			at += n
		}
		headers, e := h.decoder[dir].Decode(block)
		if e != nil {
			return nil, fmt.Errorf("http2: HPACK: %w", e)
		}
		pseudo := map[string]string{}
		regular := false
		for _, header := range headers {
			name, value := header["Name"].(string), header["Value"].(string)
			if name == "" || strings.ToLower(name) != name || strings.ContainsAny(name, " \t\r\n") || strings.ContainsAny(value, "\r\n\x00") {
				return nil, fmt.Errorf("http2: invalid header name/value")
			}
			if strings.HasPrefix(name, ":") {
				if regular || stream.headers[dir] {
					return nil, fmt.Errorf("http2: misplaced pseudo-header")
				}
				if _, ok := pseudo[name]; ok {
					return nil, fmt.Errorf("http2: duplicate pseudo-header")
				}
				pseudo[name] = value
			} else {
				regular = true
				if name == "connection" || name == "upgrade" || name == "keep-alive" || name == "proxy-connection" || name == "transfer-encoding" || name == "te" && value != "trailers" {
					return nil, fmt.Errorf("http2: forbidden connection header")
				}
			}
		}
		kind := "trailers"
		if !stream.headers[dir] {
			if dir == h.client {
				kind = "request"
				method := pseudo[":method"]
				if len(method) > 256 {
					return nil, sessionContext("HTTP/2 method exceeds local state limit")
				}
				if method == "" || method != "CONNECT" && (pseudo[":path"] == "" || pseudo[":scheme"] == "") || method == "CONNECT" && (pseudo[":authority"] == "" || pseudo[":path"] != "" || pseudo[":scheme"] != "") {
					return nil, fmt.Errorf("http2: missing/invalid request pseudo-headers")
				}
				for k := range pseudo {
					if k != ":method" && k != ":path" && k != ":scheme" && k != ":authority" {
						return nil, fmt.Errorf("http2: invalid request pseudo-header")
					}
				}
				stream.method = method
				stream.headers[dir] = true
			} else {
				kind = "response"
				status := pseudo[":status"]
				if len(pseudo) != 1 || len(status) != 3 || status[0] < '1' || status[0] > '5' || status[1] < '0' || status[1] > '9' || status[2] < '0' || status[2] > '9' || status == "101" {
					return nil, fmt.Errorf("http2: invalid response status")
				}
				if status[0] == '1' {
					if f.Flags&1 != 0 {
						return nil, fmt.Errorf("http2: informational response ends stream")
					}
					kind = "informational"
				} else {
					stream.headers[dir] = true
				}
			}
		} else if f.Flags&1 == 0 {
			return nil, fmt.Errorf("http2: trailers must end stream")
		}
		stream.ended[dir] = f.Flags&1 != 0
		info["Headers"], info["Header Kind"], info["Decoded Strings Have Direct Wire Spans"] = headers, kind, false
	}
	if f.Type == 0 {
		if stream == nil {
			return nil, sessionContext("HTTP/2 DATA has no observed stream")
		}
		if !stream.headers[dir] || stream.ended[dir] {
			return nil, fmt.Errorf("http2: DATA outside open message")
		}
		n := int64(len(wire) - 9)
		h.window[dir] -= n
		stream.window[dir] -= n
		if h.window[dir] < 0 || stream.window[dir] < 0 {
			return nil, fmt.Errorf("http2: flow-control window exceeded")
		}
		stream.ended[dir] = f.Flags&1 != 0
	}
	if f.Type == 3 {
		if stream == nil {
			return nil, sessionContext("HTTP/2 reset without observed stream")
		}
		delete(h.streams, f.Stream)
		info["Reset"] = true
	}
	if stream != nil {
		info["Request Method"], info["End Stream"], info["Exchange Complete"] = stream.method, stream.ended[dir], stream.ended[0] && stream.ended[1]
		if stream.ended[0] && stream.ended[1] {
			delete(h.streams, f.Stream)
		}
	}
	return info, nil
}

// SETTINGS may cross traffic already in flight. Until the sender acknowledges
// a reduction, accept the largest observed outstanding allowance. ACK advances
// one ordered settings batch; HPACK still records the required minimum update.
func (h *binHTTP2) effectiveSettings(receiver int) binH2Settings {
	s := h.acked[receiver]
	for _, p := range h.pending[receiver] {
		s.table = max(s.table, p.table)
		s.frame = max(s.frame, p.frame)
		s.window = max(s.window, p.window)
	}
	return s
}
func (h *binHTTP2) applyLimits(receiver int, before binH2Settings) error {
	after := h.effectiveSettings(receiver)
	for _, stream := range h.streams {
		stream.window[1-receiver] += after.window - before.window
		if stream.window[1-receiver] > 0x7fffffff {
			return fmt.Errorf("http2: stream window overflow")
		}
	}
	h.decoder[1-receiver].SetAllowedTableSize(after.table)
	return nil
}

func (h *binHTTP2) sessionStorageBytes() int64 {
	tables := int64(h.settings[0].table) + int64(h.settings[1].table)
	return 12288 + int64(len(h.streams)+1)*512 + int64(len(h.pending[0])+len(h.pending[1])+1)*64 + 8*tables + h.grpcBuffered
}
