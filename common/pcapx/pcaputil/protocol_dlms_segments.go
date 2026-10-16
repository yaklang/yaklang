package pcaputil

import (
	"bytes"
	"encoding/hex"
	"errors"
)

// One selected normal/list or observed-transfer next request, or GET response
// information field, not GBT.
// Every complete carrier frame is independently length/HCS/FCS validated.
// The first frame must include LLC and the complete LN GET service header.
// A partial request does not install pending context; a partial response never
// consumes the observed request's identity.
type dlmsFragments struct {
	info, wire                   []byte
	dir                          int
	source, destination          uint32
	first, last, nr              byte
	count                        int
	request                      bool
	nextRequest                  bool
	invoke, sequenceSet          byte
	acseCommand                  byte
	operation                    string
	operationList                bool
	actionDownloadResponseChoice byte
	actionResponseChoice         byte
	actionRequestChoice          byte
	setRequestChoice             byte
	setResponseChoice            byte
}

func (s *binDLMS) fragmentCloseDirection() int {
	if s.pending != nil {
		return s.pending.dir
	}
	return s.fragments.dir
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
	return f.dlms.fragments != nil || f.dlms.pending != nil && len(f.dlms.pending.requestInformation) > 0 && bytes.Equal(f.dlms.pending.wire, w) || len(w) >= 3 && w[1]&8 != 0
}

func (f *binFlow) dlmsProjection(w []byte) int64 {
	if f.dlms.setTransfer != nil && f.dlms.setTransfer.initial.choice == 5 {
		p := dlmsAPDU(w)
		if len(p) > 1 && p[0] == 0xc1 && p[1] == 3 {
			return 32768 + 512*int64(len(w)+wrapperBlockBytes) + 2048*wrapperDataNodes + 8192*wrapperListItems
		}
	}
	if f.dlms.fragments != nil && f.dlms.fragments.acseCommand != 0 || len(w) > 1 && w[1]&8 != 0 && dlmsACSEAPDU(dlmsAPDU(w)) {
		// ACSE has no list/compact expansion. Its independently bounded BER pool
		// still includes the assembled copy, literal frames and private projections.
		return 32768 + 512*int64(len(w)+4099) + 256
	}
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
	if t != nil && t.request || t == nil && m.segmented && m.request {
		return f.consumeDLMSRequestSegments(dir, wire, id, m)
	}
	if p == nil || p.kind != "GET" && !(dlmsOperationKind(p.kind) && (p.choice == 1 || dlmsOperationListPending(p))) || dir == p.dir || m.source != p.destination || m.destination != p.source {
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
		operation := ""
		if dlmsOperationKind(p.kind) {
			operation = dlmsOperationName(p.kind)
		}
		if operation != "" && len(info) >= 6 && bytes.Equal(info[:3], []byte{0xe6, 0xe7, 0}) {
			expected := byte(0xc5)
			if p.kind == "ACTION" {
				expected = 0xc7
			}
			if info[3] != expected || info[4] != dlmsOperationSegmentResponseChoice(p) {
				return ctx("segmented operation reply service/form differs from pending request", false)
			}
		}
		if dlmsSelectedOperationSegmentPrefix(info) {
			expected := byte(0xc5)
			if p.kind == "ACTION" {
				expected = 0xc7
			}
			if !dlmsOperationKind(p.kind) || info[1] != 0xe7 || info[3] != expected || info[4] != dlmsOperationSegmentResponseChoice(p) {
				return ctx("segmented operation response differs from known pending service/form", false)
			}
		}
		if !m.segmented || len(info) < 6 || !bytes.Equal(info[:3], []byte{0xe6, 0xe7, 0}) || info[3] != 0xc4 && !(operation != "" && dlmsSelectedOperationSegmentPrefix(info)) {
			s.invalidate()
			return nil, 0, dlmsError(ErrUnsupportedFeature, "first response segment needs complete LLC and LN GET header")
		}
		choice, flags := info[4], info[5]
		if operation == "" && choice != 1 && choice != 2 && choice != 3 || flags&0x30 != 0 {
			s.invalidate()
			return nil, 0, dlmsError(ErrMalformedMessage, "segmented GET choice/invoke flags invalid")
		}
		if flags != p.flags || operation == "" && choice != p.choice && choice != 2 || operation != "" && choice != dlmsOperationSegmentResponseChoice(p) || m.nr != (p.ns+1)&7 || s.seqKnown[dir] && m.ns != s.next[dir] {
			return ctx("first response segment service/flags/sequence differ from observed request", false)
		}
		t = &dlmsFragments{operation: operation, operationList: dlmsOperationListPending(p), dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
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
	fields, reply, err := f.consumeDecodedDLMS(dir, wire, id, decoded)
	if err != nil {
		// An assembled list response can have a valid body but a count that does
		// not match the observed request. Drop only its fragment group, just as
		// the complete unsegmented list policy preserves the pending owner. Never
		// commit the refused server sequence or claim a later device progression.
		var pe *ProtocolError
		if !(t.operationList && s.pending == p && !s.ambiguous && errors.As(err, &pe) && pe.Kind == ErrContextRequired) {
			s.invalidate()
		}
		return nil, 0, err
	}
	s.next[dir], s.seqKnown[dir] = (last+1)&7, true
	meta["assembled_information_hex"] = hex.EncodeToString(joined)
	fields["HDLC Segmentation"] = meta
	return fields, reply, nil
}

// A complete normal service header selects the normal operation profile.
func dlmsNormalOperationSegmentPrefix(info []byte) bool {
	if len(info) < 6 || info[0] != 0xe6 || info[2] != 0 || info[4] != 1 {
		return false
	}
	return info[1] == 0xe6 && (info[3] == 0xc1 || info[3] == 0xc3) || info[1] == 0xe7 && (info[3] == 0xc5 || info[3] == 0xc7)
}
func dlmsSelectedOperationSegmentPrefix(info []byte) bool {
	if dlmsNormalOperationSegmentPrefix(info) {
		return true
	}
	if len(info) < 6 || info[0] != 0xe6 || info[2] != 0 || !dlmsOperationListAPDU(info[3:]) {
		return false
	}
	return info[1] == 0xe6 && (info[3] == 0xc1 || info[3] == 0xc3) || info[1] == 0xe7 && (info[3] == 0xc5 || info[3] == 0xc7)
}
func dlmsOperationListPending(p *dlmsPending) bool {
	return p != nil && (p.kind == "SET" && p.choice == 4 || p.kind == "ACTION" && p.choice == 3)
}
func dlmsOperationSegmentResponseChoice(p *dlmsPending) byte {
	if p.kind == "SET" && p.choice == 4 {
		return 5
	}
	return p.choice
}
func (t *dlmsFragments) operationSegmentProfile() string {
	if t.actionDownloadResponseChoice != 0 {
		return dlmsActionDownloadResponseSegmentProfile(t.actionDownloadResponseChoice)
	}
	if t.actionResponseChoice != 0 {
		return dlmsActionResponseSegmentProfile(t.actionResponseChoice)
	}
	if t.actionRequestChoice != 0 {
		return dlmsActionRequestSegmentProfile(t.actionRequestChoice)
	}
	if t.setResponseChoice != 0 {
		return dlmsSetResponseSegmentProfile(t.setResponseChoice)
	}
	if t.setRequestChoice != 0 {
		return dlmsSetRequestSegmentProfile(t.setRequestChoice)
	}
	if t.operationList {
		return "dlms-hdlc-ln-operation-lists-segmented"
	}
	return "dlms-hdlc-ln-operations-segmented"
}
func (t *dlmsFragments) serviceName() string {
	if t.actionDownloadResponseChoice != 0 {
		return "Action With Pblock"
	}
	if t.actionResponseChoice != 0 {
		return "Action With Pblock"
	}
	if t.actionRequestChoice != 0 {
		if t.nextRequest {
			return "Action With Pblock"
		}
		return "Action With First Pblock"
	}
	if t.setResponseChoice != 0 {
		if t.setResponseChoice == 2 {
			return "Set With Data Block"
		}
		return "Set With List And Data Block"
	}
	if t.setRequestChoice != 0 {
		if t.nextRequest {
			return "Set With Data Block"
		}
		if t.setRequestChoice == 2 {
			return "Set With First Data Block"
		}
		return "Set With List And First Data Block"
	}
	if t.operation != "" {
		if t.operationList {
			return t.operation + " With List"
		}
		return t.operation
	}
	return "Get"
}
func (t *dlmsFragments) operationCloseSummary() string {
	if t.actionDownloadResponseChoice != 0 {
		return "DLMS HDLC exchange ended with incomplete segmented Action-download response"
	}
	if t.actionResponseChoice != 0 {
		return "DLMS HDLC exchange ended with incomplete segmented Action-with-pblock response"
	}
	if t.actionRequestChoice != 0 {
		return "DLMS HDLC exchange ended with incomplete segmented Action-with-pblock request"
	}
	if t.setResponseChoice != 0 {
		if t.setResponseChoice == 2 {
			return "DLMS HDLC exchange ended with incomplete segmented Set-with-data-block response"
		}
		return "DLMS HDLC exchange ended with incomplete segmented Set-with-list-and-data-block response"
	}
	if t.setRequestChoice != 0 {
		if t.setRequestChoice == 2 {
			return "DLMS HDLC exchange ended with incomplete segmented Set-with-data-block request"
		}
		return "DLMS HDLC exchange ended with incomplete segmented Set-with-list-and-data-block request"
	}
	direction := "response"
	if t.request {
		direction = "request"
	}
	name := t.operation
	if t.operationList {
		name += "-with-list"
	}
	return "DLMS HDLC exchange ended with incomplete segmented " + name + " " + direction
}

// Snapshot only the selected observation, not successful semantic validation.
// I-frame continuations inherit the already observed service even if refused;
// RR and other controls retain their literal link profile.
func (f *binFlow) dlmsOperationSegmentObservation(w []byte) string {
	if len(w) < 8 || w[0] != 0x7e {
		return ""
	}
	at := 3
	end := len(w) - 3
	if _, _, err := dlmsAddress(w[:end], &at); err != nil {
		return ""
	}
	if _, _, err := dlmsAddress(w[:end], &at); err != nil || at >= end || w[at]&1 != 0 {
		return ""
	}
	if f.dlms != nil && f.dlms.fragments != nil && f.dlms.fragments.operation != "" {
		return f.dlms.fragments.operationSegmentProfile()
	}
	p := dlmsAPDU(w)
	if w[1]&8 == 0 || len(p) < 3 || !dlmsOperationAPDU(p) {
		return ""
	}
	if dlmsOperationListAPDU(p) {
		return "dlms-hdlc-ln-operation-lists-segmented"
	}
	if p[1] == 1 {
		return "dlms-hdlc-ln-operations-segmented"
	}
	return ""
}
