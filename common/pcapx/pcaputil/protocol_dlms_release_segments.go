package pcaputil

import (
	"bytes"
	"encoding/hex"
)

func dlmsLifecycleSegmentPrefix(info []byte) bool {
	if len(info) < 5 || info[0] != 0xe6 || info[2] != 0 {
		return false
	}
	return info[3] == 0x62 && info[1] == 0xe6 || info[3] == 0x63 && info[1] == 0xe7 || dlmsAbortSegmentPrefix(info)
}

func dlmsAbortSegmentPrefix(info []byte) bool {
	return len(info) >= 5 && info[0] == 0xe6 && (info[1] == 0xe6 || info[1] == 0xe7) && info[2] == 0 && info[3] == 0x64
}

// A complete validated first-frame prefix retires eligible state of the same
// logical endpoint. It does not prove that the partial APDU is valid or that a
// peer released an association. Only the fully assembled lifecycle codec may
// publish complete fields and a request link. Each literal frame keeps its CRC,
// header, sequence and raw bytes; no synthetic carrier is passed to the codec.
func (f *binFlow) consumeDLMSLifecycleSegments(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s := f.dlms
	fail := func(kind ProtocolErrorKind, why string, retire bool) (map[string]any, uint64, error) {
		if retire {
			s.invalidate()
		}
		return nil, 0, dlmsError(kind, why)
	}
	t := s.fragments
	if m.kind != "I" {
		if t != nil && m.kind == "S" && dir != t.dir && m.source == t.destination && m.destination == t.source && m.nr == (t.last+1)&7 {
			m.fields["Association"] = "unassociated-link-observation"
			return m.fields, 0, nil
		}
		return fail(ErrContextRequired, "link control interrupts segmented lifecycle", true)
	}
	if t == nil {
		if !m.segmented || !dlmsLifecycleSegmentPrefix(m.information) {
			return fail(ErrUnsupportedFeature, "first lifecycle segment needs complete LLC and BER tag/length prefix", true)
		}
		command := m.information[3]
		if command != 0x64 && s.ambiguous {
			return fail(ErrContextRequired, "release segments require trustworthy conversation", false)
		}
		if command == 0x62 {
			if s.clientKnown && dir != s.clientDir {
				return fail(ErrContextRequired, "release fragment requester direction differs", false)
			}
			if p := s.pending; p != nil && (p.source != m.source || p.destination != m.destination) {
				return fail(ErrContextRequired, "release fragment differs from pending endpoint", false)
			}
			if s.seqKnown[dir] && m.ns != s.next[dir] || s.seqKnown[1-dir] && m.nr != s.next[1-dir] {
				return fail(ErrContextRequired, "release first fragment sequence differs", false)
			}
		} else if command == 0x63 {
			p := s.pending
			if p == nil {
				key := dlmsACSEKey{1 - dir, m.destination, m.source}
				if err := s.retireHDLCEndpoint(key.dir, key.source, key.destination, f.a.budget.MaxCollectionElements); err != nil {
					return nil, 0, err
				}
				h := s.acseSeen[key]
				h.used |= 1 << ((m.nr - 1) & 7)
				s.acseSeen[key] = h
				return fail(ErrContextRequired, "orphan release fragments retire acknowledged identity", false)
			}
			if p.kind != "ACSE" || p.acseCommand != 0x62 || p.dir == dir || p.source != m.destination || p.destination != m.source || m.nr != (p.ns+1)&7 || s.seqKnown[dir] && m.ns != s.next[dir] {
				return fail(ErrContextRequired, "release response fragments differ from observed request/ack", false)
			}
		} else if p := s.pending; p != nil && !(p.source == m.source && p.destination == m.destination || p.source == m.destination && p.destination == m.source) {
			return fail(ErrContextRequired, "abort fragment differs from pending endpoint", false)
		}
		if command != 0x63 {
			if err := s.retireHDLCEndpoint(dir, m.source, m.destination, f.a.budget.MaxCollectionElements); err != nil {
				return nil, 0, err
			}
		}
		t = &dlmsFragments{acseCommand: command, request: command == 0x62, dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
	} else {
		if dir != t.dir || m.source != t.source || m.destination != t.destination || m.nr != t.nr {
			return fail(ErrContextRequired, "lifecycle continuation direction/address/ack differs", false)
		}
		if bytes.Equal(t.wire, wire) {
			m.fields["Frame Kind"] = "Segmented ACSE"
			m.fields["ACSE Segmentation"] = dlmsSegmentFields(m, t.count, false)
			m.fields["Association"] = "retransmitted-acse-segment"
			return m.fields, 0, nil
		}
		if m.ns != (t.last+1)&7 || t.sequenceSet&(1<<m.ns) != 0 {
			return fail(ErrContextRequired, "lifecycle continuation sequence missing/conflicting/reused", true)
		}
	}
	if t.request && s.acseSeen[dlmsACSEKey{dir, m.source, m.destination}].used&(1<<m.ns) != 0 {
		return fail(ErrContextRequired, "release fragment reuses retired address/sequence", true)
	}
	if len(m.information) == 0 {
		return fail(ErrMalformedMessage, "empty lifecycle continuation", true)
	}
	if t.count >= min(8, f.a.budget.MaxCollectionElements) || len(m.information) > min(4099, f.a.budget.MaxCollectionElements)-len(t.info) {
		return fail(ErrResourceExceeded, "lifecycle information byte/frame budget", true)
	}
	joined := make([]byte, len(t.info)+len(m.information))
	copy(joined, t.info)
	copy(joined[len(t.info):], m.information)
	count, mask := t.count+1, t.sequenceSet|1<<m.ns
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
		return fail(ErrMalformedMessage, "assembled lifecycle command differs", true)
	}
	s.fragments = nil
	fields, reply, err := s.consumeLifecycle(decoded, wire, dir, id, f.a.budget.MaxCollectionElements)
	if err != nil {
		return failDLMSRequestAssembly(s, err)
	}
	if t.request {
		key := dlmsACSEKey{dir, m.source, m.destination}
		h := s.acseSeen[key]
		h.used |= mask
		s.acseSeen[key] = h
		if s.pending != nil {
			s.pending.ns, s.pending.requestInformation, s.pending.requestFrames = last, bytes.Clone(joined), count
		}
	}
	if t.acseCommand != 0x64 {
		s.next[dir], s.seqKnown[dir] = (last+1)&7, true
	}
	meta["assembled_information_hex"] = hex.EncodeToString(joined)
	fields["ACSE Segmentation"] = meta
	retained := s.storage() + dlmsProjection(wire)
	if retained < f.sessionBytes {
		f.a.buffered.Add(retained - f.sessionBytes)
		f.sessionBytes = retained
	}
	return fields, reply, nil
}
