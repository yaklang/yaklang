package pcaputil

import (
	"encoding/binary"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newReviewDCERPC() *binDCERPC {
	return &binDCERPC{
		ctx:            map[uint16]string{},
		uuid:           map[uint16][]byte{},
		ctxVersion:     map[uint16]uint32{},
		pending:        map[uint32]string{},
		pendingDir:     map[uint32]int{},
		pendingContext: map[uint32]uint16{},
		proposals:      map[uint32][]dcerpcProposal{},
		frags:          map[dcerpcFragmentKey]dcerpcFragment{},
	}
}

func TestProtocolSessionDCERPCFragmentContextAndAuthSafety(t *testing.T) {
	const max = 4096
	call := uint32(71)
	body := []byte{0, 0, 0, 0, 0, 0, 3, 0, 0xaa, 0xbb}

	t.Run("orphan-continuation-is-context-required", func(t *testing.T) {
		d := newReviewDCERPC()
		_, err := d.consumeFrom(0, dcerpcPDU(0, dcerpcLast, call, body), max)
		require.ErrorIs(t, err, errBinContext)
		require.Empty(t, d.frags)
	})

	t.Run("continuation-must-match-first-fragment-direction", func(t *testing.T) {
		d := newReviewDCERPC()
		_, err := d.consumeFrom(0, dcerpcPDU(0, dcerpcFirst, call, body[:9]), max)
		require.NoError(t, err)
		_, err = d.consumeFrom(1, dcerpcPDU(0, dcerpcLast, call, body[9:]), max)
		require.ErrorIs(t, err, errBinContext)
		require.Len(t, d.frags, 1, "a reverse-direction PDU cannot consume the request-direction first fragment")
		info, err := d.consumeFrom(0, dcerpcPDU(0, dcerpcLast, call, body[9:]), max)
		require.NoError(t, err)
		require.Equal(t, 2, info["Stub Bytes"])
		require.Empty(t, d.frags)
	})

	t.Run("continuation-must-match-ptype-and-clears-corrupt-state", func(t *testing.T) {
		d := newReviewDCERPC()
		_, err := d.consumeFrom(0, dcerpcPDU(0, dcerpcFirst, call, body[:9]), max)
		require.NoError(t, err)
		_, err = d.consumeFrom(0, dcerpcPDU(2, dcerpcLast, call, make([]byte, 8)), max)
		require.Error(t, err)
		require.Equal(t, ErrDesynchronized, reviewProtocolKind(t, err))
		require.Empty(t, d.frags)
	})

	t.Run("orphan-final-does-not-cross-call-or-direction", func(t *testing.T) {
		d := newReviewDCERPC()
		_, err := d.consumeFrom(0, dcerpcPDU(0, dcerpcFirst, call, body[:9]), max)
		require.NoError(t, err)
		_, err = d.consumeFrom(0, dcerpcPDU(0, dcerpcLast, call+1, body[9:]), max)
		require.ErrorIs(t, err, errBinContext)
		require.Len(t, d.frags, 1)
	})

	t.Run("object-uuid-flag-from-first-fragment-is-retained", func(t *testing.T) {
		d := newReviewDCERPC()
		objectBody := make([]byte, 26)
		binary.LittleEndian.PutUint16(objectBody[4:6], 0)
		binary.LittleEndian.PutUint16(objectBody[6:8], 3)
		copy(objectBody[8:24], dcerpcEPM)
		objectBody[24], objectBody[25] = 0xaa, 0xbb
		_, err := d.consumeFrom(0, dcerpcPDU(0, dcerpcFirst|dcerpcObject, call, objectBody[:13]), max)
		require.NoError(t, err)
		info, err := d.consumeFrom(0, dcerpcPDU(0, dcerpcLast, call, objectBody[13:]), max)
		require.NoError(t, err)
		require.Equal(t, 2, info["Stub Bytes"])
		require.Equal(t, uint16(3), info["OpNum"])
	})

	t.Run("authenticated-stub-is-published-with-auth-metadata", func(t *testing.T) {
		d := newReviewDCERPC()
		wire := dcerpcTestAuth(dcerpcRequest(call, 0, 3, nil), 2, 0, 0)
		info, err := d.consumeFrom(0, wire, max)
		require.NoError(t, err)
		require.Equal(t, true, info["Authenticated"])
		require.Equal(t, uint8(10), info["Auth Type"])
		require.Equal(t, uint8(0), info["Auth Level"])
		require.Equal(t, 2, info["Auth Value Bytes"])
		require.Equal(t, uint16(3), info["OpNum"])

		bad := append([]byte(nil), wire...)
		binary.LittleEndian.PutUint16(bad[10:12], 100)
		_, err = d.consumeFrom(0, bad, max)
		require.ErrorContains(t, err, "authentication verifier length exceeds fragment payload")
	})
}

func dcerpcTestAuth(pdu []byte, authLen, pad int, level byte) []byte {
	pdu = append([]byte(nil), pdu...)
	body := append([]byte(nil), pdu[16:]...)
	body = append(body, make([]byte, pad)...)
	trailer := make([]byte, 8+authLen)
	trailer[0] = 10 // NTLMSSP
	trailer[1] = level
	trailer[2] = byte(pad)
	body = append(body, trailer...)
	binary.LittleEndian.PutUint16(pdu[8:10], uint16(16+len(body)))
	binary.LittleEndian.PutUint16(pdu[10:12], uint16(authLen))
	return append(pdu[:16], body...)
}

func TestProtocolSessionDCERPCAuthenticatedCallsContinue(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	ts := time.Unix(1, 0)

	bind := dcerpcTestAuth(dcerpcBindContexts(11, 1,
		dcerpcBindContext{uuid: dcerpcEPM, id: 0},
	), 40, 0, 6)
	r := s.Feed(0, ts, bind)
	require.Nil(t, r.Err)
	require.Equal(t, true, r.Events[0].Session["Authenticated"])
	require.Equal(t, uint8(6), r.Events[0].Session["Auth Level"])

	ack := dcerpcBindAckResults(12, 1, dcerpcBindAckEntry{result: 0, syntax: dcerpcNDR, version: 2})
	r = s.Feed(1, ts, ack)
	require.Nil(t, r.Err)
	require.Equal(t, []uint16{0}, r.Events[0].Session["Accepted Context IDs"])

	request := dcerpcTestAuth(dcerpcRequest(2, 0, 15, []byte{1, 2, 3, 4}), 16, 8, 6)
	r = s.Feed(0, ts, request)
	require.Nil(t, r.Err)
	require.Equal(t, uint16(15), r.Events[0].Session["OpNum"])
	require.Equal(t, 4, r.Events[0].Session["Stub Bytes"], "auth padding must not be counted as stub data")
	require.Equal(t, true, r.Events[0].Session["Stub Opaque"])

	response := dcerpcTestAuth(dcerpcResponse(2, 0, []byte{9, 9}), 16, 8, 6)
	r = s.Feed(1, ts, response)
	require.Nil(t, r.Err)
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])
}

func TestProtocolSessionDCERPCBindAckNegotiateResult(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	ts := time.Unix(1, 0)

	bind := dcerpcBindContexts(11, 1,
		dcerpcBindContext{uuid: dcerpcEPM, id: 0},
		dcerpcBindContext{uuid: dcerpcSRVSVC, id: 1},
	)
	require.Nil(t, s.Feed(0, ts, bind).Err)
	ack := dcerpcBindAckResults(12, 1,
		dcerpcBindAckEntry{result: 0, syntax: dcerpcNDR, version: 2},
		dcerpcBindAckEntry{result: 3, syntax: dcerpcNDR, version: 2},
	)
	r := s.Feed(1, ts, ack)
	require.Nil(t, r.Err)
	require.Equal(t, []uint16{0}, r.Events[0].Session["Accepted Context IDs"])
	require.Equal(t, []uint16{1}, r.Events[0].Session["Negotiated Context IDs"])
}

func TestProtocolSessionDCERPCRealSMBExecAuthTraffic(t *testing.T) {
	raw, err := os.ReadFile("../../../testdata/enterprise-traffic-analysis/lateral-movement-psexec/capture.pcapng")
	require.NoError(t, err)
	events, _, err := binReplay(t, raw, 1)
	require.NoError(t, err)

	var dcerpcEvents []*BinParserEvent
	for _, event := range events {
		if event.Protocol == "dcerpc" {
			dcerpcEvents = append(dcerpcEvents, event)
		}
	}
	require.NotEmpty(t, dcerpcEvents)

	var foundAuth3, foundRequest, foundResponse bool
	for _, event := range dcerpcEvents {
		require.Empty(t, event.Error, "DCE/RPC authentication must not stop the stream: %s", event.Error)
		ptype, _ := event.Session["PType"].(byte)
		switch ptype {
		case 16:
			foundAuth3 = true
			require.Equal(t, true, event.Session["Authenticated"])
			ntlm, ok := event.Session["NTLMSSP"].(map[string]any)
			require.True(t, ok, "NTLM Type3 identity fields must be structured")
			require.NotEmpty(t, ntlm["User"])
		case 0:
			if op, ok := event.Session["OpNum"].(uint16); ok && op == 15 {
				foundRequest = true
				require.Equal(t, true, event.Session["Stub Opaque"])
			}
		case 2:
			foundResponse = true
			require.Equal(t, "matched", event.Session["Association Status"])
		}
	}
	require.True(t, foundAuth3, "NTLM AUTH3 must be framed and retained")
	require.True(t, foundRequest, "svcctl request after authenticated bind must be decoded")
	require.True(t, foundResponse, "svcctl response after authenticated bind must be decoded")
}

func TestProtocolSessionDCERPCResponseContextMustMatchRequest(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	ts := time.Unix(1, 0)

	bind := dcerpcBindContexts(11, 1,
		dcerpcBindContext{uuid: dcerpcEPM, id: 0},
		dcerpcBindContext{uuid: dcerpcSRVSVC, id: 1},
	)
	require.Nil(t, s.Feed(0, ts, bind).Err)
	ack := dcerpcBindAckResults(12, 1,
		dcerpcBindAckEntry{result: 0, syntax: dcerpcNDR, version: 2},
		dcerpcBindAckEntry{result: 0, syntax: dcerpcNDR, version: 2},
	)
	require.Nil(t, s.Feed(1, ts, ack).Err)
	require.Nil(t, s.Feed(0, ts, dcerpcRequest(9, 0, 3, nil)).Err)

	r := s.Feed(1, ts, dcerpcResponse(9, 1, nil))
	require.Nil(t, r.Err)
	require.Equal(t, "context-mismatch", r.Events[0].Session["Association Status"])
	require.Equal(t, true, r.Events[0].Session["Unmatched"])
	require.NotContains(t, r.Events[0].Session, "Matched Request")

	r = s.Feed(1, ts, dcerpcResponse(9, 0, nil))
	require.Nil(t, r.Err)
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])
	require.Equal(t, "EPM.ept_map", r.Events[0].Session["Matched Request"])
}

func testT21DCERPCFullSessionReplayMatrix(t *testing.T, pcap []byte, want map[uint64][]string, workers int, deferred bool) {
	t.Helper()
	events, _, err := binReplay(t, pcap, workers, WithBinParserDeferred(deferred))
	require.NoError(t, err)
	got := make(map[uint64]*ProtocolEvent, len(want))
	for _, event := range events {
		if event.Protocol != "dcerpc" || event.Length == 0 {
			continue
		}
		require.Len(t, event.SourceBytes.PacketRefs, 1, "each TShark PDU frame stays independently traceable")
		frame := event.SourceBytes.PacketRefs[0].Number
		if _, exists := want[frame]; !exists {
			continue
		}
		require.Nil(t, got[frame], "duplicate DCE/RPC event for TShark frame %d", frame)
		got[frame] = event
	}
	require.Equal(t, []uint64{4, 5, 6, 7, 8, 9, 10, 11}, sortedPacketFrames(got), "the complete TShark 8-PDU session must survive workers=%d deferred=%t", workers, deferred)

	uuidToInterface := map[string]string{
		"e1af8308-5d1f-11c9-91a4-08002b14a0fa": "EPM",
		"4b324fc8-1670-01d3-1278-5a47bf6ee188": "SRVSVC",
	}
	ackContext := map[string]uint16{"1": 0, "3": 1}
	for frame, row := range want {
		event := got[frame]
		require.NotNil(t, event)
		wantStatus := "decoded"
		if deferred {
			wantStatus = "deferred"
		}
		require.Equal(t, wantStatus, event.Status, "frame=%d workers=%d", frame, workers)
		ptype, parseErr := strconv.ParseUint(row[3], 10, 8)
		require.NoError(t, parseErr)
		require.Equal(t, uint8(ptype), event.Session["PType"], "TShark PType at frame %d", frame)
		require.Equal(t, int64(mustT21Int(t, row[5])), t21Int64(t, event.Session["Call ID"]), "TShark Call ID at frame %d", frame)
		if row[6] != "" {
			require.Equal(t, uint16(mustT21Int(t, row[6])), event.Session["Context ID"], "TShark Context ID at frame %d", frame)
		}
		if row[3] == "0" && row[7] != "" {
			require.Equal(t, uint16(mustT21Int(t, row[7])), event.Session["OpNum"], "TShark operation number at frame %d", frame)
		}
		switch row[3] {
		case "11":
			iface := uuidToInterface[row[13]]
			require.NotEmpty(t, iface)
			require.Equal(t, iface, event.Session["Interface"])
		case "12":
			require.Equal(t, "Bind", event.Session["Matched Request"])
			require.Contains(t, ackContext, row[5])
			require.Equal(t, []uint16{ackContext[row[5]]}, event.Session["Accepted Context IDs"])
		case "0":
			require.Equal(t, row[1], event.Session["Interface"])
			if row[1] == "EPM" {
				require.Equal(t, "EPM.ept_map", event.Session["Packet Name"])
			} else {
				require.Equal(t, "SRVSVC.NetrShareEnum", event.Session["Packet Name"])
			}
		case "2":
			require.Equal(t, "matched", event.Session["Association Status"])
			require.Equal(t, row[1]+"."+map[string]string{"EPM": "ept_map", "SRVSVC": "NetrShareEnum"}[row[1]], event.Session["Matched Request"])
		default:
			t.Fatalf("unexpected TShark DCE/RPC PType %q at frame %d", row[3], frame)
		}
	}
}

func reviewProtocolKind(t *testing.T, err error) ProtocolErrorKind {
	t.Helper()
	var typed *ProtocolError
	require.True(t, errors.As(err, &typed), "expected a typed protocol error, got %v", err)
	return typed.Kind
}
