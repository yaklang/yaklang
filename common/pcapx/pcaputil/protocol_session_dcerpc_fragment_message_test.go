package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func rpcMessageFragment(ptype, flags byte, call uint32, context, operation uint16, remaining int, stub []byte) []byte {
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header, uint32(remaining))
	binary.LittleEndian.PutUint16(header[4:], context)
	if ptype == 0 {
		binary.LittleEndian.PutUint16(header[6:], operation)
	}
	return dcerpcPDU(ptype, flags, call, append(header, stub...))
}

func TestDCERPCFragmentMessageTypedFieldsKeepOwnedContext(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	stub := bytes.Repeat([]byte{0xa5}, 40)
	r := s.Feed(0, time.Unix(1, 0), rpcMessageFragment(0, 1, 2, 0, 2, 40, stub[:20]))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	first := r.Events[0].ID
	r = s.Feed(0, time.Unix(2, 0), rpcMessageFragment(0, 2, 2, 0, 2, 20, stub[20:]))
	require.Nil(t, r.Err)
	event := r.Events[0]
	fields := event.DisplayFields()
	require.Equal(t, []any{uint64(2)}, fields["dcerpc.cn_call_id"])
	require.Equal(t, []any{uint64(2)}, fields["dcerpc.opnum"])
	require.Equal(t, []any{uint64(40)}, fields["dcerpc.reassembled_length"])
	require.Equal(t, []any{first, event.ID}, fields["dcerpc.message_parent"])
	require.Equal(t, []any{false}, fields["dcerpc.stub_decoded"])
	query, err := CompileDisplayFilter("dcerpc.opnum == 2 and dcerpc.reassembled_length == 40")
	require.NoError(t, err)
	require.True(t, query.EvaluateEvents([]*ProtocolEvent{event}).Matched)
	event.Session = map[string]any{"Call ID": uint32(99), "OpNum": uint16(99), "Reassembled Message": map[string]any{"Stub Bytes": 999}}
	require.Equal(t, fields, event.DisplayFields(), "public derived caches cannot forge the owned call/header/stub observations")
}

func TestDCERPCFragmentMessageExcludesRepeatedPerPDUHeaders(t *testing.T) {
	d := newReviewDCERPC()
	stub := make([]byte, 40)
	for i := range stub {
		stub[i] = byte(i)
	}
	first := &ProtocolEvent{ID: 10, Raw: rpcMessageFragment(0, 1, 2, 0, 2, 40, stub[:20])}
	last := &ProtocolEvent{ID: 11, Raw: rpcMessageFragment(0, 2, 2, 0, 2, 20, stub[20:])}
	require.NoError(t, d.consumeEvent(0, first, 4096))
	require.NoError(t, d.consumeEvent(0, last, 4096))
	require.Equal(t, 40, last.Session["Stub Bytes"], "each continuation's request header is not part of the logical stub")
	message, ok := last.Session["Reassembled Message"].(map[string]any)
	require.True(t, ok, "logical payload must be inspectable separately from the final wire fragment")
	require.Equal(t, stub, message["Stub"])
	require.Equal(t, uint64(10), message["Message ID"])
	require.Equal(t, uint64(11), message["Complete In PDU"])
	require.Equal(t, 2, message["Fragment Count"])
	require.Equal(t, false, message["Stub Decoded"])
	pieces := message["Fragment Sources"].([]map[string]any)
	require.Len(t, pieces, 2)
	var reconstructed []byte
	for index, piece := range pieces {
		parent := []*ProtocolEvent{first, last}[index]
		require.Equal(t, parent.ID, piece["PDU ID"])
		span := piece["Stub Range"].([]uint64)
		require.Equal(t, []uint64{24, 44}, span)
		reconstructed = append(reconstructed, parent.Raw[span[0]:span[1]]...)
	}
	require.Equal(t, stub, reconstructed, "every logical octet comes from its exact parent PDU payload interval")

	reply := bytes.Repeat([]byte{0x5a, 0xa5}, 301)
	r1 := &ProtocolEvent{ID: 12, Raw: rpcMessageFragment(2, 1, 2, 0, 0, len(reply), reply[:137])}
	r2 := &ProtocolEvent{ID: 13, Raw: rpcMessageFragment(2, 2, 2, 0, 0, len(reply)-137, reply[137:])}
	require.NoError(t, d.consumeEvent(1, r1, 4096))
	require.Zero(t, r1.ResponseTo, "a first response fragment cannot retire the pending complete request")
	require.NoError(t, d.consumeEvent(1, r2, 4096))
	require.Equal(t, uint64(10), r2.ResponseTo, "reply identifies the first PDU of the now-complete request message")
	require.Equal(t, reply, r2.Session["Reassembled Message"].(map[string]any)["Stub"])
	require.Empty(t, d.pending)
	require.Empty(t, d.pendingPDU)
}

func TestDCERPCFragmentContextOrOperationCannotDrift(t *testing.T) {
	for _, ptype := range []byte{0, 2} {
		for _, operationChange := range []bool{false, true} {
			if ptype == 2 && operationChange {
				continue
			}
			d := newReviewDCERPC()
			first := &ProtocolEvent{ID: 1, Raw: rpcMessageFragment(ptype, 1, 9, 0, 2, 2, []byte{1})}
			last := &ProtocolEvent{ID: 2, Raw: rpcMessageFragment(ptype, 2, 9, 0, 2, 1, []byte{2})}
			if operationChange {
				last.Raw[22]++
			} else {
				last.Raw[20]++
			}
			require.NoError(t, d.consumeEvent(0, first, 4096))
			require.Error(t, d.consumeEvent(0, last, 4096), "same Call ID is insufficient for fragment identity")
			require.Empty(t, d.frags)
			require.Zero(t, last.ResponseTo)
			require.NotContains(t, last.Session, "Reassembled Message")
		}
	}
}

func TestDCERPCFragmentParentsAndPayloadHaveSharedResourceBounds(t *testing.T) {
	d := newReviewDCERPC()
	for index := 0; index < 3; index++ {
		flags := byte(0)
		if index == 0 {
			flags = 1
		}
		e := &ProtocolEvent{ID: uint64(index + 1), Raw: rpcMessageFragment(0, flags, 9, 0, 2, 4-index, []byte{byte(index)})}
		require.NoError(t, d.consumeEvent(0, e, 4))
	}
	e := &ProtocolEvent{ID: 4, Raw: rpcMessageFragment(0, 2, 9, 0, 2, 1, []byte{3})}
	require.Error(t, d.consumeEvent(0, e, 4), "parent ranges count against the collection limit before append")
	require.Empty(t, d.frags)
	require.Empty(t, d.pendingPDU)
	require.NotContains(t, e.Session, "Reassembled Message")

	d = newReviewDCERPC()
	d.maxMessageBytes = 25
	request := &ProtocolEvent{ID: 10, Raw: dcerpcRequest(9, 0, 2, nil)}
	require.NoError(t, d.consumeEvent(0, request, 4096))
	first := &ProtocolEvent{ID: 11, Raw: rpcMessageFragment(2, 1, 9, 0, 0, 2, []byte{1})}
	require.NoError(t, d.consumeEvent(1, first, 4096))
	last := &ProtocolEvent{ID: 12, Raw: rpcMessageFragment(2, 2, 9, 0, 0, 1, []byte{2})}
	require.Error(t, d.consumeEvent(1, last, 4096))
	require.Empty(t, d.frags)
	require.Equal(t, uint64(10), d.pendingPDU[9], "a resource refusal cannot falsely retire the request")
	require.Zero(t, last.ResponseTo)
}

func FuzzDCERPCFragmentMessageHeaderIsolation(f *testing.F) {
	f.Add(bytes.Repeat([]byte{0x5a}, 40), byte(0))
	f.Add([]byte{0, 1, 2, 3}, byte(1))
	f.Add([]byte{}, byte(6))
	f.Fuzz(func(t *testing.T, payload []byte, mutation byte) {
		if len(payload) > 4096 {
			payload = payload[:4096]
		}
		middle := len(payload) / 2
		d := newReviewDCERPC()
		first := &ProtocolEvent{ID: 1, Raw: rpcMessageFragment(0, 1, 9, 0, 2, len(payload), payload[:middle])}
		last := &ProtocolEvent{ID: 2, Raw: rpcMessageFragment(0, 2, 9, 0, 2, len(payload)-middle, payload[middle:])}
		switch mutation % 9 {
		case 1:
			last.Raw[20]++
		case 2:
			last.Raw[22]++
		case 3:
			last.Raw[2] = 2
		case 4:
			last.Raw = dcerpcSecurityTestPDU(last.Raw, []byte{1, 2, 3}, 6)
		case 5:
			last.Raw = dcerpcPDU(0, 2, 9, []byte{1, 2, 3, 4})
		case 6:
			binary.LittleEndian.PutUint32(last.Raw[16:20], ^uint32(0))
		case 7:
			last.Raw[3] |= dcerpcObject
		case 8:
			last.Raw[3] = dcerpcFirst
		}
		require.NoError(t, d.consumeEvent(0, first, 4096))
		err := d.consumeEvent(0, last, 4096)
		if mutation%9 == 0 || mutation%9 == 6 {
			require.NoError(t, err)
			message := last.Session["Reassembled Message"].(map[string]any)
			require.True(t, bytes.Equal(payload, message["Stub"].([]byte)), "only actual stub octets, never per-fragment headers or Alloc Hint padding")
			require.Equal(t, uint64(1), d.pendingPDU[9])
		} else {
			require.Empty(t, d.pendingPDU)
			require.NotContains(t, last.Session, "Reassembled Message")
		}
		require.Empty(t, d.frags)
		require.Zero(t, last.ResponseTo)
	})
}
