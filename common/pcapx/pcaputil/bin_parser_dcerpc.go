package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// binDCERPC is the M0 session state for connection-oriented DCE/RPC v5.
// Stub decode stays separate from generic Bind/Request/Call ID session
// state. Ports including 135 are never consulted.
type binDCERPC struct {
	client               int
	ctx                  map[uint16]string
	uuid                 map[uint16][]byte
	pending              map[uint32]string
	pendingDir           map[uint32]int
	pendingContext       map[uint32]uint16
	proposals            map[uint32][]dcerpcProposal
	ctxVersion           map[uint16]uint32
	frags                map[dcerpcFragmentKey]dcerpcFragment
	maxMessageBytes      int
	maxBufferedBytes     int
	reserveSessionMemory func(int64) error
}

type dcerpcFragmentKey struct {
	direction int
	callID    uint32
}

type dcerpcFragment struct {
	ptype      byte
	firstFlags byte
	body       []byte
}

type dcerpcProposal struct {
	id              uint16
	name            string
	abstractSyntax  []byte
	abstractMajor   uint16
	abstractMinor   uint16
	transferSyntax  []byte
	transferVersion uint32
}

type dcerpcBindResult struct {
	result          uint16
	reason          uint16
	transferSyntax  []byte
	transferVersion uint32
}

const (
	dcerpcFirst  = 0x01
	dcerpcLast   = 0x02
	dcerpcObject = 0x80
)

var (
	dcerpcEPM    = []byte{0x08, 0x83, 0xaf, 0xe1, 0x1f, 0x5d, 0xc9, 0x11, 0x91, 0xa4, 0x08, 0x00, 0x2b, 0x14, 0xa0, 0xfa}
	dcerpcSRVSVC = []byte{0xc8, 0x4f, 0x32, 0x4b, 0x70, 0x16, 0xd3, 0x01, 0x12, 0x78, 0x5a, 0x47, 0xbf, 0x6e, 0xe1, 0x88}
	dcerpcNDR    = []byte{0x04, 0x5d, 0x88, 0x8a, 0xeb, 0x1c, 0xc9, 0x11, 0x9f, 0xe8, 0x08, 0x00, 0x2b, 0x10, 0x48, 0x60}
)

func probeDCERPC(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 5 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 16 {
		return probeNeed("dcerpc", "5.0", len(w), 16)
	}
	if w[1] != 0 || w[2] > 19 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if !dcerpcLittleEndian(w) {
		return ProbeResult{Verdict: ProbeReject}
	}
	switch w[2] {
	case 0, 2, 3, 11, 12, 13, 14, 15:
	default:
		return ProbeResult{Verdict: ProbeReject}
	}
	n := dcerpcFragLen(w)
	if n < 16 || n > 1<<20 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("dcerpc", "5.0", 92)
}

func dcerpcFragLen(w []byte) int {
	if len(w) < 10 {
		return 0
	}
	if w[4]&0x10 != 0 {
		return int(binary.LittleEndian.Uint16(w[8:10]))
	}
	return int(binary.BigEndian.Uint16(w[8:10]))
}

func dcerpcLittleEndian(w []byte) bool {
	return len(w) >= 8 && w[4] == 0x10 && w[5] == 0 && w[6] == 0 && w[7] == 0
}

func (f *binFlow) frameDCERPC(w []byte) (int, *binSpec, error) {
	d := f.dcerpc
	if d == nil {
		return 0, nil, sessionContext("DCE/RPC session was not observed")
	}
	d.maxMessageBytes = f.a.config.MaxMessageBytes
	d.maxBufferedBytes = f.a.config.MaxBufferedBytes
	d.reserveSessionMemory = f.reserveSession
	if err := d.reserveMemory(d.retainedBytes()); err != nil {
		return 0, nil, err
	}
	if len(w) < 16 {
		if len(w) > 0 && w[0] == 5 {
			return 0, nil, nil
		}
		return 0, nil, fmt.Errorf("dcerpc: truncated header")
	}
	if w[0] != 5 {
		return 0, nil, fmt.Errorf("dcerpc: RPC vers must be 5")
	}
	if w[1] != 0 {
		return 0, nil, protocolError(ErrUnsupportedVersion, "DCE/RPC connection-oriented session requires vers minor 0")
	}
	if !dcerpcLittleEndian(w) {
		return 0, nil, protocolError(ErrUnsupportedVersion, "DCE/RPC data representation is not supported")
	}
	if w[2] > 19 {
		return 0, nil, fmt.Errorf("dcerpc: unknown ptype %d", w[2])
	}
	n := dcerpcFragLen(w)
	if n < 16 {
		return 0, nil, fmt.Errorf("dcerpc: frag length smaller than header")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("dcerpc", "DCERPC"), nil
}

func (d *binDCERPC) consume(raw []byte, max int) (map[string]any, error) {
	if len(raw) < 3 {
		return nil, fmt.Errorf("dcerpc: truncated header")
	}
	dir := d.client
	if raw[2] == 2 || raw[2] == 3 || raw[2] == 12 || raw[2] == 13 || raw[2] == 15 {
		dir = 1 - dir
	}
	return d.consumeFrom(dir, raw, max)
}

func (d *binDCERPC) consumeFrom(dir int, raw []byte, max int) (map[string]any, error) {
	if len(raw) < 16 {
		return nil, fmt.Errorf("dcerpc: truncated header")
	}
	n := dcerpcFragLen(raw)
	if n != len(raw) {
		return nil, fmt.Errorf("dcerpc: frag length disagrees with framed message")
	}
	if !dcerpcLittleEndian(raw) {
		return nil, protocolError(ErrUnsupportedVersion, "DCE/RPC data representation is not supported")
	}
	ptype := raw[2]
	flags := raw[3]
	call := binary.LittleEndian.Uint32(raw[12:16])
	body := raw[16:]
	authLength := int(binary.LittleEndian.Uint16(raw[10:12]))
	info := map[string]any{
		"PType":         ptype,
		"Packet Name":   dcerpcPTypeName(ptype),
		"PFC Flags":     flags,
		"Call ID":       call,
		"Context Level": "observed",
		"First Frag":    flags&dcerpcFirst != 0,
		"Last Frag":     flags&dcerpcLast != 0,
	}
	if authLength > 0 {
		// A security trailer contains an 8-byte verifier header before the
		// auth_value. We do not decode authenticated stub data, so reject the
		// whole PDU as opaque rather than counting verifier bytes as stub.
		if len(body) < 8 || authLength > len(body)-8 {
			return nil, fmt.Errorf("dcerpc: authentication verifier length exceeds fragment payload")
		}
		d.discardFragment(fragmentKeyFor(dir, call))
		if err := d.reserveMemory(d.retainedBytes()); err != nil {
			return nil, err
		}
		return nil, protocolError(ErrUnsupportedFeature, "DCE/RPC authentication verifier is not supported")
	}
	fragmentKey := dcerpcFragmentKey{direction: dir, callID: call}
	parseFlags := flags
	if flags&dcerpcFirst != 0 {
		if _, exists := d.frags[fragmentKey]; exists {
			d.discardFragment(fragmentKey)
			_ = d.reserveMemory(d.retainedBytes())
			return nil, protocolError(ErrDesynchronized, "DCE/RPC duplicate first fragment for call")
		}
	}
	if flags&dcerpcFirst == 0 {
		previous, exists := d.frags[fragmentKey]
		if !exists {
			return nil, sessionContext("DCE/RPC continuation has no first fragment in this direction")
		}
		if previous.ptype != ptype {
			delete(d.frags, fragmentKey)
			_ = d.reserveMemory(d.retainedBytes())
			return nil, protocolError(ErrDesynchronized, "DCE/RPC fragment PType changed within call")
		}
		parseFlags = previous.firstFlags
		need := len(previous.body) + len(body)
		if need+16 > d.messageLimit() {
			delete(d.frags, fragmentKey)
			_ = d.reserveMemory(d.retainedBytes())
			return nil, protocolError(ErrResourceExceeded, "DCE/RPC reassembled message exceeds budget")
		}
		newCap := cap(previous.body)
		if newCap < need {
			newCap = need
		}
		if err := d.reserveFragmentGrowth(previous.body, newCap, flags&dcerpcLast != 0, true); err != nil {
			delete(d.frags, fragmentKey)
			_ = d.reserveMemory(d.retainedBytes())
			return nil, err
		}
		var assembled []byte
		if newCap == cap(previous.body) {
			assembled = append(previous.body, body...)
		} else {
			assembled = make([]byte, need, newCap)
			copy(assembled, previous.body)
			copy(assembled[len(previous.body):], body)
		}
		info["Fragment"] = true
		if flags&dcerpcLast == 0 {
			d.frags[fragmentKey] = dcerpcFragment{ptype: previous.ptype, firstFlags: previous.firstFlags, body: assembled}
			info["Outstanding"] = true
			return info, nil
		}
		body = assembled
		delete(d.frags, fragmentKey)
		if err := d.reserveMemory(d.retainedBytes()); err != nil {
			return nil, err
		}
		info["Reassembled"] = true
	} else if flags&dcerpcLast == 0 {
		if len(body)+16 > d.messageLimit() {
			return nil, protocolError(ErrResourceExceeded, "DCE/RPC fragment exceeds message budget")
		}
		if d.stateElements() >= sessionCollectionLimit(max) {
			return nil, protocolError(ErrResourceExceeded, "DCE/RPC retained fragment count exceeds budget")
		}
		if err := d.reserveFragmentGrowth(nil, len(body), false, false); err != nil {
			return nil, err
		}
		fragment := make([]byte, len(body), len(body))
		copy(fragment, body)
		d.frags[fragmentKey] = dcerpcFragment{ptype: ptype, firstFlags: flags, body: fragment}
		info["Fragment"] = true
		info["Outstanding"] = true
		if ptype == 0 && len(body) >= 8 {
			info["Context ID"] = binary.LittleEndian.Uint16(body[4:6])
			info["OpNum"] = binary.LittleEndian.Uint16(body[6:8])
		}
		return info, nil
	}
	switch ptype {
	case 11, 14:
		if _, exists := d.pending[call]; exists {
			return nil, fmt.Errorf("dcerpc: duplicate outstanding call id %d", call)
		}
		proposals, err := d.parseBind(body, info, max)
		if err != nil {
			return nil, err
		}
		if err := d.stageProposals(call, dcerpcPTypeName(ptype), dir, proposals, max); err != nil {
			return nil, err
		}
		info["Outstanding"] = true
	case 12:
		if err := d.consumeBindAck(call, "Bind", "BindAck", dir, body, info, max); err != nil {
			return nil, err
		}
		if len(body) >= 8 {
			info["Max Xmit Frag"] = binary.LittleEndian.Uint16(body[0:2])
			info["Assoc Group"] = binary.LittleEndian.Uint32(body[4:8])
		}
	case 13:
		if err := d.consumeBindNak(call, dir, body, info, max); err != nil {
			return nil, err
		}
		if len(body) >= 2 {
			info["Reject Reason"] = binary.LittleEndian.Uint16(body[:2])
		}
	case 0:
		if err := d.parseRequest(body, parseFlags, info); err != nil {
			return nil, err
		}
		contextID := info["Context ID"].(uint16)
		if err := d.push(call, fmt.Sprint(info["Packet Name"]), max, dir, contextID); err != nil {
			return nil, err
		}
		info["Outstanding"] = true
	case 2:
		if len(body) < 8 {
			return nil, fmt.Errorf("dcerpc: truncated response header")
		}
		contextID := binary.LittleEndian.Uint16(body[4:6])
		info["Context ID"] = contextID
		d.matchContext(call, "Response", info, dir, contextID)
	case 3:
		if len(body) >= 16 {
			contextID := binary.LittleEndian.Uint16(body[4:6])
			info["Context ID"] = contextID
			info["Status"] = binary.LittleEndian.Uint32(body[8:12])
			d.matchContext(call, "Fault", info, dir, contextID)
		} else {
			return nil, fmt.Errorf("dcerpc: truncated fault header")
		}
	case 15:
		if err := d.consumeBindAck(call, "AlterContext", "AlterContextResp", dir, body, info, max); err != nil {
			return nil, err
		}
	}
	return info, nil
}

func (d *binDCERPC) parseBind(body []byte, info map[string]any, max int) ([]dcerpcProposal, error) {
	if len(body) < 12 {
		return nil, fmt.Errorf("dcerpc: truncated bind")
	}
	n := int(body[8])
	if n > 20 {
		return nil, fmt.Errorf("dcerpc: too many presentation contexts")
	}
	if n > (len(body)-12)/44 {
		return nil, fmt.Errorf("dcerpc: truncated presentation context list")
	}
	max = sessionCollectionLimit(max)
	if d.stateElements()+n+1 > max {
		return nil, protocolError(ErrResourceExceeded, "DCE/RPC session element count exceeds budget")
	}
	proposals := make([]dcerpcProposal, 0, n)
	names := make([]string, 0, n)
	newIDs := make(map[uint16]struct{}, n)
	for i, off := 0, 12; i < n; i, off = i+1, off+44 {
		id := binary.LittleEndian.Uint16(body[off : off+2])
		if _, duplicate := newIDs[id]; duplicate {
			return nil, fmt.Errorf("dcerpc: duplicate presentation context id %d", id)
		}
		if _, active := d.ctx[id]; active || d.hasProposedContext(id) {
			return nil, fmt.Errorf("dcerpc: presentation context id %d is already in use", id)
		}
		if body[off+2] != 1 {
			return nil, protocolError(ErrUnsupportedFeature, "DCE/RPC presentation context must offer exactly one transfer syntax")
		}
		newIDs[id] = struct{}{}
		abstractSyntax := append([]byte(nil), body[off+4:off+20]...)
		abstractMajor := binary.LittleEndian.Uint16(body[off+20 : off+22])
		abstractMinor := binary.LittleEndian.Uint16(body[off+22 : off+24])
		name := dcerpcIfaceName(abstractSyntax, abstractMajor, abstractMinor)
		transferSyntax := append([]byte(nil), body[off+24:off+40]...)
		transferVersion := binary.LittleEndian.Uint32(body[off+40 : off+44])
		proposals = append(proposals, dcerpcProposal{
			id:              id,
			name:            name,
			abstractSyntax:  abstractSyntax,
			abstractMajor:   abstractMajor,
			abstractMinor:   abstractMinor,
			transferSyntax:  transferSyntax,
			transferVersion: transferVersion,
		})
		names = append(names, name)
		info["Context ID"] = id
		info["Interface"] = name
		info["Abstract Syntax"] = hex.EncodeToString(abstractSyntax)
		info["Context State"] = "proposed"
		info["Interface Version Major"] = abstractMajor
		info["Interface Version Minor"] = abstractMinor
		if name == "unknown" {
			info["Interface Alias State"] = "UUID or abstract syntax version is outside the supported alias profile"
		}
	}
	if len(names) > 1 {
		info["Interfaces"] = names
	}
	return proposals, nil
}

func (d *binDCERPC) stageProposals(call uint32, name string, dir int, proposals []dcerpcProposal, max int) error {
	max = sessionCollectionLimit(max)
	if _, exists := d.pending[call]; exists {
		return fmt.Errorf("dcerpc: duplicate outstanding call id %d", call)
	}
	if d.stateElements()+len(proposals)+1 > max {
		return protocolError(ErrResourceExceeded, "DCE/RPC session element count exceeds budget")
	}
	extra := int64(32 + 64 + len(name)) // proposal map slot and pending request.
	for _, proposal := range proposals {
		extra += int64(64 + len(proposal.name) + cap(proposal.abstractSyntax) + cap(proposal.transferSyntax))
	}
	if err := d.reserveMemory(d.retainedBytes() + extra); err != nil {
		return err
	}
	d.pending[call] = name
	if d.pendingDir == nil {
		d.pendingDir = make(map[uint32]int)
	}
	d.pendingDir[call] = dir
	if d.proposals == nil {
		d.proposals = make(map[uint32][]dcerpcProposal)
	}
	d.proposals[call] = proposals
	return nil
}

func (d *binDCERPC) hasProposedContext(id uint16) bool {
	for _, proposals := range d.proposals {
		for _, proposal := range proposals {
			if proposal.id == id {
				return true
			}
		}
	}
	return false
}

func (d *binDCERPC) markUnmatched(name string, info map[string]any, reason string) {
	info["Unmatched"] = true
	info["Association Status"] = reason
	info["Context Level"] = "partial"
	info["Packet Name"] = name
}

func (d *binDCERPC) consumeBindAck(call uint32, expectedRequest, responseName string, dir int, body []byte, info map[string]any, max int) error {
	results, err := parseDCERPCBindResults(body, max)
	if err != nil {
		return err
	}
	want, pending := d.pending[call]
	if !pending {
		d.markUnmatched(responseName, info, "missing-request")
		return nil
	}
	if want != expectedRequest {
		d.markUnmatched(responseName, info, "request-type-mismatch")
		return nil
	}
	if d.pendingDir[call] == dir {
		d.markUnmatched(responseName, info, "direction-mismatch")
		return nil
	}
	proposals, hasProposal := d.proposals[call]
	if !hasProposal {
		return fmt.Errorf("dcerpc: bind acknowledgement has no matching proposal state")
	}
	if len(results) != len(proposals) {
		return fmt.Errorf("dcerpc: bind acknowledgement result count does not match proposal count")
	}
	var accepted []dcerpcProposal
	var rejected []uint16
	for i, result := range results {
		proposal := proposals[i]
		switch result.result {
		case 0:
			if result.reason != 0 {
				return fmt.Errorf("dcerpc: accepted bind result has a rejection reason")
			}
			if !bytes.Equal(result.transferSyntax, proposal.transferSyntax) || result.transferVersion != proposal.transferVersion {
				return fmt.Errorf("dcerpc: bind acknowledgement selected an unoffered transfer syntax")
			}
			accepted = append(accepted, proposal)
		case 1, 2:
			rejected = append(rejected, proposal.id)
		default:
			return fmt.Errorf("dcerpc: invalid bind result %d", result.result)
		}
	}
	// Reserve the post-ack state before matching/removing the pending call so
	// an accepted context can never become visible without its memory charge.
	target := d.retainedBytes() - int64(64+len(want)) - 32
	for _, proposal := range proposals {
		target -= int64(64 + len(proposal.name) + cap(proposal.abstractSyntax) + cap(proposal.transferSyntax))
	}
	for _, proposal := range accepted {
		target += int64(64 + cap(proposal.abstractSyntax))
	}
	if err := d.reserveMemory(target); err != nil {
		return err
	}
	if !d.match(call, responseName, info, dir) {
		return nil
	}
	delete(d.proposals, call)
	for _, proposal := range accepted {
		d.ctx[proposal.id] = proposal.name
		d.uuid[proposal.id] = proposal.abstractSyntax
		if d.ctxVersion == nil {
			d.ctxVersion = make(map[uint16]uint32)
		}
		d.ctxVersion[proposal.id] = uint32(proposal.abstractMajor)<<16 | uint32(proposal.abstractMinor)
	}
	if len(accepted) > 0 {
		ids := make([]uint16, 0, len(accepted))
		for _, proposal := range accepted {
			ids = append(ids, proposal.id)
		}
		info["Accepted Context IDs"] = ids
	}
	if len(rejected) > 0 {
		info["Rejected Context IDs"] = rejected
	}
	return nil
}

func (d *binDCERPC) consumeBindNak(call uint32, dir int, body []byte, info map[string]any, max int) error {
	if len(body) < 3 {
		return fmt.Errorf("dcerpc: truncated bind nak")
	}
	count := int(body[2])
	if count > 20 {
		return fmt.Errorf("dcerpc: too many bind nak protocol versions")
	}
	if count > sessionCollectionLimit(max) {
		return protocolError(ErrResourceExceeded, "DCE/RPC bind nak version count exceeds budget")
	}
	if len(body) != 3+count*2 {
		return fmt.Errorf("dcerpc: invalid bind nak protocol-version list")
	}
	info["Version Count"] = count
	if count > 0 {
		versions := make([]string, 0, count)
		for i := 0; i < count; i++ {
			off := 3 + 2*i
			versions = append(versions, fmt.Sprintf("%d.%d", body[off], body[off+1]))
		}
		info["Protocol Versions"] = versions
	}
	want, pending := d.pending[call]
	if !pending {
		d.markUnmatched("BindNak", info, "missing-request")
		return nil
	}
	if want != "Bind" && want != "AlterContext" {
		d.markUnmatched("BindNak", info, "request-type-mismatch")
		return nil
	}
	if d.pendingDir[call] == dir {
		d.markUnmatched("BindNak", info, "direction-mismatch")
		return nil
	}
	if d.match(call, "BindNak", info, dir) {
		delete(d.proposals, call)
		info["Context Rejected"] = true
	}
	return nil
}

func parseDCERPCBindResults(body []byte, max int) ([]dcerpcBindResult, error) {
	if len(body) < 12 {
		return nil, fmt.Errorf("dcerpc: truncated bind acknowledgement")
	}
	secondaryAddressLen := int(binary.LittleEndian.Uint16(body[8:10]))
	addressEnd := 10 + secondaryAddressLen
	if addressEnd > len(body) {
		return nil, fmt.Errorf("dcerpc: truncated bind acknowledgement secondary address")
	}
	resultOffset := (addressEnd + 3) &^ 3
	if resultOffset+4 > len(body) {
		return nil, fmt.Errorf("dcerpc: truncated bind acknowledgement result list")
	}
	if body[resultOffset+1] != 0 || body[resultOffset+2] != 0 || body[resultOffset+3] != 0 {
		return nil, fmt.Errorf("dcerpc: nonzero bind acknowledgement result-list reserved field")
	}
	count := int(body[resultOffset])
	if count > sessionCollectionLimit(max) {
		return nil, protocolError(ErrResourceExceeded, "DCE/RPC bind result count exceeds budget")
	}
	if len(body) != resultOffset+4+count*24 {
		return nil, fmt.Errorf("dcerpc: invalid bind acknowledgement result list length")
	}
	results := make([]dcerpcBindResult, 0, count)
	for off := resultOffset + 4; off < len(body); off += 24 {
		results = append(results, dcerpcBindResult{
			result:          binary.LittleEndian.Uint16(body[off : off+2]),
			reason:          binary.LittleEndian.Uint16(body[off+2 : off+4]),
			transferSyntax:  append([]byte(nil), body[off+4:off+20]...),
			transferVersion: binary.LittleEndian.Uint32(body[off+20 : off+24]),
		})
	}
	return results, nil
}

func (d *binDCERPC) parseRequest(body []byte, flags byte, info map[string]any) error {
	need := 8
	if flags&dcerpcObject != 0 {
		need += 16
	}
	if len(body) < need {
		return fmt.Errorf("dcerpc: truncated request")
	}
	ctx := binary.LittleEndian.Uint16(body[4:6])
	op := binary.LittleEndian.Uint16(body[6:8])
	info["Context ID"] = ctx
	info["OpNum"] = op
	if name, ok := d.ctx[ctx]; ok {
		info["Interface"] = name
		info["Packet Name"] = dcerpcOpName(name, op)
		if u := d.uuid[ctx]; len(u) != 0 {
			info["Abstract Syntax"] = hex.EncodeToString(u)
		}
		version := d.ctxVersion[ctx]
		major, minor := uint16(version>>16), uint16(version)
		info["Interface Version Major"] = major
		info["Interface Version Minor"] = minor
		if name == "unknown" {
			info["Interface Alias State"] = "UUID or abstract syntax version is outside the supported alias profile"
		}
	} else {
		info["Context Level"] = "partial"
	}
	stub := body[need:]
	info["Stub Bytes"] = len(stub)
	return nil
}

func (d *binDCERPC) push(call uint32, name string, max, dir int, contextID uint16) error {
	max = sessionCollectionLimit(max)
	if _, exists := d.pending[call]; exists {
		return fmt.Errorf("dcerpc: duplicate outstanding call id %d", call)
	}
	if d.stateElements() >= max {
		return protocolError(ErrResourceExceeded, "DCE/RPC session element count exceeds budget")
	}
	if err := d.reserveMemory(d.retainedBytes() + int64(80+len(name))); err != nil {
		return err
	}
	d.pending[call] = name
	if d.pendingDir == nil {
		d.pendingDir = make(map[uint32]int)
	}
	d.pendingDir[call] = dir
	if d.pendingContext == nil {
		d.pendingContext = make(map[uint32]uint16)
	}
	d.pendingContext[call] = contextID
	return nil
}

func (d *binDCERPC) matchContext(call uint32, respName string, info map[string]any, dir int, contextID uint16) bool {
	if _, pending := d.pending[call]; pending {
		if requestContext, hasContext := d.pendingContext[call]; hasContext && requestContext != contextID {
			d.markUnmatched(respName, info, "context-mismatch")
			return false
		}
	}
	return d.match(call, respName, info, dir)
}

func (d *binDCERPC) match(call uint32, respName string, info map[string]any, dir int) bool {
	if want, ok := d.pending[call]; ok {
		if reqDir, known := d.pendingDir[call]; known && reqDir == dir {
			info["Unmatched"] = true
			info["Association Status"] = "direction-mismatch"
			info["Context Level"] = "partial"
			info["Packet Name"] = respName
			return false
		}
		delete(d.pending, call)
		delete(d.pendingDir, call)
		delete(d.pendingContext, call)
		info["Matched Request"] = want
		info["Association Status"] = "matched"
		info["Packet Name"] = respName
		return true
	}
	d.markUnmatched(respName, info, "missing-request")
	return false
}

func (d *binDCERPC) stateElements() int {
	n := len(d.ctx) + len(d.pending) + len(d.frags)
	for _, proposals := range d.proposals {
		n += len(proposals)
	}
	return n
}

// retainedBytes conservatively charges actual retained fragment capacities,
// UUID data, pending-call names, and map/slice bookkeeping.
func (d *binDCERPC) retainedBytes() int64 {
	n := int64(64) + int64(len(d.ctx))*80 + int64(len(d.pending))*64
	for _, uuid := range d.uuid {
		n += int64(cap(uuid))
	}
	for _, name := range d.pending {
		n += int64(len(name))
	}
	n += int64(len(d.pendingContext)) * 16
	for _, fragment := range d.frags {
		n += 48 + int64(cap(fragment.body))
	}
	for _, proposals := range d.proposals {
		n += 32
		for _, proposal := range proposals {
			n += int64(64 + len(proposal.name) + cap(proposal.abstractSyntax) + cap(proposal.transferSyntax))
		}
	}
	return n
}

func (d *binDCERPC) reserveMemory(target int64) error {
	if target < d.retainedBytes() {
		target = d.retainedBytes()
	}
	if d.maxBufferedBytes > 0 && target > int64(d.maxBufferedBytes) {
		return protocolError(ErrResourceExceeded, "DCE/RPC retained state exceeds capture memory budget")
	}
	if d.reserveSessionMemory != nil {
		if err := d.reserveSessionMemory(target); err != nil {
			return err
		}
	}
	return nil
}

func (d *binDCERPC) reserveFragmentGrowth(previous []byte, newCapacity int, final, hasPrevious bool) error {
	if newCapacity+16 > d.messageLimit() {
		return protocolError(ErrResourceExceeded, "DCE/RPC fragment exceeds message budget")
	}
	target := d.retainedBytes()
	if hasPrevious {
		target -= int64(cap(previous))
	} else if !final {
		target += 48
	}
	target += int64(newCapacity)
	return d.reserveMemory(target)
}

func fragmentKeyFor(direction int, call uint32) dcerpcFragmentKey {
	return dcerpcFragmentKey{direction: direction, callID: call}
}

func (d *binDCERPC) discardFragment(key dcerpcFragmentKey) {
	delete(d.frags, key)
}

func (d *binDCERPC) messageLimit() int {
	if d.maxMessageBytes > 0 {
		return d.maxMessageBytes
	}
	return DefaultParserBudget().MaxMessageBytes
}

func dcerpcPTypeName(p byte) string {
	switch p {
	case 0:
		return "Request"
	case 2:
		return "Response"
	case 3:
		return "Fault"
	case 11:
		return "Bind"
	case 12:
		return "BindAck"
	case 13:
		return "BindNak"
	case 14:
		return "AlterContext"
	case 15:
		return "AlterContextResp"
	}
	return fmt.Sprintf("PType %d", p)
}

func dcerpcIfaceName(uuid []byte, major, minor uint16) string {
	switch {
	case bytes.Equal(uuid, dcerpcEPM) && major == 3 && minor == 0:
		return "EPM"
	case bytes.Equal(uuid, dcerpcSRVSVC) && major == 3 && minor == 0:
		return "SRVSVC"
	default:
		return "unknown"
	}
}

func dcerpcOpName(iface string, op uint16) string {
	switch iface {
	case "EPM":
		if op == 3 {
			return "EPM.ept_map"
		}
	case "SRVSVC":
		if op == 15 {
			return "SRVSVC.NetrShareEnum"
		}
	}
	return fmt.Sprintf("%s.op_%d", iface, op)
}
