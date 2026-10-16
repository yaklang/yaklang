package pcaputil

import (
	"bytes"
	"encoding/hex"
	"math/bits"
)

// Selected single/list SET service blocks can independently span bounded HDLC I frames.
// Partial carriers never install service identity or a response owner.
// Keep every literal frame independent; only a complete APDU installs pending
// request context. A response must acknowledge the last request frame.
func (f *binFlow) consumeDLMSSetRequestSegments(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
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
		if !dlmsSetRequestSegmentPrefix(info) {
			return fail(ErrUnsupportedFeature, "first SET-list request segment needs complete LLC/service header", true)
		}
		if info[5]&0x30 != 0 {
			return fail(ErrMalformedMessage, "segmented request invoke flags are reserved", true)
		}
		nextRequest := info[4] == 3
		choice := info[4]
		if nextRequest {
			if s.setTransfer == nil || (s.setTransfer.initial.choice != 5 && s.setTransfer.initial.choice != 2) {
				return fail(ErrContextRequired, "segmented SET-next lacks observed list transfer", true)
			}
			p := &s.setTransfer.initial
			choice = p.choice
			if dir != p.dir || m.source != p.source || m.destination != p.destination || info[5] != p.flags {
				return fail(ErrContextRequired, "segmented SET-next direction/address/full flags differ", false)
			}
			if s.pending != nil || s.setTransfer.last {
				return fail(ErrContextRequired, "segmented SET-next lacks preceding ACK", true)
			}
		} else if s.pending != nil || s.transfer != nil || s.setTransfer != nil || s.actionTransfer != nil {
			return fail(ErrContextRequired, "segmented SET-list first lacks compatible observed context", true)
		}
		if s.ambiguous {
			return fail(ErrContextRequired, "segmented SET-list lacks trustworthy conversation", false)
		}
		if s.clientKnown && dir != s.clientDir || s.seqKnown[1-dir] && m.nr != s.next[1-dir] {
			return fail(ErrContextRequired, "initial SET-list request direction/acknowledgement differs", false)
		}
		if s.seqKnown[dir] && m.ns != s.next[dir] {
			return fail(ErrContextRequired, "initial SET-list request sequence differs", nextRequest)
		}
		t = &dlmsFragments{operation: "Set", setRequestChoice: choice, request: true, nextRequest: nextRequest, invoke: info[5] & 15, dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
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
	if !t.nextRequest && s.seenRequest[t.invoke]&(1<<m.ns) != 0 {
		return fail(ErrContextRequired, "segmented request reuses captured invoke/sequence identity", true)
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
	mask := t.sequenceSet | 1<<m.ns
	meta := dlmsSegmentFields(m, count, !m.segmented)
	if m.segmented {
		t.info, t.wire, t.last, t.count, t.sequenceSet = joined, bytes.Clone(wire), m.ns, count, mask
		s.fragments = t
		m.fields["Frame Kind"] = "Segmented " + t.serviceName() + " Request"
		m.fields["HDLC Segmentation"] = meta
		m.fields["Association"] = "observed-request-segment"
		return m.fields, 0, nil
	}
	// Session progression starts at the first sequence; the complete request
	// identity is the final event. Initial list-first requests retire fragment
	// identities; known next requests retain the service-transfer progression.
	newIdentities := bits.OnesCount8(mask &^ s.seenRequest[t.invoke])
	if !t.nextRequest && int(s.seenCount)+newIdentities > f.a.budget.MaxCollectionElements {
		return fail(ErrResourceExceeded, "segmented request identity history exceeds collection budget", true)
	}
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
	if !t.nextRequest {
		s.seenRequest[t.invoke] |= mask
		s.seenCount += uint16(newIdentities - 1)
	} // consume recorded the first initial normal/list identity.
	s.pending.ns = last
	s.pending.requestInformation = bytes.Clone(joined)
	s.pending.requestFrames = count
	s.next[dir], s.seqKnown[dir] = (last+1)&7, true
	meta["assembled_information_hex"] = hex.EncodeToString(joined)
	fields["HDLC Segmentation"] = meta
	return fields, reply, nil
}

func dlmsSetRequestSegmentPrefix(info []byte) bool {
	return len(info) >= 6 && info[0] == 0xe6 && info[1] == 0xe6 && info[2] == 0 && info[3] == 0xc1 && (info[4] == 2 || info[4] == 5 || info[4] == 3)
}

// Snapshot before consume/refusal can release the transfer. A shared next3 needs
// matching literal endpoints/full flags or an already retained fragment chain.
func dlmsSetRequestSegmentProfile(choice byte) string {
	if choice == 5 {
		return "dlms-hdlc-set-list-block-request-segmented"
	}
	if choice == 2 {
		return "dlms-hdlc-set-block-request-segmented"
	}
	return ""
}
func (f *binFlow) dlmsSetRequestSegmentObservation(w []byte, dir int) string {
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
	if s != nil && s.fragments != nil && s.fragments.setRequestChoice != 0 {
		return dlmsSetRequestSegmentProfile(s.fragments.setRequestChoice)
	}
	if s != nil && s.pending != nil && len(s.pending.requestInformation) > 0 && bytes.Equal(w, s.pending.wire) && dlmsSetRequestSegmentPrefix(s.pending.requestInformation) {
		return dlmsSetRequestSegmentProfile(s.pending.choice)
	}
	if w[1]&8 == 0 {
		return ""
	}
	p := dlmsAPDU(w)
	if len(p) < 3 || p[0] != 0xc1 {
		return ""
	}
	if p[1] == 2 || p[1] == 5 {
		return dlmsSetRequestSegmentProfile(p[1])
	}
	if p[1] != 3 || s == nil || s.setTransfer == nil {
		return ""
	}
	old := s.setTransfer.initial
	if dir == old.dir && src == old.source && dst == old.destination && p[2] == old.flags {
		return dlmsSetRequestSegmentProfile(old.choice)
	}
	return ""
}
