package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDCERPCReplyDoesNotConsumePresentationNegotiation(t *testing.T) {
	for _, ptype := range []byte{11, 14} {
		for _, replyType := range []byte{2, 3} {
			t.Run(dcerpcPTypeName(ptype)+"/"+dcerpcPTypeName(replyType), func(t *testing.T) {
				d := newReviewDCERPC()
				_, err := d.consumeFrom(0, dcerpcBindType(ptype, dcerpcEPM, 0, 7), 4096)
				require.NoError(t, err)
				body := make([]byte, 8)
				if replyType == 3 {
					body = make([]byte, 16)
				}
				info, err := d.consumeFrom(1, dcerpcPDU(replyType, 3, 7, body), 4096)
				require.NoError(t, err)
				require.Equal(t, "request-type-mismatch", info["Association Status"])
				require.Equal(t, true, info["Unmatched"])
				require.NotContains(t, info, "Matched Request")
				require.Equal(t, dcerpcPTypeName(ptype), d.pending[7])
				require.Len(t, d.proposals[7], 1)
				ackType := byte(12)
				if ptype == 14 {
					ackType = 15
				}
				info, err = d.consumeFrom(1, dcerpcBindAckPType(ackType, 7, 0, dcerpcNDR, 2), 4096)
				require.NoError(t, err)
				require.Equal(t, "matched", info["Association Status"])
				require.Equal(t, "EPM", d.ctx[0])
				require.Empty(t, d.pending)
				require.Empty(t, d.proposals)
			})
		}
	}
}

func TestProtocolSessionDCERPCReplyOwnsExactRequestPDU(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	feed := func(dir int, wire []byte) *ProtocolEvent {
		r := s.Feed(dir, time.Unix(1, 0), wire)
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		require.Positive(t, r.Events[0].ID)
		return r.Events[0]
	}
	bind := feed(0, dcerpcBind(dcerpcEPM, 0, 1))
	ack := feed(1, dcerpcBindAck(1))
	require.Equal(t, bind.ID, ack.ResponseTo)
	for _, call := range []uint32{0, 7, 7} {
		request := feed(0, dcerpcRequest(call, 0, 2, nil))
		require.Zero(t, request.ResponseTo)
		require.Equal(t, "EPM.ept_lookup", request.Session["Packet Name"])
		wrong := feed(1, dcerpcResponse(call, 1, nil))
		require.Zero(t, wrong.ResponseTo)
		require.Equal(t, "context-mismatch", wrong.Session["Association Status"])
		response := feed(1, dcerpcResponse(call, 0, nil))
		require.Equal(t, request.ID, response.ResponseTo)
		require.Equal(t, "EPM.ept_lookup", response.Session["Matched Request"])
		values := response.DisplayFieldValues()
		found := false
		for _, value := range values.Values {
			if value.Name == "pdu.response_to" {
				require.Equal(t, request.ID, value.Value)
				found = true
			}
		}
		require.True(t, found, "the public typed projection must preserve the generated link")
		duplicate := feed(1, dcerpcResponse(call, 0, nil))
		require.Zero(t, duplicate.ResponseTo)
		require.Equal(t, "missing-request", duplicate.Session["Association Status"])
	}
}

func TestDCERPCRefusedMessagesCannotStealNumericOwner(t *testing.T) {
	d := newReviewDCERPC()
	request := &ProtocolEvent{ID: 42, Raw: dcerpcRequest(9, 0, 2, nil)}
	require.NoError(t, d.consumeEvent(0, request, 4096))
	for _, step := range []struct {
		dir  int
		wire []byte
	}{
		{1, dcerpcBindAck(9)},
		{1, dcerpcBindNak(9)},
		{0, dcerpcResponse(9, 0, nil)},
		{1, dcerpcResponse(9, 1, nil)},
		{0, dcerpcRequest(9, 0, 2, nil)},
		{1, dcerpcRequest(9, 0, 2, nil)},
		{1, dcerpcSecurityTestPDU(dcerpcResponse(9, 0, nil), []byte{1, 2, 3}, 6)},
	} {
		e := &ProtocolEvent{ID: 100, Raw: step.wire}
		_ = d.consumeEvent(step.dir, e, 4096)
		require.Zero(t, e.ResponseTo)
		require.Equal(t, uint64(42), d.pendingPDU[9])
		require.Contains(t, d.pending, uint32(9))
	}
	fault := &ProtocolEvent{ID: 101, Raw: dcerpcPDU(3, 3, 9, make([]byte, 16))}
	require.NoError(t, d.consumeEvent(1, fault, 4096))
	require.Equal(t, uint64(42), fault.ResponseTo)
	require.Empty(t, d.pendingPDU)
	require.Empty(t, d.pending)

	// Reusing a completed wire Call ID must get a fresh immutable PDU owner.
	request = &ProtocolEvent{ID: 102, Raw: dcerpcRequest(9, 0, 4, nil)}
	require.NoError(t, d.consumeEvent(0, request, 4096))
	response := &ProtocolEvent{ID: 103, Raw: dcerpcResponse(9, 0, nil)}
	require.NoError(t, d.consumeEvent(1, response, 4096))
	require.Equal(t, uint64(102), response.ResponseTo)
}

func TestDCERPCNumericOwnerIsReservedBeforeAdmission(t *testing.T) {
	d := newReviewDCERPC()
	// The pending request has a context slot and an optional numeric-owner slot.
	d.maxBufferedBytes = int(d.retainedBytes()) + 96 + len("Request") - 1
	e := &ProtocolEvent{ID: 42, Raw: dcerpcRequest(9, 0, 2, nil)}
	require.Error(t, d.consumeEvent(0, e, 4096))
	require.Empty(t, d.pending)
	require.Empty(t, d.pendingPDU)
	d.maxBufferedBytes++
	require.NoError(t, d.consumeEvent(0, e, 4096))
	require.Equal(t, uint64(42), d.pendingPDU[9])
	require.Equal(t, int64(d.maxBufferedBytes), d.retainedBytes())
}

func TestDCERPCFragmentViewsDoNotInventNumericCallLinks(t *testing.T) {
	d := newReviewDCERPC()
	wire := dcerpcRequest(9, 0, 2, []byte{1, 2})
	first := &ProtocolEvent{ID: 42, Raw: dcerpcPDU(0, 1, 9, wire[16:24])}
	last := &ProtocolEvent{ID: 43, Raw: dcerpcPDU(0, 2, 9, wire[24:])}
	require.NoError(t, d.consumeEvent(0, first, 4096))
	require.NoError(t, d.consumeEvent(0, last, 4096))
	require.Empty(t, d.pendingPDU, "logical fragment body needs its own owned source before a numeric link")
	response := &ProtocolEvent{ID: 44, Raw: dcerpcResponse(9, 0, nil)}
	require.NoError(t, d.consumeEvent(1, response, 4096))
	require.Zero(t, response.ResponseTo)
}

func FuzzDCERPCNumericCallOwnership(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte{2, 2, 2, 0, 0, 0})
	f.Add([]byte{0x60, 0x24, 0x44, 0x84, 0, 0x24, 0, 0xa4})
	f.Fuzz(func(t *testing.T, steps []byte) {
		if len(steps) > 128 {
			steps = steps[:128]
		}
		d := newReviewDCERPC()
		for index, step := range steps {
			call := uint32(step & 3)
			dir := int(step>>2) & 1
			context := uint16(step>>3) & 1
			ptype := []byte{0, 2, 3, 11, 12, 13, 14, 15}[step>>5]
			var wire []byte
			switch ptype {
			case 0:
				wire = dcerpcRequest(call, context, 2, nil)
			case 2:
				wire = dcerpcResponse(call, context, nil)
			case 3:
				body := make([]byte, 16)
				binary.LittleEndian.PutUint16(body[4:6], context)
				wire = dcerpcPDU(3, 3, call, body)
			case 11, 14:
				wire = dcerpcBindType(ptype, dcerpcEPM, context, call)
			case 12, 15:
				wire = dcerpcBindAckPType(ptype, call, 0, dcerpcNDR, 2)
			case 13:
				wire = dcerpcBindNak(call)
			}
			owner := d.pendingPDU[call]
			e := &ProtocolEvent{ID: uint64(index + 1), Raw: wire}
			err := d.consumeEvent(dir, e, 4096)
			if e.ResponseTo != 0 {
				require.NoError(t, err)
				require.Equal(t, owner, e.ResponseTo)
				require.Equal(t, "matched", e.Session["Association Status"])
				require.Less(t, e.ResponseTo, e.ID)
			}
			for id := range d.pendingPDU {
				require.Contains(t, d.pending, id, "numeric ownership must never outlive the outstanding call")
			}
		}
	})
}
