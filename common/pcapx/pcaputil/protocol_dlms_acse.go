package pcaputil

import "bytes"

type dlmsACSEKey struct {
	dir                 int
	source, destination uint32
}

func dlmsACSEAPDU(p []byte) bool {
	return len(p) > 0 && (p[0] >= 0x60 && p[0] <= 0x64)
}

func dlmsACSEWireEvidence(w []byte) bool {
	p := dlmsAPDU(w)
	if !dlmsACSEAPDU(p) || !acseEnvelopeHint(p) {
		return false
	}
	m, err := decodeDLMSFrameBudget(w, 0, 0, true)
	if err != nil || m.kind != "I" || m.segmented || len(m.information) < 4 {
		return false
	}
	llc := m.information[:3]
	return bytes.Equal(llc, []byte{0xe6, 0xe6, 0}) && (p[0] == 0x60 || p[0] == 0x62 || p[0] == 0x64) || bytes.Equal(llc, []byte{0xe6, 0xe7, 0}) && (p[0] == 0x61 || p[0] == 0x63 || p[0] == 0x64)
}

func decodeDLMSACSEInformation(m *dlmsMessage, p []byte, llc byte, limit, depth int) (*dlmsMessage, error) {
	if depth < 4 {
		return nil, dlmsError(ErrResourceExceeded, "ACSE field depth exceeds recursion budget")
	}
	if (p[0] == 0x60 || p[0] == 0x62) && llc != 0xe6 || (p[0] == 0x61 || p[0] == 0x63) && llc != 0xe7 {
		return nil, dlmsError(ErrMalformedMessage, "ACSE command differs from LLC direction")
	}
	a, err := decodeACSEAPDU(p, limit)
	if err != nil {
		return nil, dlmsBlockError(err)
	}
	m.acse, m.kind, m.request = a, "ACSE", a.request
	m.fields["Frame Kind"], m.fields["ACSE"] = a.fields["kind"], a.fields
	return m, nil
}

// HDLC's observed modulo8 send/receive sequence is part of response ownership.
// Each logical-address/request-sequence tuple is retained and charged once;
// wrap/reuse never manufactures an association generation. Exact active retries
// preserve the original request ID without advancing either peer sequence.
func (s *binDLMS) consumeACSE(m *dlmsMessage, w []byte, dir int, id uint64, limit int) (map[string]any, uint64, error) {
	ctx := func(why string) (map[string]any, uint64, error) { return nil, 0, dlmsError(ErrContextRequired, why) }
	if s.ambiguous {
		return ctx("ACSE requires trustworthy HDLC conversation")
	}
	a := m.acse
	if a.acseRelease || a.acseAbort {
		return s.consumeLifecycle(m, w, dir, id, limit)
	}
	if m.request {
		if s.transfer != nil || s.fragments != nil {
			s.invalidate()
			return ctx("ACSE interrupts active Get transfer")
		}
		if s.clientKnown && dir != s.clientDir {
			return ctx("ACSE requester direction differs from observed link")
		}
		if p := s.pending; p != nil {
			if p.kind == "ACSE" && p.dir == dir && bytes.Equal(p.wire, w) {
				m.fields["Association"] = "retransmitted-association-request"
				m.fields["Association Request Event"] = p.id
				return m.fields, 0, nil
			}
			s.invalidate()
			return ctx("ACSE overlaps distinct outstanding request")
		}
		key := dlmsACSEKey{dir, m.source, m.destination}
		if s.acseSeen[key].used&(1<<m.ns) != 0 {
			s.invalidate()
			return ctx("ACSE logical-address/send-sequence tuple reused")
		}
		if s.seqKnown[dir] && m.ns != s.next[dir] || s.seqKnown[1-dir] && m.nr != s.next[1-dir] {
			s.invalidate()
			return ctx("ACSE request sequence differs from observed progression")
		}
		if _, ok := s.acseSeen[key]; !ok && len(s.acseSeen) >= sessionCollectionLimit(limit) {
			s.invalidate()
			return nil, 0, dlmsError(ErrResourceExceeded, "ACSE retained address history exceeds budget")
		}
		if s.acseSeen == nil {
			s.acseSeen = make(map[dlmsACSEKey]dlmsACSEHistory)
		}
		h := s.acseSeen[key]
		h.used |= 1 << m.ns
		s.acseSeen[key] = h
		s.clientKnown, s.clientDir = true, dir
		s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
		s.pending = &dlmsPending{wire: bytes.Clone(w), kind: "ACSE", acseCommand: 0x60, acseInfo: a.acseInfo, acseLowAuthentication: a.acseLowAuthentication, acseConformance: a.acseConformance, acseMaxPDU: a.acseMaxPDU, dir: dir, source: m.source, destination: m.destination, id: id, ns: m.ns}
		m.fields["Association"] = "observed-association-request"
		if !a.acseInfo {
			m.fields["Association"] = "unverified-association-initiate-context"
		}
		return m.fields, 0, nil
	}
	p := s.pending
	if p == nil {
		// Observing AARE proves possible prior use of its acknowledged request
		// sequence, not the request's identity. Retain a charged refusal token
		// so the same late response cannot own a later AARQ with that tuple.
		key := dlmsACSEKey{1 - dir, m.destination, m.source}
		if _, ok := s.acseSeen[key]; !ok && len(s.acseSeen) >= sessionCollectionLimit(limit) {
			s.invalidate()
			return nil, 0, dlmsError(ErrResourceExceeded, "orphan ACSE address history exceeds budget")
		}
		if s.acseSeen == nil {
			s.acseSeen = make(map[dlmsACSEKey]dlmsACSEHistory)
		}
		h := s.acseSeen[key]
		h.used |= 1 << ((m.nr - 1) & 7)
		s.acseSeen[key] = h
		s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
	}
	if p == nil || p.kind != "ACSE" || p.acseCommand != 0x60 || p.dir == dir || p.source != m.destination || p.destination != m.source {
		return ctx("AARE lacks reversed observed carrier/logical-address AARQ")
	}
	if m.nr != (p.ns+1)&7 || s.seqKnown[dir] && m.ns != s.next[dir] {
		return ctx("AARE send/receive sequence differs from observed AARQ")
	}
	if a.acseAccepted && a.acseInfo && p.acseInfo && a.acseConformance & ^p.acseConformance != 0 {
		s.invalidate()
		return ctx("AARE selects unproposed conformance")
	}
	s.pending = nil
	if a.acseAccepted && a.acseInfo && p.acseInfo {
		key := dlmsACSEKey{p.dir, p.source, p.destination}
		h := s.acseSeen[key]
		h.retired = false
		s.acseSeen[key] = h
	}
	s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
	if !p.acseInfo || a.acseAccepted && !a.acseInfo {
		m.fields["Association"] = "unverified-association-initiate-context"
		return m.fields, 0, nil
	}
	a.fields["proposed_conformance_bits"], a.fields["proposed_max_pdu_size"] = p.acseConformance, p.acseMaxPDU
	a.fields["association_observed"], a.fields["association_request_event"] = true, p.id
	if p.acseLowAuthentication {
		a.fields["proposed_authentication_mechanism_oid"] = "2.16.756.5.8.2.1"
	}
	m.fields["Association"] = "observed-association-response"
	return m.fields, p.id, nil
}
