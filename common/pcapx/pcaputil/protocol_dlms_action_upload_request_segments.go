package pcaputil

import (
	"bytes"
	"encoding/hex"
	"math/bits"
)

// Selected single-method Action upload service blocks can independently span bounded HDLC I frames.
// Partial carriers never install service identity or a response owner.
// Keep every literal frame independent; only a complete APDU installs pending
// request context. A response must acknowledge the last request frame.
func (f *binFlow) consumeDLMSActionRequestSegments(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
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
		if !dlmsActionRequestSegmentPrefix(info) {
			return fail(ErrUnsupportedFeature, "first Action upload request segment needs complete LLC/service header", true)
		}
		if info[5]&0x30 != 0 {
			return fail(ErrMalformedMessage, "segmented request invoke flags are reserved", true)
		}
		nextRequest := info[4] == 6
		choice := info[4]
		if nextRequest {
			if s.actionTransfer == nil || s.actionTransfer.initial.choice != 4 {
				return fail(ErrContextRequired, "segmented Action-next lacks observed Action transfer", true)
			}
			p := &s.actionTransfer.initial
			choice = p.choice
			if dir != p.dir || m.source != p.source || m.destination != p.destination || info[5] != p.flags {
				return fail(ErrContextRequired, "segmented Action-next direction/address/full flags differ", false)
			}
			if s.pending != nil || s.actionTransfer.last {
				return fail(ErrContextRequired, "segmented Action-next lacks preceding ACK", true)
			}
		} else if s.pending != nil || s.transfer != nil || s.setTransfer != nil || s.actionTransfer != nil {
			return fail(ErrContextRequired, "segmented Action upload first lacks compatible observed context", true)
		}
		if s.ambiguous {
			return fail(ErrContextRequired, "segmented Action upload lacks trustworthy conversation", false)
		}
		if s.clientKnown && dir != s.clientDir || s.seqKnown[1-dir] && m.nr != s.next[1-dir] {
			return fail(ErrContextRequired, "initial Action upload request direction/acknowledgement differs", false)
		}
		if s.seqKnown[dir] && m.ns != s.next[dir] {
			return fail(ErrContextRequired, "initial Action upload request sequence differs", nextRequest)
		}
		t = &dlmsFragments{operation: "Action", actionRequestChoice: choice, request: true, nextRequest: nextRequest, invoke: info[5] & 15, dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
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
	// identity is the final event. Initial first-pblock requests retire fragment
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
	} // consume recorded the first initial service-block identity.
	s.pending.ns = last
	s.pending.requestInformation = bytes.Clone(joined)
	s.pending.requestFrames = count
	s.next[dir], s.seqKnown[dir] = (last+1)&7, true
	meta["assembled_information_hex"] = hex.EncodeToString(joined)
	fields["HDLC Segmentation"] = meta
	return fields, reply, nil
}

func dlmsActionRequestSegmentPrefix(info []byte) bool {
	return len(info) >= 6 && info[0] == 0xe6 && info[1] == 0xe6 && info[2] == 0 && info[3] == 0xc3 && (info[4] == 4 || info[4] == 6)
}

// Snapshot before consume/refusal can release the transfer. A known next6 needs
// matching literal endpoints/full flags or an already retained fragment chain.
func dlmsActionRequestSegmentProfile(choice byte) string {
	if choice == 4 {
		return "dlms-hdlc-action-upload-request-segmented"
	}
	return ""
}
func (f *binFlow) dlmsActionRequestSegmentObservation(w []byte, dir int) string {
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
	if s != nil && s.fragments != nil && s.fragments.actionRequestChoice != 0 {
		return dlmsActionRequestSegmentProfile(s.fragments.actionRequestChoice)
	}
	if s != nil && s.pending != nil && len(s.pending.requestInformation) > 0 && bytes.Equal(w, s.pending.wire) && dlmsActionRequestSegmentPrefix(s.pending.requestInformation) {
		return dlmsActionRequestSegmentProfile(s.pending.choice)
	}
	if w[1]&8 == 0 {
		return ""
	}
	p := dlmsAPDU(w)
	if len(p) < 3 || p[0] != 0xc3 {
		return ""
	}
	if p[1] == 4 {
		return dlmsActionRequestSegmentProfile(p[1])
	}
	if p[1] != 6 || s == nil || s.actionTransfer == nil {
		return ""
	}
	old := s.actionTransfer.initial
	if dir == old.dir && src == old.source && dst == old.destination && p[2] == old.flags {
		return dlmsActionRequestSegmentProfile(old.choice)
	}
	return ""
}
