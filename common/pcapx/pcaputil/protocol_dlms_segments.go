package pcaputil

import (
	"bytes"
	"encoding/hex"
)

// One selected response information field, not APDU data-blocks or GBT.
// Every complete carrier frame is independently length/HCS/FCS validated.
// The first frame must include LLC and the complete LN GET service header.
// A partial response never consumes the observed request's identity.
type dlmsFragments struct {
	info, wire          []byte
	dir                 int
	source, destination uint32
	first, last, nr     byte
	count               int
}

func (s *binDLMS) fragmentCloseFields() map[string]any {
	t := s.fragments
	fields := map[string]any{"Outstanding": 1, "ObservedFrames": t.count, "InformationBytes": len(t.info)}
	if s.transfer != nil {
		fields["ObservedBlocks"], fields["EncodedBytes"] = s.transfer.blocks, len(s.transfer.data)
	}
	return fields
}

func (f *binFlow) dlmsLinkAssembly(w []byte) bool {
	return f.dlms.fragments != nil || len(w) >= 3 && w[1]&8 != 0
}

func (f *binFlow) dlmsProjection(w []byte) int64 {
	if f.dlmsLinkAssembly(w) {
		// Reserve joined information, the fixed Data graph and list expansion
		// before retaining any fragment or decoding a final assembled body.
		return 32768 + 512*int64(len(w)+wrapperBlockBytes+3) + 2048*wrapperDataNodes + 8192*wrapperListItems
	}
	return dlmsProjection(w)
}

func dlmsSegmentFields(m *dlmsMessage, count int, complete bool) map[string]any {
	return map[string]any{"more": m.segmented, "complete": complete, "observed_frames": count, "information_hex": hex.EncodeToString(m.information)}
}

func (f *binFlow) consumeDLMSSegments(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s := f.dlms
	ctx := func(why string, retire bool) (map[string]any, uint64, error) {
		if retire {
			s.invalidate()
		}
		return nil, 0, dlmsError(ErrContextRequired, why)
	}
	if s.ambiguous {
		return ctx("segmented response lacks trustworthy conversation", false)
	}
	t := s.fragments
	if m.kind != "I" {
		if t != nil && m.kind == "S" && dir != t.dir && m.source == t.destination && m.destination == t.source {
			advance := (m.nr - (t.last+1)&7) & 7
			if advance > 0 && advance <= 4 {
				return ctx("supervisory acknowledgement advances beyond observed fragments", true)
			}
			m.fields["Association"] = "unassociated-link-observation"
			return m.fields, 0, nil
		}
		s.invalidate()
		if m.kind == "DM" {
			m.fields["Association"] = "unassociated-link-observation"
			return m.fields, 0, nil
		}
		return ctx("link control interrupts segmented response", false)
	}
	p := s.pending
	if t == nil && m.segmented && len(m.information) >= 4 && bytes.Equal(m.information[:3], []byte{0xe6, 0xe6, 0}) && m.information[3] == 0xc0 {
		s.invalidate()
		return nil, 0, dlmsError(ErrUnsupportedFeature, "segmented requests require a separate initial-request profile")
	}
	if p == nil || p.kind != "GET" || dir == p.dir || m.source != p.destination || m.destination != p.source {
		interrupt := p != nil && dir == p.dir && m.source == p.source && m.destination == p.destination
		// A new peer I-frame after completion can belong to a missed request.
		// Just rejecting this first fragment would let its replay bind to a
		// later request with the same observed sequence/invoke context.
		if p == nil && s.clientKnown && dir != s.clientDir && (!s.seqKnown[dir] || m.ns == s.next[dir]) {
			interrupt = true
		}
		return ctx("response fragments require reversed observed request endpoints", interrupt)
	}
	if t == nil {
		info := m.information
		if !m.segmented || len(info) < 6 || !bytes.Equal(info[:3], []byte{0xe6, 0xe7, 0}) || info[3] != 0xc4 {
			s.invalidate()
			return nil, 0, dlmsError(ErrUnsupportedFeature, "first response segment needs complete LLC and LN GET header")
		}
		choice, flags := info[4], info[5]
		if choice != 1 && choice != 2 && choice != 3 || flags&0x30 != 0 {
			s.invalidate()
			return nil, 0, dlmsError(ErrMalformedMessage, "segmented GET choice/invoke flags invalid")
		}
		if flags != p.flags || choice != p.choice && choice != 2 || m.nr != (p.ns+1)&7 || s.seqKnown[dir] && m.ns != s.next[dir] {
			return ctx("first response segment service/flags/sequence differ from observed request", false)
		}
		t = &dlmsFragments{dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
	} else {
		if dir != t.dir || m.source != t.source || m.destination != t.destination || m.nr != t.nr {
			return ctx("continuation direction/address/acknowledgement differ", false)
		}
		if bytes.Equal(t.wire, wire) {
			m.fields["Frame Kind"] = "Segmented Get Response"
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
		m.fields["Frame Kind"] = "Segmented Get Response"
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
	fields, reply, err := f.consumeDecodedDLMS(dir, wire, id, decoded)
	if err != nil {
		s.invalidate()
		return nil, 0, err
	}
	s.next[dir], s.seqKnown[dir] = (last+1)&7, true
	meta["assembled_information_hex"] = hex.EncodeToString(joined)
	fields["HDLC Segmentation"] = meta
	return fields, reply, nil
}
