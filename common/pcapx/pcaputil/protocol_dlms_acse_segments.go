package pcaputil

import (
	"bytes"
	"encoding/hex"
)

// Prefix evidence selects bounded assembly, never validates a partial APDU.
// The complete BER/ACSE decoder still decides acceptance after the final frame.
func dlmsACSESegmentPrefix(info []byte) bool {
	if len(info) < 5 || info[0] != 0xe6 || info[2] != 0 || !dlmsACSEAPDU(info[3:]) {
		return false
	}
	tag := info[3]
	return tag == 0x60 && info[1] == 0xe6 || tag == 0x61 && info[1] == 0xe7 || dlmsLifecycleSegmentPrefix(info)
}

// Only complete selected ACSE information installs or consumes request identity.
// Literal final frame header/CRC/raw bytes remain captured, never synthesized.
func (f *binFlow) consumeDLMSACSESegments(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s := f.dlms
	if s.fragments != nil && s.fragments.acseCommand >= 0x62 || s.fragments == nil && dlmsLifecycleSegmentPrefix(m.information) {
		return f.consumeDLMSLifecycleSegments(dir, wire, id, m)
	}
	fail := func(kind ProtocolErrorKind, why string, retire bool) (map[string]any, uint64, error) {
		if retire {
			s.invalidate()
		}
		return nil, 0, dlmsError(kind, why)
	}
	if s.ambiguous {
		return fail(ErrContextRequired, "ACSE segments require trustworthy conversation", false)
	}
	t := s.fragments
	if m.kind != "I" {
		if t != nil && m.kind == "S" && dir != t.dir && m.source == t.destination && m.destination == t.source {
			if m.nr != (t.last+1)&7 {
				return fail(ErrContextRequired, "ACSE supervisory acknowledgement differs from observed fragments", false)
			}
			m.fields["Association"] = "unassociated-link-observation"
			return m.fields, 0, nil
		}
		return fail(ErrContextRequired, "link control interrupts segmented ACSE", true)
	}
	if t == nil {
		if !m.segmented || !dlmsACSESegmentPrefix(m.information) {
			return fail(ErrUnsupportedFeature, "first ACSE segment needs complete LLC and BER tag/length prefix", true)
		}
		command := m.information[3]
		request := command == 0x60
		if s.transfer != nil {
			return fail(ErrContextRequired, "ACSE fragments overlap active Get transfer", true)
		}
		if request {
			if s.pending != nil {
				return fail(ErrContextRequired, "ACSE fragments overlap pending request", true)
			}
			if s.clientKnown && dir != s.clientDir {
				return fail(ErrContextRequired, "ACSE fragment requester direction differs", false)
			}
		} else {
			p := s.pending
			if p == nil {
				key := dlmsACSEKey{1 - dir, m.destination, m.source}
				if err := s.retireHDLCEndpoint(key.dir, key.source, key.destination, f.a.budget.MaxCollectionElements); err != nil {
					return nil, 0, err
				}
				h := s.acseSeen[key]
				h.used |= 1 << ((m.nr - 1) & 7)
				s.acseSeen[key] = h
				s.invalidate()
				return fail(ErrContextRequired, "orphan ACSE response fragments retire incomplete conversation", false)
			}
			if p.kind != "ACSE" || p.acseCommand != command-1 || dir == p.dir || m.source != p.destination || m.destination != p.source || m.nr != (p.ns+1)&7 {
				return fail(ErrContextRequired, "ACSE response fragments lack reversed observed request/ack", false)
			}
		}
		if s.seqKnown[dir] && m.ns != s.next[dir] || s.seqKnown[1-dir] && m.nr != s.next[1-dir] {
			return fail(ErrContextRequired, "ACSE first fragment sequence differs", false)
		}
		t = &dlmsFragments{acseCommand: command, request: request, dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
	} else {
		if dir != t.dir || m.source != t.source || m.destination != t.destination || m.nr != t.nr {
			return fail(ErrContextRequired, "ACSE continuation direction/address/ack differs", false)
		}
		if bytes.Equal(t.wire, wire) {
			m.fields["Frame Kind"] = "Segmented ACSE"
			m.fields["ACSE Segmentation"] = dlmsSegmentFields(m, t.count, false)
			m.fields["Association"] = "retransmitted-acse-segment"
			return m.fields, 0, nil
		}
		if m.ns != (t.last+1)&7 || t.sequenceSet&(1<<m.ns) != 0 {
			return fail(ErrContextRequired, "ACSE continuation sequence is missing/conflicting/reused", true)
		}
	}
	if t.request && s.acseSeen[dlmsACSEKey{dir, m.source, m.destination}].used&(1<<m.ns) != 0 {
		return fail(ErrContextRequired, "ACSE fragment reuses retired address/sequence identity", true)
	}
	if len(m.information) == 0 {
		return fail(ErrMalformedMessage, "empty ACSE continuation", true)
	}
	if t.count >= min(8, f.a.budget.MaxCollectionElements) || len(m.information) > min(4099, f.a.budget.MaxCollectionElements)-len(t.info) {
		return fail(ErrResourceExceeded, "ACSE information byte/frame budget", true)
	}
	joined := make([]byte, len(t.info)+len(m.information))
	copy(joined, t.info)
	copy(joined[len(t.info):], m.information)
	count := t.count + 1
	mask := t.sequenceSet | 1<<m.ns
	meta := dlmsSegmentFields(m, count, !m.segmented)
	if m.segmented {
		t.info, t.wire, t.last, t.count, t.sequenceSet = joined, bytes.Clone(wire), m.ns, count, mask
		s.fragments = t
		m.fields["Frame Kind"] = "Segmented ACSE"
		m.fields["ACSE Segmentation"] = meta
		m.fields["Association"] = "observed-acse-segment"
		return m.fields, 0, nil
	}
	last := m.ns
	m.ns = t.first
	decoded, err := decodeDLMSInformation(m, joined, f.a.budget.MaxCollectionElements, f.a.budget.MaxRecursionDepth)
	if err != nil {
		return failDLMSRequestAssembly(s, err)
	}
	if decoded.acse == nil || joined[3] != t.acseCommand {
		return fail(ErrMalformedMessage, "assembled ACSE command differs", true)
	}
	s.fragments = nil
	fields, reply, err := s.consumeACSE(decoded, wire, dir, id, f.a.budget.MaxCollectionElements)
	if err != nil {
		return failDLMSRequestAssembly(s, err)
	}
	if t.request {
		key := dlmsACSEKey{dir, m.source, m.destination}
		h := s.acseSeen[key]
		h.used |= mask
		s.acseSeen[key] = h
		if s.pending != nil {
			s.pending.ns = last
			s.pending.requestInformation = bytes.Clone(joined)
			s.pending.requestFrames = count
		}
	}
	s.next[dir], s.seqKnown[dir] = (last+1)&7, true
	meta["assembled_information_hex"] = hex.EncodeToString(joined)
	fields["ACSE Segmentation"] = meta
	retained := s.storage() + 32768 + 512*int64(len(joined)+len(wire))
	if retained < f.sessionBytes {
		f.a.buffered.Add(retained - f.sessionBytes)
		f.sessionBytes = retained
	}
	return fields, reply, nil
}
