package pcaputil

import (
	"bytes"
	"encoding/hex"
)

// Selected SET-list ACK/terminal response information frames retain the pending
// request until a complete bounded APDU is validated; no partial reply owner.
func (f *binFlow) consumeDLMSSetListResponseSegments(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s := f.dlms
	ctx := func(why string, retire bool) (map[string]any, uint64, error) {
		if retire {
			s.invalidate()
		}
		return nil, 0, dlmsError(ErrContextRequired, why)
	}
	if s.ambiguous {
		return ctx("SET-list response fragments lack trustworthy conversation", false)
	}
	t := s.fragments
	p := s.pending
	if t != nil && t.request || t == nil && m.segmented && m.request {
		return f.consumeDLMSRequestSegments(dir, wire, id, m)
	}
	if p == nil || p.kind != "SET" || p.choice != 5 || s.setTransfer == nil || s.setTransfer.initial.choice != 5 || dir == p.dir || m.source != p.destination || m.destination != p.source {
		return ctx("SET-list response fragments require reversed observed list request", false)
	}
	if t == nil {
		info := m.information
		if !m.segmented || len(info) < 6 || !bytes.Equal(info[:3], []byte{0xe6, 0xe7, 0}) {
			return ctx("first SET-list response fragment needs complete LLC/header", false)
		}
		if info[5]&0x30 != 0 {
			s.invalidate()
			return nil, 0, dlmsError(ErrMalformedMessage, "segmented SET-list response reserved invoke flags")
		}
		if info[3] != 0xc5 || info[4] != 2 && info[4] != 4 || info[5] != p.flags || m.nr != (p.ns+1)&7 || s.seqKnown[dir] && m.ns != s.next[dir] {
			return ctx("SET-list response fragment service/form/flags/sequence differs", false)
		}
		t = &dlmsFragments{operation: "Set", setListResponse: true, dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
	} else {
		if dir != t.dir || m.source != t.source || m.destination != t.destination || m.nr != t.nr {
			return ctx("continuation direction/address/acknowledgement differ", false)
		}
		if bytes.Equal(t.wire, wire) {
			m.fields["Frame Kind"] = "Segmented " + t.serviceName() + " Response"
			m.fields["HDLC Segmentation"] = dlmsSegmentFields(m, t.count, false)
			m.fields["Association"] = "retransmitted-response-segment"
			return m.fields, 0, nil
		}
		if m.ns != (t.last+1)&7 {
			return ctx("missing/conflicting/repeated response fragment sequence", true)
		}
	}
	if len(m.information) == 0 {
		s.invalidate()
		return nil, 0, dlmsError(ErrMalformedMessage, "empty response continuation information")
	}
	if t.count >= min(wrapperBlockCount, f.a.budget.MaxCollectionElements) || len(m.information) > min(wrapperBlockBytes+3, f.a.budget.MaxCollectionElements)-len(t.info) {
		s.invalidate()
		return nil, 0, dlmsError(ErrResourceExceeded, "HDLC information bytes/frame count exceed selected/caller pool")
	}
	joined := make([]byte, len(t.info)+len(m.information))
	copy(joined, t.info)
	copy(joined[len(t.info):], m.information)
	count := t.count + 1
	meta := dlmsSegmentFields(m, count, !m.segmented)
	if m.segmented {
		t.info, t.wire, t.last, t.count = joined, bytes.Clone(wire), m.ns, count
		s.fragments = t
		m.fields["Frame Kind"] = "Segmented " + t.serviceName() + " Response"
		m.fields["HDLC Segmentation"] = meta
		m.fields["Association"] = "observed-response-segment"
		return m.fields, 0, nil
	}
	// Decode the assembled LLC/APDU directly. Captured frame header, CRCs,
	// Raw and sequence fields remain those of the final frame; no synthetic
	// HDLC wire is presented as captured input.
	last := m.ns
	m.ns = t.first
	decoded, err := decodeDLMSInformation(m, joined, f.a.budget.MaxCollectionElements, f.a.budget.MaxRecursionDepth)
	if err != nil {
		s.invalidate()
		return nil, 0, err
	}
	s.fragments = nil
	// Only this fully checked private group may carry list identity across a
	// final carrier without LLC/APDU. A foreign literal terminal3 cannot set it.
	decoded.setListResponseAssembly = true
	fields, reply, err := f.consumeDecodedDLMS(dir, wire, id, decoded)
	if err != nil {
		return failDLMSRequestAssembly(s, err)
	}
	s.next[dir], s.seqKnown[dir] = (last+1)&7, true
	meta["assembled_information_hex"] = hex.EncodeToString(joined)
	fields["HDLC Segmentation"] = meta
	return fields, reply, nil
}

func (f *binFlow) dlmsSetListResponseSegmentObservation(w []byte, dir int) bool {
	if len(w) < 8 || w[0] != 0x7e {
		return false
	}
	at, end := 3, len(w)-3
	dst, _, err := dlmsAddress(w[:end], &at)
	if err != nil {
		return false
	}
	src, _, err := dlmsAddress(w[:end], &at)
	if err != nil || at >= end || w[at]&1 != 0 {
		return false
	}
	s := f.dlms
	if s != nil && s.fragments != nil && s.fragments.setListResponse {
		return true
	}
	p := dlmsAPDU(w)
	if w[1]&8 == 0 || len(p) < 3 || p[0] != 0xc5 {
		return false
	}
	if p[1] == 4 {
		return true
	}
	if p[1] != 2 || s == nil || s.pending == nil || s.pending.kind != "SET" || s.pending.choice != 5 {
		return false
	}
	old := s.pending
	return dir != old.dir && src == old.destination && dst == old.source && p[2] == old.flags
}
