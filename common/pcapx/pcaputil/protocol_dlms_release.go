package pcaputil

import (
	"bytes"
	"encoding/hex"
)

func acseLifecycleAPDU(p []byte) bool { return len(p) > 0 && p[0] >= 0x62 && p[0] <= 0x64 }

// X.227:1995 release reason is OPTIONAL, not DEFAULT. Absence never invents
// a normal reason. Selected DLMS user-information reuses the exact initiate
// codec; abort information remains a literal observation without a role claim.
func decodeACSELifecycle(p []byte, limit int) (*wrapperMessage, error) {
	if len(p) > min(limit, 4096) {
		return nil, wrapperError(ErrResourceExceeded, "ACSE lifecycle field-byte budget")
	}
	c := acseCursor{w: p}
	tag, body, err := c.tlv()
	if err != nil {
		return nil, err
	}
	if c.at != len(p) {
		return nil, wrapperError(ErrMalformedMessage, "ACSE lifecycle trailing bytes")
	}
	f := map[string]any{"kind": "RLRQ", "command": tag, "apdu_hex": hex.EncodeToString(p), "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "reason_present": false, "user_information_present": false}
	m := &wrapperMessage{fields: f, request: tag == 0x62, acse: true, acseRelease: tag != 0x64, acseAbort: tag == 0x64}
	if tag == 0x63 {
		f["kind"] = "RLRE"
	}
	if tag == 0x64 {
		f["kind"] = "ABRT"
		delete(f, "reason_present")
		f["diagnostic_present"] = false
	}
	parts := acseCursor{w: body}
	seen := map[byte]bool{}
	var info []byte
	for parts.at < len(body) {
		if len(seen) >= min(limit, 16) {
			return nil, wrapperError(ErrResourceExceeded, "ACSE lifecycle component budget")
		}
		t, v, e := parts.tlv()
		if e != nil {
			return nil, e
		}
		if seen[t] {
			return nil, wrapperError(ErrMalformedMessage, "duplicate ACSE lifecycle component")
		}
		seen[t] = true
		switch t {
		case 0x80:
			if len(v) != 1 {
				return nil, wrapperError(ErrMalformedMessage, "ACSE lifecycle INTEGER size")
			}
			if tag == 0x64 {
				if v[0] > 1 {
					return nil, wrapperError(ErrUnsupportedFeature, "unselected abort source")
				}
				f["abort_source"] = v[0]
			} else {
				if v[0] != 0 && v[0] != 1 && v[0] != 30 {
					return nil, wrapperError(ErrUnsupportedFeature, "unselected release reason")
				}
				f["reason_present"], f["reason"] = true, v[0]
			}
		case 0x81:
			if tag != 0x64 {
				return nil, wrapperError(ErrUnsupportedFeature, "release extension outside selected profile")
			}
			if len(v) != 1 {
				return nil, wrapperError(ErrMalformedMessage, "abort diagnostic INTEGER size")
			}
			if v[0] < 1 || v[0] > 6 {
				return nil, wrapperError(ErrUnsupportedFeature, "unselected abort diagnostic")
			}
			f["diagnostic_present"], f["abort_diagnostic"] = true, v[0]
		case 0xbe:
			info, e = acseOnlyTLV(v, 4)
			if e != nil {
				return nil, e
			}
			f["user_information_present"], f["user_information_hex"] = true, hex.EncodeToString(info)
		default:
			return nil, wrapperError(ErrUnsupportedFeature, "unselected ACSE lifecycle component")
		}
	}
	if tag == 0x64 {
		if !seen[0x80] {
			return nil, wrapperError(ErrMalformedMessage, "abort source mandatory")
		}
		if seen[0xbe] {
			f["user_information_semantics"] = "unverified-abort-information"
		}
		return m, nil
	}
	f["initiate_information_present"] = seen[0xbe]
	if seen[0xbe] {
		if len(info) == 0 {
			return nil, wrapperError(ErrMalformedMessage, "empty selected release initiate")
		}
		m.acseInfo = true
		return decodeACSEInitiate(m, info)
	}
	return m, nil
}

func acseLifecycleEnvelopeHint(p []byte) bool {
	c := acseCursor{w: p}
	tag, body, err := c.tlv()
	if err != nil || c.at != len(p) {
		return false
	}
	seen := uint8(0)
	parts := acseCursor{w: body}
	for parts.at < len(body) {
		t, v, e := parts.tlv()
		if e != nil {
			return false
		}
		var bit uint8
		switch t {
		case 0x80:
			bit = 1
			if len(v) != 1 {
				return false
			}
			if tag == 0x64 {
				if v[0] > 1 {
					return false
				}
			} else if v[0] != 0 && v[0] != 1 && v[0] != 30 {
				return false
			}
		case 0x81:
			bit = 2
			if tag != 0x64 || len(v) != 1 || v[0] < 1 || v[0] > 6 {
				return false
			}
		case 0xbe:
			bit = 4
			if _, e := acseOnlyTLV(v, 4); e != nil {
				return false
			}
		default:
			return false
		}
		if seen&bit != 0 {
			return false
		}
		seen |= bit
	}
	return tag != 0x64 || seen&1 != 0
}

func (s *binDLMSWrapper) wrapperEndpointClosed(dir int, src, dst uint16, request bool) bool {
	if !request {
		dir, src, dst = 1-dir, dst, src
	}
	p := s.associations[wrapperAssociationKey{dir, src, dst}]
	return p != nil && p.closed
}
func wrapperSameEndpoints(src, dst, a, b uint16) bool {
	return src == a && dst == b || src == b && dst == a
}

func (s *binDLMSWrapper) retireWrapperEndpoint(src, dst uint16) {
	for key, p := range s.seen {
		if wrapperSameEndpoints(src, dst, key.source, key.destination) {
			p.ambiguousBlocks()
		}
	}
	for key, p := range s.gbt {
		if wrapperSameEndpoints(src, dst, key.source, key.destination) {
			p.data = nil
			p.active = false
			p.ambiguous = true
		}
	}
	for key, p := range s.associations {
		if wrapperSameEndpoints(src, dst, key.source, key.destination) {
			p.pending = false
			p.ambiguous = true
			p.closed = true
		}
	}
}

func (f *binFlow) consumeWrapperLifecycle(s *binDLMSWrapper, m *wrapperMessage, dir int, e *ProtocolEvent) (string, error) {
	key := wrapperAssociationKey{dir, m.source, m.destination}
	if !m.request {
		key = wrapperAssociationKey{1 - dir, m.destination, m.source}
	}
	if !m.acseAbort {
		for k := range s.associations {
			if k.source == key.source && k.destination == key.destination && k.dir != key.dir {
				return "unverified-release-direction", nil
			}
		}
		for k := range s.seen {
			if k.source == key.source && k.destination == key.destination && k.dir != key.dir {
				return "unverified-release-direction", nil
			}
		}
	}
	if m.acseAbort {
		for _, k := range []wrapperAssociationKey{key, {1 - key.dir, key.destination, key.source}} {
			p := s.associations[k]
			if p == nil {
				if len(s.associations) >= sessionCollectionLimit(f.a.budget.MaxCollectionElements) {
					s.invalidate()
					return "", wrapperError(ErrResourceExceeded, "abort endpoint history budget")
				}
				if s.associations == nil {
					s.associations = make(map[wrapperAssociationKey]*wrapperAssociation)
				}
				p = &wrapperAssociation{}
				s.associations[k] = p
			}
			p.releasePending = false
			p.releaseSeen = true
			p.releaseAmbiguous = true
			p.closed = true
		}
		s.retireWrapperEndpoint(m.source, m.destination)
		m.fields["eligibility_retired"] = true
		return "observed-association-abort", nil
	}
	p := s.associations[key]
	if p == nil {
		if len(s.associations) >= sessionCollectionLimit(f.a.budget.MaxCollectionElements) {
			s.invalidate()
			return "", wrapperError(ErrResourceExceeded, "release endpoint history budget")
		}
		if s.associations == nil {
			s.associations = make(map[wrapperAssociationKey]*wrapperAssociation)
		}
		p = &wrapperAssociation{}
		s.associations[key] = p
	}
	if m.request {
		s.retireWrapperEndpoint(m.source, m.destination)
		m.fields["eligibility_retired"] = true
		if p.releaseSeen {
			p.releasePending = false
			p.releaseAmbiguous = true
			return "ambiguous-release-generation", nil
		}
		p.releaseSeen, p.releasePending, p.releaseID = true, true, e.ID
		e.TransactionID = e.ID
		return "observed-release-request", nil
	}
	if !p.releaseSeen {
		p.releaseSeen, p.releaseAmbiguous, p.closed = true, true, true
		s.retireWrapperEndpoint(m.source, m.destination)
		return "unmatched-release-response", nil
	}
	if p.releaseAmbiguous || !p.releasePending {
		return "ambiguous-release-generation", nil
	}
	p.releasePending = false
	p.closed = true
	e.TransactionID, e.ResponseTo = p.releaseID, p.releaseID
	m.fields["release_request_event"], m.fields["release_observed"] = p.releaseID, true
	return "observed-release-response", nil
}

type dlmsACSEHistory struct {
	used    uint8
	retired bool
}

func (s *binDLMS) hdlcEndpointRetired(dir int, src, dst uint32, request bool) bool {
	if !request {
		dir, src, dst = 1-dir, dst, src
	}
	return s.acseSeen[dlmsACSEKey{dir, src, dst}].retired
}
func (s *binDLMS) retireHDLCEndpoint(dir int, src, dst uint32, limit int) error {
	for _, key := range []dlmsACSEKey{{dir, src, dst}, {1 - dir, dst, src}} {
		if _, ok := s.acseSeen[key]; !ok && len(s.acseSeen) >= sessionCollectionLimit(limit) {
			s.invalidate()
			return dlmsError(ErrResourceExceeded, "lifecycle endpoint history budget")
		}
		if s.acseSeen == nil {
			s.acseSeen = make(map[dlmsACSEKey]dlmsACSEHistory)
		}
		h := s.acseSeen[key]
		h.retired = true
		s.acseSeen[key] = h
	}
	if p := s.pending; p != nil && (p.source == src && p.destination == dst || p.source == dst && p.destination == src) {
		s.pending = nil
	}
	if p := s.transfer; p != nil && (p.initial.source == src && p.initial.destination == dst || p.initial.source == dst && p.initial.destination == src) {
		s.transfer = nil
	}
	if p := s.setTransfer; p != nil && (p.initial.source == src && p.initial.destination == dst || p.initial.source == dst && p.initial.destination == src) {
		s.setTransfer = nil
	}
	if p := s.fragments; p != nil && (p.source == src && p.destination == dst || p.source == dst && p.destination == src) {
		s.fragments = nil
	}
	return nil
}

func (s *binDLMS) consumeLifecycle(m *dlmsMessage, w []byte, dir int, id uint64, limit int) (map[string]any, uint64, error) {
	ctx := func(why string) (map[string]any, uint64, error) { return nil, 0, dlmsError(ErrContextRequired, why) }
	a := m.acse
	if a.acseAbort {
		if err := s.retireHDLCEndpoint(dir, m.source, m.destination, limit); err != nil {
			return nil, 0, err
		}
		a.fields["eligibility_retired"] = true
		m.fields["Association"] = "observed-association-abort"
		return m.fields, 0, nil
	}
	if s.ambiguous {
		return ctx("release requires trustworthy HDLC conversation")
	}
	if m.request {
		if s.clientKnown && dir != s.clientDir {
			return ctx("release requester contradicts observed direction")
		}
		if p := s.pending; p != nil && p.kind == "ACSE" && p.acseCommand == 0x62 && p.dir == dir && bytes.Equal(p.wire, w) {
			m.fields["Association"] = "retransmitted-release-request"
			m.fields["Association Request Event"] = p.id
			return m.fields, 0, nil
		}
		if p := s.pending; p != nil && (p.source != m.source || p.destination != m.destination) {
			return ctx("release does not belong to pending logical endpoint")
		}
		key := dlmsACSEKey{dir, m.source, m.destination}
		h := s.acseSeen[key]
		if err := s.retireHDLCEndpoint(dir, m.source, m.destination, limit); err != nil {
			return nil, 0, err
		}
		a.fields["eligibility_retired"] = true
		if h.used&(1<<m.ns) != 0 || s.seqKnown[dir] && m.ns != s.next[dir] || s.seqKnown[1-dir] && m.nr != s.next[1-dir] {
			s.invalidate()
			return ctx("release sequence reused or differs from observed progression")
		}
		h = s.acseSeen[key]
		h.used |= 1 << m.ns
		s.acseSeen[key] = h
		s.clientKnown, s.clientDir = true, dir
		s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
		s.pending = &dlmsPending{wire: bytes.Clone(w), kind: "ACSE", acseCommand: 0x62, dir: dir, source: m.source, destination: m.destination, id: id, ns: m.ns}
		m.fields["Association"] = "observed-release-request"
		return m.fields, 0, nil
	}
	p := s.pending
	if p == nil {
		key := dlmsACSEKey{1 - dir, m.destination, m.source}
		if err := s.retireHDLCEndpoint(key.dir, key.source, key.destination, limit); err != nil {
			return nil, 0, err
		}
		h := s.acseSeen[key]
		h.used |= 1 << ((m.nr - 1) & 7)
		s.acseSeen[key] = h
		s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
		return ctx("RLRE lacks observed release request")
	}
	if p.kind != "ACSE" || p.acseCommand != 0x62 || p.dir == dir || p.source != m.destination || p.destination != m.source {
		return ctx("RLRE differs from pending release endpoint/direction")
	}
	if m.nr != (p.ns+1)&7 || s.seqKnown[dir] && m.ns != s.next[dir] {
		return ctx("RLRE acknowledgement or peer sequence differs")
	}
	s.pending = nil
	s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
	a.fields["release_observed"], a.fields["release_request_event"] = true, p.id
	m.fields["Association"] = "observed-release-response"
	return m.fields, p.id, nil
}
