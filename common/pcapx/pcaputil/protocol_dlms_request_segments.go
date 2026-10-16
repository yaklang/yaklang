package pcaputil

import (
	"bytes"
	"encoding/hex"
	"math/bits"
)

// Selected normal/list and observed-transfer GET-next requests are wire
// observations, not proof of a negotiated AA.
// Keep every literal frame independent; only a complete APDU installs pending
// request context. A response must acknowledge the last request frame.
func (f *binFlow) consumeDLMSRequestSegments(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
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
		operation := ""
		if dlmsSelectedOperationSegmentPrefix(info) && info[1] == 0xe6 {
			if info[3] == 0xc1 {
				operation = "Set"
			} else {
				operation = "Action"
			}
		}
		if len(info) < 6 || operation == "" && (!bytes.Equal(info[:4], []byte{0xe6, 0xe6, 0, 0xc0}) || info[4] != 1 && info[4] != 2 && info[4] != 3) {
			return fail(ErrUnsupportedFeature, "first request segment needs complete LLC and selected GET header", true)
		}
		// Preserve the complete-APDU malformed boundary before comparing context:
		// reserved service bits are invalid bytes, not a different valid request.
		if info[5]&0x30 != 0 {
			return fail(ErrMalformedMessage, "segmented request invoke flags are reserved", true)
		}
		nextRequest := operation == "" && info[4] == 2
		// GET-next belongs to a completely observed block transfer. Partial request
		// bytes cannot replace a pending hop or create missing initial context.
		if s.pending != nil || s.transfer != nil && !nextRequest || nextRequest && s.transfer == nil {
			return fail(ErrUnsupportedFeature, "request segmentation lacks compatible observed exchange context", true)
		}
		if nextRequest {
			p := &s.transfer.initial
			if dir != p.dir || m.source != p.source || m.destination != p.destination || info[5] != p.flags {
				return fail(ErrContextRequired, "segmented Get-next direction/address/full flags differ from initial request", false)
			}
		}
		if s.clientKnown && dir != s.clientDir || s.seqKnown[dir] && m.ns != s.next[dir] || s.seqKnown[1-dir] && m.nr != s.next[1-dir] {
			return fail(ErrContextRequired, "initial request direction/sequence differs from observed progression", false)
		}
		t = &dlmsFragments{operation: operation, operationList: operation != "" && dlmsOperationListAPDU(info[3:]), request: true, nextRequest: nextRequest, invoke: info[5] & 15, dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
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
	// identity is the final event. Initial normal/list requests retire fragment
	// identities; observed GET-next instead requires the current transfer's block
	// number and progression, preserving consecutive modulo8 block exchanges.
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

func failDLMSRequestAssembly(s *binDLMS, err error) (map[string]any, uint64, error) {
	s.invalidate()
	return nil, 0, err
}
