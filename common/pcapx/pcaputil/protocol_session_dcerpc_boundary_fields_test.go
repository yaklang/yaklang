package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDCERPCInvalidResultCannotBorrowMissingRequestContext(t *testing.T) {
	for _, ptype := range []byte{12, 15} {
		for _, index := range []int{0, 1} {
			entries := []dcerpcBindAckEntry{{syntax: dcerpcNDR, version: 2}, {syntax: dcerpcNDR, version: 2}}
			entries[index].result = 256
			wire := dcerpcBindAckResults(ptype, 1, entries...)
			_, err := newReviewDCERPC().consumeFrom(1, wire, 4096)
			require.ErrorContains(t, err, "invalid bind result", "result values are intrinsic even without an observed request")
			events, _ := sessionTestFlow(t, "dcerpc", []sessionStep{{1, wire}}, 1, true)
			require.NotEmpty(t, events)
			require.Equal(t, "malformed", events[0].Status)
			_, err = events[0].GetFields()
			require.Error(t, err)
		}
	}
}

func TestDCERPCBoundedBadResultTailDoesNotEraseOtherDirection(t *testing.T) {
	bad := dcerpcBindAck(1)
	bad[28] = 0 // A complete result remains after a list declaring zero entries.
	for _, deferred := range []bool{false, true} {
		for _, chunk := range []int{0, 1} {
			events, _ := sessionTestFlow(t, "dcerpc", []sessionStep{
				{0, dcerpcBind(dcerpcEPM, 0, 1)}, {1, bad},
				{0, dcerpcRequest(2, 0, 3, nil)}, {1, dcerpcBindAck(1)},
				{0, dcerpcRequest(3, 0, 3, nil)}, {1, dcerpcResponse(3, 0, nil)},
			}, chunk, deferred)
			var messages []*ProtocolEvent
			for _, e := range events {
				if len(e.Raw) > 0 {
					messages = append(messages, e)
				}
			}
			require.Len(t, messages, 6)
			require.Equal(t, "malformed", messages[1].Status)
			require.Equal(t, "message", messages[1].Session["Error Scope"])
			require.Equal(t, false, messages[1].Session["Presentation Context Updated"])
			require.NotContains(t, messages[2].Session, "Interface", "rejected bytes cannot install an accepted context")
			require.Equal(t, "Bind", messages[3].Session["Matched Request"], "the original proposal owner survives")
			require.Equal(t, "EPM.ept_map", messages[4].Session["Packet Name"])
			require.Equal(t, "matched", messages[5].Session["Association Status"])
		}
	}
}

func TestDCERPCObjectUUIDHasItsOwnWireField(t *testing.T) {
	object := bytes.Repeat([]byte{0xa5}, 16)
	stub := []byte{0, 0xff, 0x80, 1}
	wire := dcerpcRequest(2, 7, 3, append(bytes.Clone(object), stub...))
	wire[3] |= dcerpcObject
	events, _ := sessionTestFlow(t, "dcerpc", []sessionStep{{0, wire}}, 1, false)
	require.NotEmpty(t, events)
	e := events[0]
	require.Equal(t, "decoded", e.Status, e.Error)
	fields, err := e.GetFields()
	require.NoError(t, err)
	request := fields["PDU"].(map[string]any)["Request"].(map[string]any)
	require.Equal(t, object, request["Object UUID"])
	require.Equal(t, stub, request["Stub"], "object identity is not part of the NDR stub")
	require.Equal(t, object, e.Session["Object UUID"])
	require.Equal(t, len(stub), e.Session["Stub Bytes"])
	require.Equal(t, []uint64{24, 40}, e.Session["Wire Field Ranges"].(map[string]any)["Object UUID"])
	for cut := 24; cut < 40; cut++ {
		bad := bytes.Clone(wire[:cut])
		binary.LittleEndian.PutUint16(bad[8:10], uint16(cut))
		events, _ = sessionTestFlow(t, "dcerpc", []sessionStep{{0, bad}}, 0, false)
		require.Equal(t, "malformed", events[0].Status)
	}
}

func TestDCERPCResultTailRecoveryCannotScanOrBypassLimits(t *testing.T) {
	bad := dcerpcBindAckResults(15, 1, dcerpcBindAckEntry{syntax: dcerpcNDR, version: 2}, dcerpcBindAckEntry{syntax: dcerpcNDR, version: 2})
	bad[28] = 1
	failure := &ProtocolError{Kind: ErrMalformedMessage, Message: "structured parse: 24 unconsumed message bytes"}
	require.NotNil(t, dcerpcIsolatedUnconsumedTail(bad, failure))
	for cut := 0; cut < len(bad); cut++ {
		require.Nil(t, dcerpcResultTailDiagnostic(bad[:cut]))
	}
	for _, mutate := range []func([]byte){
		func(w []byte) { w[3] = dcerpcFirst },
		func(w []byte) { w[10] = 1 },
		func(w []byte) { w[28] = 21 },
		func(w []byte) { w[29] = 1 },
		func(w []byte) { binary.LittleEndian.PutUint16(w[24:26], 257) },
		func(w []byte) { binary.LittleEndian.PutUint16(w[32:34], 256) },
	} {
		wire := bytes.Clone(bad)
		mutate(wire)
		require.Nil(t, dcerpcIsolatedUnconsumedTail(wire, failure))
	}
	for _, kind := range []ProtocolErrorKind{ErrResourceExceeded, ErrDesynchronized, ErrContextRequired} {
		require.Nil(t, dcerpcIsolatedUnconsumedTail(bad, &ProtocolError{Kind: kind, Message: failure.Message}))
	}
	_, err := newReviewDCERPC().consumeFrom(0, bad, 4096)
	require.ErrorContains(t, err, "result list length", "direct session parsing still refuses the malformed PDU")
}

func FuzzDCERPCResultTailRecovery(f *testing.F) {
	f.Add([]byte{0xff, 0x80, 1}, uint8(0))
	f.Add([]byte("opaque"), uint8(8))
	f.Fuzz(func(t *testing.T, payload []byte, mode uint8) {
		payload = payload[:min(len(payload), 32)]
		declared := 1 + int(mode%3)
		entries := make([]dcerpcBindAckEntry, declared+1)
		for i := range entries {
			entries[i] = dcerpcBindAckEntry{syntax: dcerpcNDR, version: 2}
		}
		bad := dcerpcBindAckResults(12, 1, entries...)
		bad[28] = byte(declared)
		recover := mode&8 == 0
		if !recover {
			binary.LittleEndian.PutUint16(bad[32:34], 256)
		}
		events, _ := sessionTestFlow(t, "dcerpc", []sessionStep{
			{0, dcerpcBind(dcerpcEPM, 0, 1)}, {1, bad},
			{0, dcerpcRequest(3, 0, 3, payload)},
		}, int(mode%13), mode&4 != 0)
		observed, refused := false, false
		for _, e := range events {
			if bytes.Equal(e.Raw, bad) {
				refused = true
				require.Equal(t, "malformed", e.Status)
				_, err := e.GetFields()
				require.Error(t, err)
			}
			if e.Session["Call ID"] == uint32(3) && e.Session["Packet Name"] == "Request" {
				observed = true
				require.NotContains(t, e.Session, "Interface", "bad result tail cannot establish presentation context")
			}
		}
		require.True(t, refused)
		require.Equal(t, recover, observed)
	})
}
