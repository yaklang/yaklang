package pcaputil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// This TCP profile deliberately follows the fixed GPA/GSF BinaryImageFrames
// producer layout, not an unbounded 4GB collector. Every publisher direction
// owns its bytes. ID/SOC/FRACSEC equality, consecutive indices and a 30-second
// capture-time lifetime are explicit conservative association policies.
type c37118Continuation struct {
	image           []byte
	count           int
	next            uint16
	started, latest time.Time
}

// Every TCP message from this publisher advances its capture watermark, even
// DATA without usable configuration. An unrelated publisher cannot advance it;
// backdated arrivals cannot revive an expired series or start a stale one.
func (f *binFlow) advanceC37118ContinuationClock(dir int) {
	if s := f.c37118; s != nil {
		if ts := f.directions[dir].ts; ts.After(s.clock[dir]) {
			s.clock[dir] = ts
		}
		if p := s.continuation[dir]; p != nil && s.clock[dir].Sub(p.started) > 30*time.Second {
			s.continuation[dir] = nil
		}
	}
}

func (f *binFlow) consumeC37118Continuation(dir int, raw []byte) (map[string]any, error, bool) {
	s := f.c37118
	// The producer can split the two-byte cumulative CRC across frames: a
	// 17-byte final physical frame carries just its second checksum byte.
	if s == nil || len(raw) < 17 || raw[0] != 0xaa || raw[1] != 0x52 {
		return nil, nil, false
	}
	index := binary.BigEndian.Uint16(raw[14:])
	if index == 0 || index == 1 && len(raw) != 65535 {
		return nil, nil, false
	}
	// Never borrow a previous configuration while its replacement is incomplete.
	s.configuration[dir] = nil
	p := s.continuation[dir]
	reject := func(kind ProtocolErrorKind, why string) (map[string]any, error, bool) {
		s.continuation[dir] = nil
		return nil, protocolError(kind, why), true
	}
	if int(binary.BigEndian.Uint16(raw[2:])) != len(raw) {
		return reject(ErrMalformedMessage, "C37.118 continuation physical boundary mismatch")
	}
	ts := f.directions[dir].ts
	if index == 1 {
		// A new first frame explicitly replaces the previous incomplete series.
		p = &c37118Continuation{next: 2, started: ts, latest: s.clock[dir]}
		s.continuation[dir] = nil
		if p.latest.Sub(p.started) > 30*time.Second {
			return reject(ErrContextRequired, "C37.118 first continuation predates publisher capture watermark")
		}
	} else {
		if p == nil {
			return reject(ErrContextRequired, "C37.118 continuation without this publisher's first fragment")
		}
		if ts.After(p.latest) {
			p.latest = ts
		}
		if p.latest.Sub(p.started) > 30*time.Second {
			return reject(ErrContextRequired, "C37.118 continuation capture-time lifetime expired")
		}
		if !bytes.Equal(raw[4:14], p.image[4:14]) {
			return reject(ErrContextRequired, "C37.118 continuation publisher ID or timestamp differs")
		}
		if index != 65535 && (index != p.next || len(raw) != 65535) {
			return reject(ErrContextRequired, "C37.118 continuation index or intermediate size differs")
		}
	}
	add := raw
	if index != 1 {
		add = raw[16:]
	}
	limit := min(f.a.config.MaxMessageBytes, f.a.budget.MaxMessageBytes)
	if len(add) > limit-len(p.image) || p.count >= f.a.budget.MaxCollectionElements {
		return reject(ErrResourceExceeded, "C37.118 continuation logical image or fragment budget exceeded")
	}
	n := len(p.image) + len(add)
	other := len(s.configuration[1-dir])
	if q := s.continuation[1-dir]; q != nil {
		other += len(q.image)
	}
	if err := f.reserveSession(512 + int64(other)*32 + int64(n)*32); err != nil {
		s.continuation[dir] = nil
		return nil, err, true
	}
	// Exact-sized allocation keeps the retained capacity inside the checked
	// byte budget; append growth cannot hide extra capacity beyond n.
	image := make([]byte, n)
	copy(image, p.image)
	copy(image[len(p.image):], add)
	p.image, p.count = image, p.count+1
	if index != 65535 {
		if index != 1 {
			p.next++
		}
		s.continuation[dir] = p
		out := c37118ContinuationFields(raw, p, false)
		return out, protocolError(ErrContextRequired, "C37.118 CFG-3 awaiting final fragment and cumulative CRC"), true
	}
	s.continuation[dir] = nil
	m, err := stream_parser.DecodeC37118ConfigurationImage(image, f.a.budget.MaxCollectionElements, limit)
	if err != nil {
		if errors.Is(err, stream_parser.ErrC37118CollectionLimit) {
			return reject(ErrResourceExceeded, err.Error())
		}
		return reject(ErrMalformedMessage, err.Error())
	}
	// Report the actual final physical frame, never a fabricated FRAMESIZE for
	// the logical image. Extended fields originate only from validated image.
	m.FrameSize = uint16(len(raw))
	out := c37118Fields(m)
	for k, v := range c37118ContinuationFields(raw, p, true) {
		out[k] = v
	}
	s.configuration[dir] = image
	return out, nil, true
}

func c37118ContinuationFields(raw []byte, p *c37118Continuation, validated bool) map[string]any {
	return map[string]any{"Sync": 170, "Sync Reserved": 0, "Packet Name": "CFG-3", "Frame Type": 5,
		"Version": 2, "ID Code": int(binary.BigEndian.Uint16(raw[4:])), "Frame Size": len(raw),
		"Second Of Century": binary.BigEndian.Uint32(raw[6:]), "Time Quality": raw[10],
		"Fraction Of Second": binary.BigEndian.Uint32(raw[10:]) & 0xffffff, "Body Decoded": validated,
		"Role": "configuration", "Continuation Index": int(binary.BigEndian.Uint16(raw[14:])),
		"Configuration Fragments": p.count, "Configuration Image Bytes": len(p.image), "Cumulative CRC Validated": validated}
}
