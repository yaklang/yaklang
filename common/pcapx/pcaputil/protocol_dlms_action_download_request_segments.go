package pcaputil

import (
	"bytes"
	"encoding/hex"
)

// Selected knownnext normal-root Action download requests can independently span bounded HDLC I frames.
// Partial carriers never install service identity or a response owner.
// Keep every literal frame independent; only a complete APDU installs pending
// request context. A response must acknowledge the last request frame.
func (f *binFlow) consumeDLMSActionDownloadRequestSegments(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s := f.dlms
	fail := func(kind ProtocolErrorKind, why string, retire bool) (map[string]any, uint64, error) {
		if retire {
			s.invalidate()
		}
		return nil, 0, dlmsError(kind, why)
	}
	t := s.fragments
	if t == nil {
		info := m.information
		if !dlmsActionDownloadRequestSegmentPrefix(info) {
			return fail(ErrUnsupportedFeature, "first Action download request segment needs complete LLC/service header", true)
		}
		if info[5]&0x30 != 0 {
			return fail(ErrMalformedMessage, "segmented request invoke flags are reserved", true)
		}
		if s.transfer == nil || s.transfer.initial.kind != "ACTION" || s.transfer.initial.choice != 1 {
			return fail(ErrContextRequired, "segmented Action-next lacks observed normal-root Action download", true)
		}
		p := &s.transfer.initial
		if dir != p.dir || m.source != p.source || m.destination != p.destination || info[5] != p.flags {
			return fail(ErrContextRequired, "segmented Action-next direction/address/full flags differ", false)
		}
		if s.pending != nil {
			return fail(ErrContextRequired, "segmented Action-next lacks preceding response", true)
		}
		if s.ambiguous {
			return fail(ErrContextRequired, "segmented Action download lacks trustworthy conversation", false)
		}
		if s.clientKnown && dir != s.clientDir || s.seqKnown[1-dir] && m.nr != s.next[1-dir] {
			return fail(ErrContextRequired, "initial Action download request direction/acknowledgement differs", false)
		}
		if s.seqKnown[dir] && m.ns != s.next[dir] {
			return fail(ErrContextRequired, "initial Action download request sequence differs", true)
		}
		t = &dlmsFragments{operation: "Action", actionDownloadRequestChoice: 2, request: true, nextRequest: true, invoke: info[5] & 15, dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
	} else {
		if dir != t.dir || m.source != t.source || m.destination != t.destination || m.nr != t.nr {
			return fail(ErrContextRequired, "request continuation direction/address/acknowledgement differ", false)
		}
		if bytes.Equal(t.wire, wire) {
			m.fields["Frame Kind"] = "Segmented " + t.serviceName() + " Request"
			m.fields["HDLC Segmentation"] = dlmsSegmentFields(m, t.count, false)
			m.fields["Association"] = "retransmitted-request-segment"
			return m.fields, 0, nil
		}
		if m.ns != (t.last+1)&7 {
			return fail(ErrContextRequired, "missing/conflicting/repeated request fragment sequence", true)
		}
	}
	if len(m.information) == 0 {
		return fail(ErrMalformedMessage, "empty request continuation information", true)
	}
	if t.count >= min(wrapperBlockCount, f.a.budget.MaxCollectionElements) || len(m.information) > min(wrapperBlockBytes+3, f.a.budget.MaxCollectionElements)-len(t.info) {
		return fail(ErrResourceExceeded, "request HDLC information bytes/frame count exceed selected/caller pool", true)
	}
	joined := make([]byte, len(t.info)+len(m.information))
	copy(joined, t.info)
	copy(joined[len(t.info):], m.information)
	count := t.count + 1
	meta := dlmsSegmentFields(m, count, !m.segmented)
	if m.segmented {
		t.info, t.wire, t.last, t.count = joined, bytes.Clone(wire), m.ns, count
		s.fragments = t
		m.fields["Frame Kind"] = "Segmented " + t.serviceName() + " Request"
		m.fields["HDLC Segmentation"] = meta
		m.fields["Association"] = "observed-request-segment"
		return m.fields, 0, nil
	}
	// Link progression validates from the first sequence; only the final
	// captured request event installs an owner, acknowledged at its final N(S).
	last := m.ns
	m.ns = t.first
	decoded, err := decodeDLMSInformation(m, joined, f.a.budget.MaxCollectionElements, f.a.budget.MaxRecursionDepth)
	if err != nil {
		return failDLMSRequestAssembly(s, err)
	}
	s.fragments = nil
	fields, reply, err := f.consumeDecodedDLMS(dir, wire, id, decoded)
	if err != nil {
		return failDLMSRequestAssembly(s, err)
	}
	s.pending.ns = last
	s.pending.requestInformation = bytes.Clone(joined)
	s.pending.requestFrames = count
	s.next[dir], s.seqKnown[dir] = (last+1)&7, true
	meta["assembled_information_hex"] = hex.EncodeToString(joined)
	fields["HDLC Segmentation"] = meta
	return fields, reply, nil
}

func dlmsActionDownloadRequestSegmentPrefix(info []byte) bool {
	return len(info) >= 6 && info[0] == 0xe6 && info[1] == 0xe6 && info[2] == 0 && info[3] == 0xc3 && info[4] == 2
}

// Snapshot before consume/refusal can release the normal-root download. A known next2 needs
// matching literal endpoints/full flags or an already retained fragment chain.
func dlmsActionDownloadRequestSegmentProfile(choice byte) string {
	if choice == 2 {
		return "dlms-hdlc-action-download-request-segmented"
	}
	return ""
}
func (f *binFlow) dlmsActionDownloadRequestSegmentObservation(w []byte, dir int) string {
	if len(w) < 8 || w[0] != 0x7e {
		return ""
	}
	at, end := 3, len(w)-3
	dst, _, err := dlmsAddress(w[:end], &at)
	if err != nil {
		return ""
	}
	src, _, err := dlmsAddress(w[:end], &at)
	if err != nil || at >= end || w[at]&1 != 0 {
		return ""
	}
	s := f.dlms
	if s != nil && s.fragments != nil && s.fragments.actionDownloadRequestChoice != 0 {
		return dlmsActionDownloadRequestSegmentProfile(s.fragments.actionDownloadRequestChoice)
	}
	if s != nil && s.pending != nil && len(s.pending.requestInformation) > 0 && bytes.Equal(w, s.pending.wire) && dlmsActionDownloadRequestSegmentPrefix(s.pending.requestInformation) {
		return dlmsActionDownloadRequestSegmentProfile(s.pending.choice)
	}
	if w[1]&8 == 0 {
		return ""
	}
	p := dlmsAPDU(w)
	if len(p) < 3 || p[0] != 0xc3 {
		return ""
	}
	if p[1] != 2 || s == nil || s.transfer == nil || s.transfer.initial.kind != "ACTION" || s.transfer.initial.choice != 1 {
		return ""
	}
	old := s.transfer.initial
	if dir == old.dir && src == old.source && dst == old.destination && p[2] == old.flags {
		return dlmsActionDownloadRequestSegmentProfile(2)
	}
	return ""
}
