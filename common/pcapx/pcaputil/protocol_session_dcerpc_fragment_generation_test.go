package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDCERPCFragmentCallStartCannotCrossAnotherCallGeneration(t *testing.T) {
	t.Run("overlap-started-before-old-reply", func(t *testing.T) {
		d := newReviewDCERPC()
		old := &ProtocolEvent{ID: 10, Raw: dcerpcRequest(7, 0, 2, nil)}
		require.NoError(t, d.consumeEvent(0, old, 4096))
		first := &ProtocolEvent{ID: 11, Raw: rpcMessageFragment(0, 1, 7, 0, 4, 2, []byte{1})}
		firstErr := d.consumeEvent(0, first, 4096)
		reply := &ProtocolEvent{ID: 12, Raw: dcerpcResponse(7, 0, nil)}
		require.NoError(t, d.consumeEvent(1, reply, 4096))
		require.Equal(t, old.ID, reply.ResponseTo)
		last := &ProtocolEvent{ID: 13, Raw: rpcMessageFragment(0, 2, 7, 0, 4, 1, []byte{2})}
		lastErr := d.consumeEvent(0, last, 4096)
		require.Error(t, firstErr, "a request cannot begin while its Call ID still owns an outstanding call")
		require.Error(t, lastErr, "a refused first fragment cannot become valid merely because the old reply later completed")
		require.Empty(t, d.pending)
		require.Empty(t, d.pendingPDU)
		require.NotContains(t, last.Session, "Reassembled Message")
	})
	t.Run("reply-started-before-any-observed-request", func(t *testing.T) {
		d := newReviewDCERPC()
		first := &ProtocolEvent{ID: 10, Raw: rpcMessageFragment(2, 1, 7, 0, 0, 2, []byte{1})}
		require.NoError(t, d.consumeEvent(1, first, 4096))
		request := &ProtocolEvent{ID: 11, Raw: dcerpcRequest(7, 0, 2, nil)}
		require.NoError(t, d.consumeEvent(0, request, 4096))
		last := &ProtocolEvent{ID: 12, Raw: rpcMessageFragment(2, 2, 7, 0, 0, 1, []byte{2})}
		require.NoError(t, d.consumeEvent(1, last, 4096))
		require.Zero(t, last.ResponseTo, "a reply already in progress cannot acquire a later request with the same wire Call ID")
		require.Equal(t, request.ID, d.pendingPDU[7])
		reply := &ProtocolEvent{ID: 13, Raw: dcerpcResponse(7, 0, nil)}
		require.NoError(t, d.consumeEvent(1, reply, 4096))
		require.Equal(t, request.ID, reply.ResponseTo)
	})
	t.Run("first-request-observed-before-first-reply", func(t *testing.T) {
		d := newReviewDCERPC()
		r1 := &ProtocolEvent{ID: 10, Raw: rpcMessageFragment(0, 1, 7, 0, 2, 2, []byte{1})}
		s1 := &ProtocolEvent{ID: 11, Raw: rpcMessageFragment(2, 1, 7, 0, 0, 2, []byte{3})}
		r2 := &ProtocolEvent{ID: 12, Raw: rpcMessageFragment(0, 2, 7, 0, 2, 1, []byte{2})}
		s2 := &ProtocolEvent{ID: 13, Raw: rpcMessageFragment(2, 2, 7, 0, 0, 1, []byte{4})}
		require.NoError(t, d.consumeEvent(0, r1, 4096))
		require.NoError(t, d.consumeEvent(1, s1, 4096))
		require.NoError(t, d.consumeEvent(0, r2, 4096))
		require.NoError(t, d.consumeEvent(1, s2, 4096))
		require.Equal(t, r1.ID, s2.ResponseTo, "late request completion is valid when that original request began before the response")
	})
	t.Run("request-first-fragment-later-invalidated", func(t *testing.T) {
		d := newReviewDCERPC()
		r1 := &ProtocolEvent{ID: 10, Raw: rpcMessageFragment(0, 1, 7, 0, 2, 2, []byte{1})}
		s1 := &ProtocolEvent{ID: 11, Raw: rpcMessageFragment(2, 1, 7, 0, 0, 2, []byte{3})}
		r2 := &ProtocolEvent{ID: 12, Raw: rpcMessageFragment(0, 2, 7, 1, 2, 1, []byte{2})}
		require.NoError(t, d.consumeEvent(0, r1, 4096))
		require.NoError(t, d.consumeEvent(1, s1, 4096))
		require.Error(t, d.consumeEvent(0, r2, 4096))
		request := &ProtocolEvent{ID: 13, Raw: dcerpcRequest(7, 0, 2, nil)}
		require.NoError(t, d.consumeEvent(0, request, 4096))
		s2 := &ProtocolEvent{ID: 14, Raw: rpcMessageFragment(2, 2, 7, 0, 0, 1, []byte{4})}
		require.NoError(t, d.consumeEvent(1, s2, 4096))
		require.Zero(t, s2.ResponseTo, "same Call ID/opnum/context cannot join a response to a replacement request generation")
		require.Equal(t, request.ID, d.pendingPDU[7])
	})
}

func TestProtocolSessionDCERPCDuplicateFragmentDoesNotPoisonOtherCalls(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	first := rpcMessageFragment(0, 1, 7, 0, 2, 2, []byte{1})
	require.Nil(t, s.Feed(0, time.Unix(1, 0), first).Err)
	refused := s.Feed(0, time.Unix(2, 0), first)
	require.NotNil(t, refused.Err)
	require.Len(t, refused.Events, 1)
	require.Equal(t, "message", refused.Events[0].Session["Error Scope"])
	next := s.Feed(0, time.Unix(3, 0), dcerpcRequest(8, 0, 2, nil))
	require.Nil(t, next.Err, "a semantic refusal at a validated RPC boundary cannot disable later independent calls")
	require.Len(t, next.Events, 1)
	reply := s.Feed(1, time.Unix(4, 0), dcerpcResponse(8, 0, nil))
	require.Nil(t, reply.Err)
	require.Equal(t, next.Events[0].ID, reply.Events[0].ResponseTo)
}

func TestDCERPCFragmentGenerationWithoutPublicPDUIdentifiers(t *testing.T) {
	d := newReviewDCERPC()
	_, err := d.consumeFrom(1, rpcMessageFragment(2, 1, 7, 0, 0, 2, []byte{1}), 4096)
	require.NoError(t, err)
	_, err = d.consumeFrom(0, dcerpcRequest(7, 0, 2, nil), 4096)
	require.NoError(t, err)
	info, err := d.consumeFrom(1, rpcMessageFragment(2, 2, 7, 0, 0, 1, []byte{2}), 4096)
	require.NoError(t, err)
	require.Equal(t, "request-not-observed-at-message-start", info["Association Status"])
	require.Contains(t, d.pending, uint32(7))
	require.NotZero(t, d.pendingGeneration[7], "private call identity must work when the legacy API has no generated PDU IDs")
	info, err = d.consumeFrom(1, dcerpcResponse(7, 0, nil), 4096)
	require.NoError(t, err)
	require.Equal(t, "matched", info["Association Status"])
	require.Empty(t, d.pendingGeneration)
}

func TestDCERPCFragmentGenerationCannotWrapIntoUnknownIdentity(t *testing.T) {
	for _, fragmented := range []bool{false, true} {
		d := newReviewDCERPC()
		d.callGeneration = ^uint64(0) - 1
		wire := dcerpcRequest(7, 0, 2, nil)
		if fragmented {
			wire = rpcMessageFragment(0, 1, 7, 0, 2, 2, []byte{1})
		}
		e := &ProtocolEvent{ID: 10, Raw: wire}
		require.NoError(t, d.consumeEvent(0, e, 4096))
		if fragmented {
			e = &ProtocolEvent{ID: 11, Raw: rpcMessageFragment(0, 2, 7, 0, 2, 1, []byte{2})}
			require.NoError(t, d.consumeEvent(0, e, 4096), "the final pre-reserved identity can still complete")
		}
		require.Equal(t, ^uint64(0), d.pendingGeneration[7])
		e = &ProtocolEvent{ID: 12, Raw: dcerpcResponse(7, 0, nil)}
		require.NoError(t, d.consumeEvent(1, e, 4096))
		require.Equal(t, uint64(10), e.ResponseTo)
		for _, next := range [][]byte{dcerpcRequest(8, 0, 2, nil), rpcMessageFragment(0, 1, 8, 0, 2, 2, []byte{1}), dcerpcBind(dcerpcEPM, 0, 8)} {
			before := d.retainedBytes()
			e = &ProtocolEvent{ID: 13, Raw: next}
			require.Error(t, d.consumeEvent(0, e, 4096))
			require.Zero(t, e.ResponseTo)
			require.Empty(t, d.pending)
			require.Empty(t, d.pendingGeneration)
			require.Empty(t, d.frags)
			require.Equal(t, before, d.retainedBytes())
			require.Equal(t, ^uint64(0), d.callGeneration)
		}
	}
}

func FuzzDCERPCFragmentCallGenerationCausality(f *testing.F) {
	f.Add([]byte{0x34, 0x00, 0x44, 0x54})
	f.Add([]byte{0x10, 0x34, 0x20, 0x44})
	f.Add([]byte{0x10, 0x34, 0x60, 0x00, 0x44})
	f.Add([]byte{0x00, 0x10, 0x54, 0x20, 0x54})
	f.Fuzz(func(t *testing.T, steps []byte) {
		if len(steps) > 128 {
			steps = steps[:128]
		}
		d := newReviewDCERPC()
		observed := map[uint64][]byte{}
		for index, step := range steps {
			call, dir, context := uint32(step&3), int(step>>2&1), uint16(step>>3&1)
			var wire []byte
			switch step >> 4 {
			case 0:
				wire = dcerpcRequest(call, context, 2, nil)
			case 1, 12:
				wire = rpcMessageFragment(0, 1, call, context, 2, 2, []byte{1})
			case 2:
				wire = rpcMessageFragment(0, 2, call, context, 2, 1, []byte{2})
			case 3:
				wire = rpcMessageFragment(2, 1, call, context, 0, 2, []byte{3})
			case 4:
				wire = rpcMessageFragment(2, 2, call, context, 0, 1, []byte{4})
			case 5:
				wire = dcerpcResponse(call, context, nil)
			case 6:
				wire = rpcMessageFragment(0, 2, call, context^1, 2, 1, []byte{2})
			case 7:
				wire = dcerpcSecurityTestPDU(dcerpcResponse(call, context, nil), []byte{1, 2, 3}, 6)
			case 8:
				wire = dcerpcBind(dcerpcEPM, context, call)
			case 9:
				wire = dcerpcBindAck(call)
			case 10, 11:
				body := make([]byte, 16)
				body[4] = byte(context)
				flags := byte(1)
				if step>>4 == 11 {
					flags = 2
				}
				wire = dcerpcPDU(3, flags, call, body)
			case 13:
				wire = dcerpcPDU(19, 1, call, make([]byte, 8))
			case 14:
				wire = rpcMessageFragment(0, 0, call, context, 2, 1, []byte{2})
			case 15:
				wire = rpcMessageFragment(2, 0, call, context, 0, 1, []byte{4})
			}
			e := &ProtocolEvent{ID: uint64(index + 1), Raw: wire}
			observed[e.ID] = wire
			err := d.consumeEvent(dir, e, 4096)
			if e.ResponseTo != 0 {
				require.NoError(t, err)
				first := e.ID
				if message, ok := e.Session["Reassembled Message"].(map[string]any); ok {
					first = message["Message ID"].(uint64)
				}
				require.Contains(t, observed, e.ResponseTo)
				require.Less(t, e.ResponseTo, first, "a complete reply cannot acquire a request whose first PDU was observed after the reply began")
				request := observed[e.ResponseTo]
				require.Contains(t, []byte{0, 11, 14}, request[2])
				require.NotZero(t, request[3]&dcerpcFirst)
			}
			for call, generation := range d.pendingGeneration {
				require.NotZero(t, generation)
				require.Contains(t, d.pending, call, "private identity cannot outlive the original pending owner")
			}
		}
	})
}
