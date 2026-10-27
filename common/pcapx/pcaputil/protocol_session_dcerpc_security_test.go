package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func dcerpcSecurityTestPDU(plain, token []byte, level byte) []byte {
	wire := bytes.Clone(plain)
	padding := (16 - (len(wire)-16)%16) % 16
	wire = append(wire, make([]byte, padding)...)
	wire = append(wire, 10, level, byte(padding), 0, 7, 0, 0, 0)
	wire = append(wire, token...)
	binary.LittleEndian.PutUint16(wire[8:10], uint16(len(wire)))
	binary.LittleEndian.PutUint16(wire[10:12], uint16(len(token)))
	return wire
}

func TestDCERPCSecurityEnvelopeNeverAuthenticatesOrCommitsContext(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, pdu := range [][]byte{dcerpcBind(dcerpcEPM, 0, 2), dcerpcRequest(2, 0, 3, []byte{1, 2, 3})} {
			token := []byte{0, 0xff, 0x80, 1, 2, 3}
			wire := dcerpcSecurityTestPDU(pdu, token, 6)
			events, _ := sessionTestFlow(t, "dcerpc", []sessionStep{{0, wire}}, 1, deferred)
			require.Len(t, events, 1)
			e := events[0]
			require.Contains(t, []string{"decoded", "deferred"}, e.Status, e.Error)
			require.Equal(t, "dcerpc-security-envelope", e.Profile)
			require.Equal(t, "envelope", e.Completeness)
			require.Equal(t, false, e.Session["Authentication Verified"])
			require.Equal(t, false, e.Session["Peer Identity Verified"])
			require.Equal(t, false, e.Session["Stub Decoded"])
			require.Equal(t, false, e.Session["Trailer Alignment Validated"])
			require.Equal(t, token, e.Session["Authentication Token"])
			require.Equal(t, pdu[16:], e.Session["Opaque PDU Body"])
			d := newReviewDCERPC()
			_, err := d.consumeFrom(0, wire, 4096)
			require.NoError(t, err)
			require.Empty(t, d.ctx)
			require.Empty(t, d.pending)
			require.Empty(t, d.proposals)
			require.Equal(t, [2]bool{}, d.peerMessages, "an opaque security PDU is not an interpreted peer completion")
			fields, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, token, fields["Authentication Token"])
			fields["Authentication Token"].([]byte)[0] = 0x99
			e.Fields, e.Structured = map[string]any{"Accepted": true}, nil
			e.Protocol, e.Status = "tcp", "malformed"
			fields, err = e.GetFields()
			require.NoError(t, err)
			require.Equal(t, token, fields["Authentication Token"], "native accepted wire fields have private ownership")
		}
	}
}

func TestDCERPCMissingPeerCompletionIsContextAndLateAckCanRecover(t *testing.T) {
	d := newReviewDCERPC()
	_, err := d.consumeFrom(0, dcerpcBind(dcerpcEPM, 0, 2), 4096)
	require.NoError(t, err)
	_, err = d.consumeFrom(0, dcerpcRequest(2, 0, 3, nil), 4096)
	require.ErrorIs(t, err, errBinContext, "a missing peer bind completion is not proof of malformed reuse")
	require.Equal(t, "Bind", d.pending[2], "missing evidence cannot discard the actual bind proposal")
	require.Empty(t, d.ctx)
	_, err = d.consumeFrom(1, dcerpcBindAck(2), 4096)
	require.NoError(t, err)
	info, err := d.consumeFrom(0, dcerpcRequest(2, 0, 3, nil), 4096)
	require.NoError(t, err)
	require.Equal(t, "EPM.ept_map", info["Packet Name"])
	for _, deferred := range []bool{false, true} {
		events, _ := sessionTestFlow(t, "dcerpc", []sessionStep{
			{0, dcerpcBind(dcerpcEPM, 0, 2)},
			{0, dcerpcRequest(2, 0, 3, nil)},
			{1, dcerpcBindAck(2)},
			{0, dcerpcRequest(2, 0, 3, nil)},
		}, 1, deferred)
		messages := events[:0]
		for _, event := range events {
			if len(event.Raw) > 0 {
				messages = append(messages, event)
			}
		}
		events = messages // Close also emits an explicit outstanding-call diagnostic.
		require.Len(t, events, 4, "a message-level missing completion must not close the framed connection")
		require.Equal(t, "context-required", events[1].Status)
		require.Equal(t, "message", events[1].Session["Error Scope"])
		require.Equal(t, "EPM.ept_map", events[3].Session["Packet Name"])
		_, original := events[1].GetFields()
		require.ErrorContains(t, original, "ContextRequired")
		events[1].Status, events[1].Fields = "decoded", map[string]any{"Accepted": true}
		_, repeated := events[1].GetFields()
		require.EqualError(t, repeated, original.Error(), "published context diagnostics remain immutable after recovery")
	}
}

func TestDCERPCBindFeatureAckIsNotAnAcceptedTransferContext(t *testing.T) {
	feature := []byte{0x2c, 0x1c, 0xb7, 0x6c, 0x12, 0x98, 0x40, 0x45, 3, 0, 0, 0, 0, 0, 0, 0}
	bind := dcerpcBindContexts(11, 2, dcerpcBindContext{uuid: dcerpcEPM, id: 0}, dcerpcBindContext{uuid: dcerpcEPM, id: 1})
	copy(bind[16+12+44+24:16+12+44+40], feature)
	binary.LittleEndian.PutUint32(bind[16+12+44+40:], 1)
	ack := dcerpcBindAckResults(12, 2, dcerpcBindAckEntry{result: 0, syntax: dcerpcNDR, version: 2}, dcerpcBindAckEntry{result: 3, syntax: make([]byte, 16), version: 0})
	binary.LittleEndian.PutUint16(ack[16+16+24+2:], 3)
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	require.Nil(t, s.Feed(0, time.Time{}, bind).Err)
	r := s.Feed(1, time.Time{}, ack)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, []uint16{0}, r.Events[0].Session["Accepted Context IDs"])
	require.Equal(t, uint16(3), r.Events[0].Session["Observed Bind Time Feature Bits"])
	d := s.(*captureSession).f.dcerpc
	require.NotContains(t, d.ctx, uint16(1), "feature negotiation is not an NDR transfer context")
	require.Empty(t, d.pending)
}

func TestDCERPCSecurityEnvelopeRefusalAndFragmentIsolation(t *testing.T) {
	wire := dcerpcSecurityTestPDU(dcerpcRequest(9, 0, 3, nil), []byte{1, 2, 3}, 6)
	for _, mutate := range []func([]byte){
		func(w []byte) { w[len(w)-3-8+2] = 0xff },
		func(w []byte) { binary.LittleEndian.PutUint16(w[10:12], uint16(len(w))) },
		func(w []byte) { w[len(w)-3-8+1] = 0 },
	} {
		bad := bytes.Clone(wire)
		mutate(bad)
		events, _ := sessionTestFlow(t, "dcerpc", []sessionStep{{0, bad}}, 1, true)
		require.NotEmpty(t, events)
		e := events[0]
		require.Contains(t, []string{"malformed", "context-required"}, e.Status)
		_, original := e.GetFields()
		require.Error(t, original)
		e.Protocol, e.Status, e.Entry = "dcerpc", "decoded", "DCERPC"
		e.Fields, e.Structured, e.Session = map[string]any{"Accepted": true}, map[string]any{"fields": map[string]any{"Accepted": true}}, map[string]any{"Authentication Verified": true}
		_, repeated := e.GetFields()
		require.EqualError(t, repeated, original.Error())
	}
	d := newReviewDCERPC()
	_, err := d.consumeFrom(0, dcerpcPDU(0, dcerpcFirst, 9, make([]byte, 8)), 4096)
	require.NoError(t, err)
	require.Len(t, d.frags, 1)
	info, err := d.consumeFrom(0, wire, 4096)
	require.NoError(t, err)
	require.Equal(t, false, info["Stub Decoded"])
	require.Empty(t, d.frags, "opaque protected bytes cannot join a retained clear fragment")
	require.Empty(t, d.pending)
}

func TestDCERPCSecurityEnvelopeDoesNotGuessPDUAlignmentOrigin(t *testing.T) {
	plain := dcerpcBindAckPType(15, 4, 0, dcerpcNDR, 2)
	require.Equal(t, 56, len(plain))
	wire := append(bytes.Clone(plain), []byte{68, 6, 0, 0, 1, 0, 0, 0}...)
	token := make([]byte, 12)
	wire = append(wire, token...)
	binary.LittleEndian.PutUint16(wire[8:10], uint16(len(wire)))
	binary.LittleEndian.PutUint16(wire[10:12], uint16(len(token)))
	events, _ := sessionTestFlow(t, "dcerpc", []sessionStep{{1, wire}}, 1, false)
	require.NotEmpty(t, events)
	require.Equal(t, "decoded", events[0].Status, events[0].Error)
	require.Equal(t, token, events[0].Session["Authentication Token"])
	require.Equal(t, false, events[0].Session["Trailer Alignment Validated"])
}

func TestDCERPCBindCountCannotHideTrailingContext(t *testing.T) {
	wire := dcerpcBindContexts(11, 3, dcerpcBindContext{uuid: dcerpcEPM, id: 0}, dcerpcBindContext{uuid: dcerpcEPM, id: 1}, dcerpcBindContext{uuid: dcerpcEPM, id: 2})
	wire[24] = 2 // Three physical contexts, only two declared in the wire header.
	_, err := newReviewDCERPC().consumeFrom(0, wire, 4096)
	require.ErrorContains(t, err, "44 trailing bytes")
	events, _ := sessionTestFlow(t, "dcerpc", []sessionStep{{0, wire}}, 1, false)
	require.NotEmpty(t, events)
	require.Equal(t, "malformed", events[0].Status)
	_, original := events[0].GetFields()
	require.Error(t, original)
	events[0].Fields, events[0].Status = map[string]any{"Accepted": true}, "decoded"
	_, repeated := events[0].GetFields()
	require.EqualError(t, repeated, original.Error(), "a failed shared rule cannot be replaced by a caller-provided tree")
}

func FuzzDCERPCSecurityEnvelopeBoundaries(f *testing.F) {
	f.Add([]byte{0, 0xff, 0x80}, []byte{1, 2, 3}, uint8(0))
	f.Add([]byte("opaque body"), []byte("not-authenticated"), uint8(5))
	f.Fuzz(func(t *testing.T, body, token []byte, mode uint8) {
		body, token = body[:min(len(body), 256)], token[:min(len(token), 256)]
		if len(token) == 0 {
			token = []byte{0}
		}
		wire := dcerpcSecurityTestPDU(dcerpcRequest(2, 0, 3, body), token, 2+mode%5)
		d := newReviewDCERPC()
		info, err := d.consumeFrom(0, wire, 4096)
		require.NoError(t, err)
		require.Equal(t, token, info["Authentication Token"])
		require.Equal(t, false, info["Authentication Verified"])
		require.Equal(t, false, info["Stub Decoded"])
		require.Empty(t, d.pending)
		require.Empty(t, d.ctx)
		bad := bytes.Clone(wire)
		trailer := len(bad) - len(token) - 8
		switch mode % 3 {
		case 0:
			binary.LittleEndian.PutUint16(bad[10:12], uint16(len(bad)))
		case 1:
			bad[trailer+1] = 0
		case 2:
			bad[1] = 1
		}
		_, err = newReviewDCERPC().consumeFrom(0, bad, 4096)
		require.Error(t, err)
	})
}
