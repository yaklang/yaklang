package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"sort"
)

// WithDNP3TransportFragments opts into bounded TCP transport reassembly. Each
// complete link frame remains a separate event; only FIN publishes the assembled
// APDU. Application fragmentation and authentication remain separate capabilities.
func WithDNP3TransportFragments(enabled bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.DecodeDNP3TransportFragments = enabled
		return nil
	}
}

// WithSessionDNP3TransportFragments selects bounded assembly on a TCP session.
func WithSessionDNP3TransportFragments(enabled bool) ProtocolSessionOption {
	return func(s *captureSession) error { s.f.a.config.DecodeDNP3TransportFragments = enabled; return nil }
}

type dnp3TransportKey struct {
	dir                 int
	source, destination uint16
	master              bool
}
type dnp3TransportFragment struct {
	data     []byte
	last     byte
	digest   [32]byte
	segments int
}

func (s *binDNP3) memoryTarget(extra int64) int64 {
	n := int64(256 + len(s.pending)*80)
	for _, r := range s.pending {
		n += int64(cap(r.selectors)) * 64
	}
	for _, f := range s.fragments {
		n += 128 + int64(len(f.data))
	}
	return n + extra
}

// Sequence numbers advance modulo 64. A gap is reported without appending bytes;
// the expected segment can still arrive. Exact duplicate last segments are
// observations only. A conflicting duplicate invalidates this link pair's
// application context, so it cannot manufacture a later solicited response.
func (s *binDNP3) assembleTransport(dir int, src, dst uint16, control byte, user []byte, out map[string]any) ([]byte, bool, error) {
	th := user[0]
	seq, fir, fin := th&63, th&64 != 0, th&128 != 0
	out["Transport Sequence"], out["Transport FIR"], out["Transport FIN"] = int(seq), fir, fin
	out["Packet Name"] = "Transport Segment"
	fail := func(kind ProtocolErrorKind, why string) ([]byte, bool, error) {
		out["Error Scope"] = "application"
		return nil, false, protocolError(kind, why)
	}
	if len(user) == 1 {
		return fail(ErrMalformedMessage, "DNP3 empty transport payload")
	}
	key := dnp3TransportKey{dir, src, dst, control&128 != 0}
	fragment := s.fragments[key]
	if fir && fin {
		delete(s.fragments, key)
		out["Transport State"], out["Transport Segments"], out["Transport Bytes"] = "complete", 1, len(user)-1
		out["Application Data"] = bytes.Clone(user[1:])
		return append([]byte{0xc0}, user[1:]...), true, nil
	}
	if fir {
		state := "buffering"
		if fragment != nil {
			state = "restarted"
		}
		if fragment == nil && len(s.fragments) >= sessionCollectionLimit(s.maxPending) {
			return nil, false, protocolError(ErrResourceExceeded, "DNP3 transport assembly count")
		}
		// Allocate exactly retained bytes; slice capacity is not an uncharged reserve.
		delta := int64(128 + len(user) - 1)
		if fragment != nil {
			delta -= 128 + int64(len(fragment.data))
		}
		if len(user)-1 > s.maxApplication {
			return nil, false, protocolError(ErrResourceExceeded, "DNP3 assembled application length")
		}
		if s.reserve != nil {
			if err := s.reserve(s.memoryTarget(delta)); err != nil {
				return nil, false, err
			}
		}
		fragment = &dnp3TransportFragment{data: bytes.Clone(user[1:]), last: seq, digest: sha256.Sum256(user), segments: 1}
		if s.fragments == nil {
			s.fragments = map[dnp3TransportKey]*dnp3TransportFragment{}
		}
		s.fragments[key] = fragment
		out["Transport State"], out["Transport Segments"], out["Transport Bytes"] = state, fragment.segments, len(fragment.data)
		return nil, false, nil
	}
	if fragment == nil {
		return fail(ErrContextRequired, "DNP3 continuation has no observed transport start")
	}
	out["Transport Segments"], out["Transport Bytes"] = fragment.segments, len(fragment.data)
	if seq == fragment.last {
		if sha256.Sum256(user) == fragment.digest {
			out["Transport State"] = "duplicate"
			return nil, false, nil
		}
		delete(s.fragments, key)
		s.discardLink(dir, src, dst, control&0x80 != 0)
		return fail(ErrContextRequired, "DNP3 conflicting duplicate transport segment")
	}
	if seq != (fragment.last+1)&63 {
		return fail(ErrContextRequired, "DNP3 unexpected transport sequence")
	}
	if fragment.segments >= sessionCollectionLimit(s.maxPending) {
		return nil, false, protocolError(ErrResourceExceeded, "DNP3 transport segment count")
	}
	if len(user)-1 > s.maxApplication-len(fragment.data) {
		return nil, false, protocolError(ErrResourceExceeded, "DNP3 assembled application length")
	}
	if s.reserve != nil {
		if err := s.reserve(s.memoryTarget(int64(len(user) - 1))); err != nil {
			return nil, false, err
		}
	}
	data := make([]byte, len(fragment.data)+len(user)-1)
	copy(data, fragment.data)
	copy(data[len(fragment.data):], user[1:])
	fragment.data, fragment.last, fragment.digest = data, seq, sha256.Sum256(user)
	fragment.segments++
	out["Transport State"], out["Transport Segments"], out["Transport Bytes"] = "buffering", fragment.segments, len(data)
	if !fin {
		return nil, false, nil
	}
	delete(s.fragments, key)
	out["Transport State"] = "complete"
	out["Application Data"] = bytes.Clone(data)
	return append([]byte{0xc0}, data...), true, nil
}

func (f *binFlow) finishDNP3Transport(reason TrafficFlowCloseReason) {
	s := f.dnp3
	if s == nil || !s.transportFragments || len(s.fragments) == 0 {
		return
	}
	keys := make([]dnp3TransportKey, 0, len(s.fragments))
	for k := range s.fragments {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.dir != b.dir {
			return a.dir < b.dir
		}
		if a.source != b.source {
			return a.source < b.source
		}
		if a.destination != b.destination {
			return a.destination < b.destination
		}
		return !a.master && b.master
	})
	for _, k := range keys {
		x := s.fragments[k]
		e := f.event(k.dir, nil, "incomplete", string(reason)+": DNP3 transport ended before FIN")
		e.Completeness = "incomplete"
		e.sessionError = &ProtocolError{Kind: ErrNeedMore, Message: "DNP3 transport ended before FIN"}
		e.Error = e.sessionError.Message
		e.Session = map[string]any{"Packet Name": "Transport Segment", "Source": int(k.source), "Destination": int(k.destination), "Transport State": "incomplete", "Transport Segments": x.segments, "Transport Bytes": len(x.data), "Transport Sequence": int(x.last)}
		e.semanticSession = cloneSession(e.Session)
		f.a.incomplete.Add(1)
		f.a.emit(e)
	}
}
