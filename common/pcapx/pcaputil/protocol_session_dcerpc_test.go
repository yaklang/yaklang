package pcaputil

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func dcerpcHdr(ptype, flags byte, frag uint16, call uint32) []byte {
	h := make([]byte, 16)
	h[0] = 5
	h[2] = ptype
	h[3] = flags
	h[4] = 0x10
	binary.LittleEndian.PutUint16(h[8:], frag)
	binary.LittleEndian.PutUint32(h[12:], call)
	return h
}

func dcerpcPDU(ptype, flags byte, call uint32, body []byte) []byte {
	return append(dcerpcHdr(ptype, flags, uint16(16+len(body)), call), body...)
}

func dcerpcBind(uuid []byte, ctx uint16, call uint32) []byte {
	return dcerpcBindType(11, uuid, ctx, call)
}

func dcerpcBindVersion(uuid []byte, ctx uint16, call uint32, major, minor uint16) []byte {
	pdu := dcerpcBind(uuid, ctx, call)
	off := 16 + 12 + 20
	binary.LittleEndian.PutUint16(pdu[off:off+2], major)
	binary.LittleEndian.PutUint16(pdu[off+2:off+4], minor)
	return pdu
}

type dcerpcBindContext struct {
	uuid []byte
	id   uint16
}

func dcerpcBindType(ptype byte, uuid []byte, ctx uint16, call uint32) []byte {
	return dcerpcBindContexts(ptype, call, dcerpcBindContext{uuid: uuid, id: ctx})
}

func dcerpcBindContexts(ptype byte, call uint32, contexts ...dcerpcBindContext) []byte {
	body := make([]byte, 12+44*len(contexts))
	binary.LittleEndian.PutUint16(body[0:], 5840)
	binary.LittleEndian.PutUint16(body[2:], 5840)
	body[8] = byte(len(contexts))
	for i, context := range contexts {
		off := 12 + 44*i
		binary.LittleEndian.PutUint16(body[off:], context.id)
		body[off+2] = 1
		copy(body[off+4:off+20], context.uuid)
		binary.LittleEndian.PutUint32(body[off+20:], 3)
		copy(body[off+24:off+40], dcerpcNDR)
		binary.LittleEndian.PutUint32(body[off+40:], 2)
	}
	return dcerpcPDU(ptype, 0x03, call, body)
}

func dcerpcBindAck(call uint32) []byte {
	return dcerpcBindAckResult(call, 0, dcerpcNDR, 2)
}

// dcerpcM1LegacyBindAck preserves the original M1 smoke fixture's historical
// truncated BindAck bytes. The complete BindAck layout is exercised by the
// generated T21 capture and the focused parser tests below.
func dcerpcM1LegacyBindAck(call uint32) []byte {
	body := make([]byte, 10)
	binary.LittleEndian.PutUint16(body[0:], 5840)
	binary.LittleEndian.PutUint16(body[2:], 5840)
	return dcerpcPDU(12, 0x03, call, body)
}

func dcerpcBindAckResult(call uint32, result uint16, syntax []byte, version uint32) []byte {
	return dcerpcBindAckPType(12, call, result, syntax, version)
}

func dcerpcBindAckPType(ptype byte, call uint32, result uint16, syntax []byte, version uint32) []byte {
	return dcerpcBindAckResults(ptype, call, dcerpcBindAckEntry{result: result, syntax: syntax, version: version})
}

type dcerpcBindAckEntry struct {
	result  uint16
	syntax  []byte
	version uint32
}

func dcerpcBindAckResults(ptype byte, call uint32, results ...dcerpcBindAckEntry) []byte {
	// BindAck fixed fields and a complete, aligned result list with one result.
	body := make([]byte, 12+4+24*len(results))
	binary.LittleEndian.PutUint16(body[0:], 5840)
	binary.LittleEndian.PutUint16(body[2:], 5840)
	body[12] = byte(len(results))
	for i, result := range results {
		off := 16 + 24*i
		binary.LittleEndian.PutUint16(body[off:], result.result)
		copy(body[off+4:off+20], result.syntax)
		binary.LittleEndian.PutUint32(body[off+20:], result.version)
	}
	return dcerpcPDU(ptype, 0x03, call, body)
}

func dcerpcBindNak(call uint32) []byte {
	// Reject reason followed by an empty supported-protocol-version list.
	return dcerpcPDU(13, 0x03, call, []byte{2, 0, 0})
}

func dcerpcRequest(call uint32, ctx, op uint16, stub []byte) []byte {
	body := make([]byte, 8+len(stub))
	binary.LittleEndian.PutUint16(body[4:], ctx)
	binary.LittleEndian.PutUint16(body[6:], op)
	copy(body[8:], stub)
	return dcerpcPDU(0, 0x03, call, body)
}

func dcerpcResponse(call uint32, ctx uint16, stub []byte) []byte {
	body := make([]byte, 8+len(stub))
	binary.LittleEndian.PutUint16(body[4:], ctx)
	copy(body[8:], stub)
	return dcerpcPDU(2, 0x03, call, body)
}

func dcerpcFault(call uint32, status uint32) []byte {
	body := make([]byte, 16)
	binary.LittleEndian.PutUint32(body[8:], status)
	return dcerpcPDU(3, 0x03, call, body)
}

func TestProtocolSessionDCERPCFaultRequiresCompleteFixedBody(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)).Err)
	require.Nil(t, s.Feed(1, ts, dcerpcBindAck(1)).Err)
	require.Nil(t, s.Feed(0, ts, dcerpcRequest(5, 0, 3, nil)).Err)

	truncatedFault := dcerpcPDU(3, 0x03, 5, dcerpcFault(5, 0x00000005)[16:28])
	d := s.(*captureSession).f.dcerpc
	_, err = d.consumeFrom(1, truncatedFault, DefaultParserBudget().MaxCollectionElements)
	require.Error(t, err)
	require.Equal(t, "EPM.ept_map", d.pending[5], "truncated fault must preserve the outstanding request")

	info, err := d.consumeFrom(1, dcerpcFault(5, 0x00000005), DefaultParserBudget().MaxCollectionElements)
	require.NoError(t, err)
	require.Equal(t, "Fault", info["Packet Name"])
	require.Equal(t, uint32(5), info["Status"])
	require.Equal(t, "matched", info["Association Status"])
	require.Equal(t, "EPM.ept_map", info["Matched Request"])
}

func TestProtocolSessionDCERPCBindEPMAndSRVSVC(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	bind := dcerpcBind(dcerpcEPM, 0, 1)
	p := s.Probe(bind)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "dcerpc", p.Protocol)
	require.Equal(t, "5.0", p.Version)

	r := s.Feed(0, ts, bind)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Bind", r.Events[0].Session["Packet Name"])
	require.Equal(t, "EPM", r.Events[0].Session["Interface"])
	require.Equal(t, uint32(1), r.Events[0].Session["Call ID"])
	require.Equal(t, true, r.Events[0].Session["Outstanding"])

	// A syntactically valid BindAck from the Bind direction cannot consume the
	// outstanding call or activate the proposed context.
	r = s.Feed(0, ts, dcerpcBindAck(1))
	require.Nil(t, r.Err)
	require.Equal(t, "direction-mismatch", r.Events[0].Session["Association Status"])
	require.Equal(t, true, r.Events[0].Session["Unmatched"])

	// A proposed context is not active until the peer accepts it.
	r = s.Feed(0, ts, dcerpcRequest(90, 0, 3, nil))
	require.Nil(t, r.Err)
	require.Equal(t, "partial", r.Events[0].Session["Context Level"])
	require.NotContains(t, r.Events[0].Session, "Interface")
	require.Nil(t, s.Feed(1, ts, dcerpcResponse(90, 0, nil)).Err)

	r = s.Feed(1, ts, dcerpcBindAck(1))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "BindAck", r.Events[0].Session["Packet Name"])
	require.Equal(t, "Bind", r.Events[0].Session["Matched Request"])
	require.Equal(t, []uint16{0}, r.Events[0].Session["Accepted Context IDs"])

	r = s.Feed(0, ts, dcerpcRequest(2, 0, 3, []byte{1, 2, 3, 4}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "EPM.ept_map", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint16(3), r.Events[0].Session["OpNum"])
	require.Equal(t, "EPM", r.Events[0].Session["Interface"])

	r = s.Feed(1, ts, dcerpcResponse(2, 0, []byte{9, 9}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Response", r.Events[0].Session["Packet Name"])
	require.Equal(t, "EPM.ept_map", r.Events[0].Session["Matched Request"])

	r = s.Feed(0, ts, dcerpcBind(dcerpcSRVSVC, 1, 3))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SRVSVC", r.Events[0].Session["Interface"])
	require.Nil(t, s.Feed(1, ts, dcerpcBindAck(3)).Err)

	r = s.Feed(0, ts, dcerpcRequest(4, 1, 15, []byte{0, 0, 0, 0}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SRVSVC.NetrShareEnum", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint16(15), r.Events[0].Session["OpNum"])
	r = s.Feed(1, ts, dcerpcResponse(4, 1, nil))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SRVSVC.NetrShareEnum", r.Events[0].Session["Matched Request"])

	r = s.Feed(1, ts, dcerpcFault(5, 0x00000005))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Fault", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint32(5), r.Events[0].Session["Status"])
	require.Equal(t, true, r.Events[0].Session["Unmatched"])

	first := dcerpcPDU(0, dcerpcFirst, 6, append([]byte{0, 0, 0, 0, 1, 0, 15, 0}, []byte{0xaa, 0xbb}...))
	last := dcerpcPDU(0, dcerpcLast, 6, []byte{0xcc, 0xdd})
	r = s.Feed(0, ts, first)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Fragment"])
	r = s.Feed(0, ts, last)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Reassembled"])
	require.Equal(t, "SRVSVC.NetrShareEnum", r.Events[0].Session["Packet Name"])
	require.Equal(t, 4, r.Events[0].Session["Stub Bytes"])

	cut := s.Feed(0, ts, bind[:8])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
}

func TestProtocolSessionDCERPCInterfaceAliasRequiresKnownVersion(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	ts := time.Unix(1, 0)
	bind := dcerpcBindVersion(dcerpcEPM, 0, 61, 4, 0)
	r := s.Feed(0, ts, bind)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "unknown", r.Events[0].Session["Interface"])
	require.Equal(t, uint16(4), r.Events[0].Session["Interface Version Major"])
	require.Equal(t, uint16(0), r.Events[0].Session["Interface Version Minor"])
	require.Contains(t, r.Events[0].Session["Interface Alias State"], "outside the supported alias profile")
	r = s.Feed(1, ts, dcerpcBindAck(61))
	require.Nil(t, r.Err, "%v", r.Err)
	r = s.Feed(0, ts, dcerpcRequest(62, 0, 3, nil))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "unknown.op_3", r.Events[0].Session["Packet Name"])
	require.Equal(t, "unknown", r.Events[0].Session["Interface"])
	require.Equal(t, uint16(4), r.Events[0].Session["Interface Version Major"])
}

func TestProtocolSessionDCERPCFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Equal(t, ProbeReject, s.Probe([]byte{4, 0, 0, 0, 0, 0, 0, 0, 16, 0, 0, 0, 1, 0, 0, 0}).Verdict)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{5, 0, 11}).Verdict)
	require.Equal(t, ProbeReject, s.Probe(dcerpcHdr(30, 3, 16, 1)).Verdict)

	r := s.Feed(0, ts, []byte{4, 0, 0, 0})
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)).Err)
	short := dcerpcHdr(0, 3, 10, 2)
	r = s2.Feed(0, ts, short)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s3.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)[:10])
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)
}

func TestProtocolSessionDCERPCBindAcceptance(t *testing.T) {
	ts := time.Unix(1, 0)
	// A complete result list may accept one proposed interface and reject
	// another without making the rejected context usable.
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, ts, dcerpcBindContexts(11, 5,
		dcerpcBindContext{uuid: dcerpcEPM, id: 0},
		dcerpcBindContext{uuid: dcerpcSRVSVC, id: 1},
	)).Err)
	r := s.Feed(1, ts, dcerpcBindAckResults(12, 5,
		dcerpcBindAckEntry{result: 0, syntax: dcerpcNDR, version: 2},
		dcerpcBindAckEntry{result: 2},
	))
	require.Nil(t, r.Err)
	require.Equal(t, []uint16{0}, r.Events[0].Session["Accepted Context IDs"])
	require.Equal(t, []uint16{1}, r.Events[0].Session["Rejected Context IDs"])
	r = s.Feed(0, ts, dcerpcRequest(6, 0, 3, nil))
	require.Nil(t, r.Err)
	require.Equal(t, "EPM", r.Events[0].Session["Interface"])
	require.Nil(t, s.Feed(1, ts, dcerpcResponse(6, 0, nil)).Err)
	r = s.Feed(0, ts, dcerpcRequest(7, 1, 15, nil))
	require.Nil(t, r.Err)
	require.Equal(t, "partial", r.Events[0].Session["Context Level"])
	require.NotContains(t, r.Events[0].Session, "Interface")
	require.Nil(t, s.Feed(1, ts, dcerpcResponse(7, 1, nil)).Err)

	// Per-context rejection consumes the proposal, leaves that context
	// unusable, and permits the same ID to be proposed again.
	s, err = NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)).Err)
	r = s.Feed(1, ts, dcerpcBindAckResult(1, 2, nil, 0))
	require.Nil(t, r.Err)
	require.Equal(t, "Bind", r.Events[0].Session["Matched Request"])
	require.Equal(t, []uint16{0}, r.Events[0].Session["Rejected Context IDs"])
	r = s.Feed(0, ts, dcerpcRequest(2, 0, 3, nil))
	require.Nil(t, r.Err)
	require.Equal(t, "partial", r.Events[0].Session["Context Level"])
	require.NotContains(t, r.Events[0].Session, "Interface")
	require.Nil(t, s.Feed(1, ts, dcerpcResponse(2, 0, nil)).Err)
	require.Nil(t, s.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 3)).Err)
	require.Nil(t, s.Feed(1, ts, dcerpcBindAck(3)).Err)
	r = s.Feed(0, ts, dcerpcRequest(4, 0, 3, nil))
	require.Nil(t, r.Err)
	require.Equal(t, "EPM", r.Events[0].Session["Interface"])
	require.Nil(t, s.Feed(1, ts, dcerpcResponse(4, 0, nil)).Err)

	// BindNak removes all proposals and does not activate them.
	s, err = NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, ts, dcerpcBind(dcerpcSRVSVC, 1, 10)).Err)
	r = s.Feed(0, ts, dcerpcBindNak(10))
	require.Nil(t, r.Err)
	require.Equal(t, "direction-mismatch", r.Events[0].Session["Association Status"])
	r = s.Feed(1, ts, dcerpcBindNak(10))
	require.Nil(t, r.Err)
	require.Equal(t, "Bind", r.Events[0].Session["Matched Request"])
	require.Equal(t, true, r.Events[0].Session["Context Rejected"])
	r = s.Feed(0, ts, dcerpcRequest(11, 1, 15, nil))
	require.Nil(t, r.Err)
	require.Equal(t, "partial", r.Events[0].Session["Context Level"])
	require.NotContains(t, r.Events[0].Session, "Interface")

	// A truncated 10-byte BindAck or an incomplete result list cannot consume
	// the pending proposal or make the context usable; a later valid ack can.
	d := &binDCERPC{
		ctx: map[uint16]string{}, uuid: map[uint16][]byte{}, ctxVersion: map[uint16]uint32{},
		pending: map[uint32]string{}, pendingDir: map[uint32]int{}, pendingContext: map[uint32]uint16{},
		proposals: map[uint32][]dcerpcProposal{}, frags: map[dcerpcFragmentKey]dcerpcFragment{},
	}
	proposal, err := d.parseBind(dcerpcBind(dcerpcEPM, 0, 20)[16:], map[string]any{}, 4096)
	require.NoError(t, err)
	require.NoError(t, d.stageProposals(20, "Bind", 0, proposal, 4096))
	shortAck := dcerpcPDU(12, 0x03, 20, make([]byte, 10))
	_, err = d.consumeFrom(1, shortAck, 4096)
	require.Error(t, err)
	info, err := d.consumeFrom(0, dcerpcRequest(21, 0, 3, nil), 4096)
	require.NoError(t, err)
	require.Equal(t, "partial", info["Context Level"])
	badList := dcerpcPDU(12, 0x03, 20, make([]byte, 16))
	badList[28] = 1 // result count says one, but the 24-byte result is absent.
	_, err = d.consumeFrom(1, badList, 4096)
	require.Error(t, err)
	info, err = d.consumeFrom(0, dcerpcRequest(22, 0, 3, nil), 4096)
	require.NoError(t, err)
	require.Equal(t, "partial", info["Context Level"])
	info, err = d.consumeFrom(1, dcerpcBindAck(20), 4096)
	require.NoError(t, err)
	require.Equal(t, "Bind", info["Matched Request"])
	info, err = d.consumeFrom(0, dcerpcRequest(23, 0, 3, nil), 4096)
	require.NoError(t, err)
	require.Equal(t, "EPM", info["Interface"])

	// AlterContext also stages a new interface until its own accepted result.
	s, err = NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, ts, dcerpcBindType(14, dcerpcSRVSVC, 2, 30)).Err)
	r = s.Feed(0, ts, dcerpcRequest(31, 2, 15, nil))
	require.Nil(t, r.Err)
	require.Equal(t, "partial", r.Events[0].Session["Context Level"])
	require.NotContains(t, r.Events[0].Session, "Interface")
	r = s.Feed(1, ts, dcerpcBindAckPType(15, 30, 0, dcerpcNDR, 2))
	require.Nil(t, r.Err)
	require.Equal(t, "AlterContext", r.Events[0].Session["Matched Request"])
	r = s.Feed(0, ts, dcerpcRequest(32, 2, 15, nil))
	require.Nil(t, r.Err)
	require.Equal(t, "SRVSVC", r.Events[0].Session["Interface"])
}

func TestProtocolSessionDCERPCBudgetsDirectionAndBigEndian(t *testing.T) {
	ts := time.Unix(1, 0)

	// A response with the right call ID in the request direction must not
	// consume the outstanding request. The later reverse-direction response
	// remains eligible to match it.
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)).Err)
	require.Nil(t, s.Feed(1, ts, dcerpcBindAck(1)).Err)
	require.Nil(t, s.Feed(0, ts, dcerpcRequest(20, 0, 3, []byte{1})).Err)
	r := s.Feed(0, ts, dcerpcResponse(20, 0, []byte{2}))
	require.Nil(t, r.Err)
	require.Equal(t, "direction-mismatch", r.Events[0].Session["Association Status"])
	require.Equal(t, true, r.Events[0].Session["Unmatched"])
	r = s.Feed(1, ts, dcerpcResponse(20, 0, []byte{3}))
	require.Nil(t, r.Err)
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])
	require.Equal(t, "EPM.ept_map", r.Events[0].Session["Matched Request"])

	// The supported profile is the canonical little-endian data
	// representation. The currently unsupported big-endian representation is
	// rejected instead of being parsed with little-endian body fields.
	be := dcerpcResponse(21, 0, nil)
	be[4], be[5], be[6], be[7] = 0, 0, 0, 0
	binary.BigEndian.PutUint16(be[8:10], uint16(len(be)))
	require.Equal(t, ProbeReject, probeDCERPC(be, len(be)).Verdict)
	r = s.Feed(1, ts, be)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrUnsupportedVersion, r.Err.Kind)

	// Outstanding calls and partial fragments share the configured element
	// budget with presentation contexts.
	budget := DefaultParserBudget()
	budget.MaxCollectionElements = 3
	s, err = NewProtocolSession(budget)
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)).Err)
	require.Nil(t, s.Feed(1, ts, dcerpcBindAck(1)).Err)
	require.Nil(t, s.Feed(0, ts, dcerpcRequest(30, 0, 3, nil)).Err)
	require.Nil(t, s.Feed(0, ts, dcerpcRequest(31, 0, 3, nil)).Err)
	r = s.Feed(0, ts, dcerpcRequest(32, 0, 3, nil))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)

	budget = DefaultParserBudget()
	budget.MaxCollectionElements = 2
	s, err = NewProtocolSession(budget)
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)).Err)
	require.Nil(t, s.Feed(1, ts, dcerpcBindAck(1)).Err)
	require.Nil(t, s.Feed(0, ts, dcerpcRequest(40, 0, 3, nil)).Err)
	fragmentBody := []byte{0, 0, 0, 0, 0, 0, 3, 0, 1}
	r = s.Feed(0, ts, dcerpcPDU(0, dcerpcFirst, 41, fragmentBody))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)

	// Even individually valid fragments cannot reassemble past the message
	// budget. Their retained byte capacities are charged before append.
	budget = DefaultParserBudget()
	budget.MaxMessageBytes = 128
	budget.MaxBufferedBytes = 4096
	s, err = NewProtocolSession(budget)
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)).Err)
	require.Nil(t, s.Feed(1, ts, dcerpcBindAck(1)).Err)
	first := dcerpcPDU(0, dcerpcFirst, 50, append([]byte{0, 0, 0, 0, 0, 0, 3, 0}, make([]byte, 60)...))
	require.Nil(t, s.Feed(0, ts, first).Err)
	last := dcerpcPDU(0, dcerpcLast, 50, make([]byte, 65))
	r = s.Feed(0, ts, last)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)

	// The shared buffered-byte budget also covers retained fragment bytes,
	// independently of the per-message ceiling.
	budget = DefaultParserBudget()
	budget.MaxMessageBytes = 128
	budget.MaxBufferedBytes = 263
	s, err = NewProtocolSession(budget)
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)).Err)
	require.Nil(t, s.Feed(1, ts, dcerpcBindAck(1)).Err)
	fragmentBody = append([]byte{0, 0, 0, 0, 0, 0, 3, 0}, make([]byte, 97)...)
	r = s.Feed(0, ts, dcerpcPDU(0, dcerpcFirst, 60, fragmentBody))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
}

func TestProtocolSessionDCERPCFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, dcerpcBind(dcerpcEPM, 0, 1)},
		{1, dcerpcBindAck(1)},
		{0, dcerpcRequest(2, 0, 3, []byte{1, 2, 3, 4})},
		{1, dcerpcResponse(2, 0, []byte{9, 9})},
		{0, dcerpcBind(dcerpcSRVSVC, 1, 3)},
		{1, dcerpcBindAck(3)},
		{0, dcerpcRequest(4, 1, 15, []byte{0, 0, 0, 0})},
		{1, dcerpcResponse(4, 1, nil)},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		ts := time.Unix(1, 0)
		var names []string
		for _, st := range steps {
			for w := st.wire; len(w) > 0; {
				n := len(w)
				if chunk > 0 {
					n = min(n, chunk)
				}
				r := s.Feed(st.dir, ts, w[:n])
				for _, e := range r.Events {
					if e.Status == "decoded" || e.Status == "deferred" {
						names = append(names, fmt.Sprint(e.Session["Packet Name"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}

func TestProtocolSessionDCERPCGeneratedExchange(t *testing.T) {
	const sample = "testdata/protocol-sessions/dcerpc-v5-bindack-request-response.pcap"
	pcap, err := os.ReadFile(sample)
	require.NoError(t, err)
	events, _, err := binReplay(t, pcap, 1)
	require.NoError(t, err)
	byFrame := make(map[uint64]*ProtocolEvent)
	for _, event := range events {
		if event.Protocol != "dcerpc" {
			continue
		}
		require.Len(t, event.SourceBytes.PacketRefs, 1)
		byFrame[event.SourceBytes.PacketRefs[0].Number] = event
	}
	require.Len(t, byFrame, 4, "Bind, BindAck, Request and Response are independently framed")

	bind := byFrame[4]
	require.NotNil(t, bind)
	require.Equal(t, "decoded", bind.Status)
	require.Equal(t, "Bind", bind.Session["Packet Name"])
	require.Equal(t, "EPM", bind.Session["Interface"])
	require.Equal(t, "proposed", bind.Session["Context State"])

	ack := byFrame[5]
	require.NotNil(t, ack)
	require.Equal(t, "decoded", ack.Status)
	require.Equal(t, "Bind", ack.Session["Matched Request"])
	require.Equal(t, []uint16{0}, ack.Session["Accepted Context IDs"])

	request := byFrame[6]
	require.NotNil(t, request)
	require.Equal(t, "decoded", request.Status)
	require.Equal(t, "EPM", request.Session["Interface"])
	require.Equal(t, uint16(99), request.Session["OpNum"])

	response := byFrame[7]
	require.NotNil(t, response)
	require.Equal(t, "decoded", response.Status)
	require.Equal(t, "matched", response.Session["Association Status"])
	require.Equal(t, "EPM.op_99", response.Session["Matched Request"])
}
