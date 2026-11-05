package pcaputil

import (
	"bytes"
	"encoding/binary"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Historical-shaped layout observed in the two original official GetInfo
// captures: fixed16/StructureSize17, no modern PreviousSessionId field. The
// opaque bytes are a test marker, not a real security token or authentication.
func smb2UnsupportedSetup() []byte {
	body := make([]byte, 20)
	binary.LittleEndian.PutUint16(body, 17)
	binary.LittleEndian.PutUint16(body[12:], 80)
	binary.LittleEndian.PutUint16(body[14:], 4)
	copy(body[16:], []byte{0xaa, 0xbb, 0xcc, 0xdd})
	return smb2TCP(append(smb2Hdr(1, 0, 99, 0, 0), body...))
}

func TestSMB2BodyRecoveryPreservesErrorSourcesAndPending(t *testing.T) {
	for _, compound := range []bool{false, true} {
		session, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		smb2ChainFeed(t, session, 0, smb2QueryRequest(21, 17, 7, 1, 5, nil))
		state := session.(*captureSession).f.smb2
		before := maps.Clone(state.pending)
		bad := smb2UnsupportedSetup()
		if compound {
			bad = smb2CompoundWire(smb2QueryRequest(22, 17, 7, 1, 7, nil), bad)
		}
		result := session.Feed(0, time.Unix(2, 0), bad)
		require.NotNil(t, result.Err)
		require.Equal(t, ErrUnsupportedVersion, result.Err.Kind)
		require.Len(t, result.Events, 1)
		event := result.Events[0]
		require.Equal(t, "context-required", event.Status)
		require.Equal(t, "message", event.Session["Error Scope"])
		require.Equal(t, false, event.Session["State Updated"])
		require.Equal(t, bad, event.Raw)
		require.Equal(t, before, state.pending, "even a valid earlier command in the bad compound cannot publish pending state")
		require.Empty(t, state.async)
		require.Empty(t, state.files)
		require.Zero(t, state.dialect)
		commands := []map[string]any{event.Session}
		if compound {
			commands = event.Session["Commands"].([]map[string]any)
		}
		for _, command := range commands {
			require.NotContains(t, command, "Authentication")
			require.NotContains(t, command, "Matched Request")
			require.NotContains(t, command, "Request Info Type")
			body := command["Body"].(map[string]any)
			require.NotContains(t, body, "PreviousSessionId")
			rangePair := body["Wire Field Ranges"].(map[string]any)["Unparsed Body"].([]any)
			require.Equal(t, bad[rangePair[0].(uint64):rangePair[1].(uint64)], body["Unparsed Body"])
		}
		info := smb2ChainFeed(t, session, 1, smb2QueryResponse(21, 17, 7, 0, smb2StandardOutput()))
		require.Equal(t, "FileStandardInformation", info["Output Information Type"])
		smb2ChainFeed(t, session, 0, smb2QueryRequest(22, 17, 7, 1, 7, nil))
		info = smb2ChainFeed(t, session, 1, smb2QueryResponse(22, 17, 7, 0, []byte{1, 2, 3, 4}))
		require.Equal(t, "FileEaInformation", info["Output Information Type"])
		session.Close("FIN")
		require.Zero(t, session.Stats().BufferedBytes)
		// The canonical modern entry refuses this old layout too. Its security
		// bytes must not become a successful modern PreviousSessionId leaf.
		_, err = parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(smb2UnsupportedSetup()[4:]), bits: uint64(len(smb2UnsupportedSetup())-4) * 8}, "application-layer.smb2", "SMB2")
		require.Error(t, err)
	}
}

func TestSMB2BodyRecoveryDoesNotRecoverFramingHeadersStateOrBudgets(t *testing.T) {
	for _, change := range []string{"magic", "header-size", "next-alignment", "next-outside", "later-magic", "later-header-size", "mixed-direction", "too-many-commands", "association-conflict", "resource-error"} {
		t.Run(change, func(t *testing.T) {
			wire := smb2UnsupportedSetup()
			max := 64
			switch change {
			case "magic":
				wire[4] ^= 1
			case "header-size":
				binary.LittleEndian.PutUint16(wire[8:], 63)
			case "next-alignment":
				binary.LittleEndian.PutUint32(wire[24:], 65)
			case "next-outside":
				binary.LittleEndian.PutUint32(wire[24:], 4096)
			case "later-magic", "later-header-size", "mixed-direction", "too-many-commands":
				wire = smb2CompoundWire(wire, smb2QueryRequest(22, 17, 7, 1, 7, nil))
				at := 4 + int(binary.LittleEndian.Uint32(wire[24:]))
				if change == "later-magic" {
					wire[at] ^= 1
				}
				if change == "later-header-size" {
					binary.LittleEndian.PutUint16(wire[at+4:], 63)
				}
				if change == "mixed-direction" {
					binary.LittleEndian.PutUint32(wire[at+16:], smb2FlagResp)
				}
				if change == "too-many-commands" {
					max = 1
				}
			}
			state := &binSMB2{client: 0}
			_, err := state.consumeFrom(0, wire, max)
			if change == "association-conflict" {
				wire = smb2QueryRequest(21, 17, 7, 1, 5, nil)
				_, err = state.consumeFrom(0, wire, max)
				require.NoError(t, err)
				_, err = state.consumeFrom(0, wire, max)
			} else if change == "resource-error" {
				err = &smb2BodyDecodeError{cause: protocolError(ErrResourceExceeded, "body collection limit")}
			}
			require.Error(t, err)
			require.Nil(t, smb2RecoverBodyRecord(wire, max, err), "body marker alone cannot authorize a bad enclosing record")
		})
	}
}

func TestSMB2BodyRecoveryCoalescedAndSplitRecordsAcrossWorkers(t *testing.T) {
	bad := smb2UnsupportedSetup()
	request := smb2QueryRequest(21, 17, 7, 1, 7, nil)
	response := smb2QueryResponse(21, 17, 7, 0, []byte{41, 0, 0, 0})
	for _, workers := range []int{1, 3} {
		for _, deferred := range []bool{false, true} {
			for _, split := range []bool{false, true} {
				wire := append(bytes.Clone(bad), request...)
				steps := []tcpStep{{seq: 99, syn: true}, {seq: 999, syn: true, reverse: true}}
				if split {
					steps = append(steps, tcpStep{seq: 100, data: string(wire[:7])}, tcpStep{seq: 107, data: string(wire[7 : len(bad)+64])}, tcpStep{seq: 100 + uint32(len(bad)+64), data: string(wire[len(bad)+64:])})
				} else {
					steps = append(steps, tcpStep{seq: 100, data: string(wire)})
				}
				steps = append(steps, tcpStep{seq: 1000, data: string(response), reverse: true}, tcpStep{seq: 100 + uint32(len(wire)), fin: true})
				events, _, err := binReplay(t, binTestPcap(t, steps, 445, false, true), workers, WithBinParserDeferred(deferred))
				require.NoError(t, err)
				require.Len(t, events, 3)
				require.Equal(t, "context-required", events[0].Status)
				require.Equal(t, bad, events[0].Raw)
				require.Equal(t, request, events[1].Raw)
				require.Equal(t, response, events[2].Raw)
				require.Equal(t, "FileEaInformation", events[2].Session["Output Information Type"])
				require.Equal(t, events[0].FlowID, events[2].FlowID)
			}
		}
	}
	// A declared record that has not arrived completely cannot scan ahead to a
	// signature inside the retained bytes and pretend that it reached a boundary.
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	smb2ChainFeed(t, session, 0, request)
	declared := append(bytes.Clone(bad), request...)
	declared[1], declared[2], declared[3] = 0, 2, 0
	result := session.Feed(0, time.Unix(2, 0), declared)
	require.Empty(t, result.Events)
	require.True(t, result.NeedMore)
	session.Close("FIN")
	require.Zero(t, session.Stats().BufferedBytes)
}

func TestSMB2BodyRecoveryCannotHidePendingConflict(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	request := smb2QueryRequest(21, 17, 7, 1, 5, nil)
	smb2ChainFeed(t, session, 0, request)
	result := session.Feed(0, time.Unix(2, 0), smb2CompoundWire(request, smb2UnsupportedSetup()))
	require.Len(t, result.Events, 1)
	require.NotNil(t, result.Err)
	require.NotEqual(t, "message", result.Events[0].Session["Error Scope"], "a later body failure cannot mask a header-visible pending conflict")
	result = session.Feed(1, time.Unix(3, 0), smb2QueryResponse(21, 17, 7, 0, smb2StandardOutput()))
	for _, event := range result.Events {
		require.NotContains(t, event.Session, "Output Information Type")
	}
}

func TestSMB2BodyRecoveryCannotDowngradeRealCodecResourceLimit(t *testing.T) {
	session, err := NewProtocolSession(ParserBudget{MaxFrameBytes: 2 << 20, MaxMessageBytes: 2 << 20, MaxBufferedBytes: 8 << 20, MaxCollectionElements: 64})
	require.NoError(t, err)
	defer session.Close("test")
	smb2ChainFeed(t, session, 0, smb2QueryRequest(21, 17, 7, 1, 5, nil))
	body := make([]byte, stream_parser.SMB2QueryInfoMaxBodyBytes+1)
	binary.LittleEndian.PutUint16(body, 9)
	bad := smb2TCP(append(smb2Hdr(16, smb2FlagResp, 21, 17, 7), body...))
	result := session.Feed(1, time.Unix(2, 0), bad)
	require.NotNil(t, result.Err)
	require.Equal(t, ErrResourceExceeded, result.Err.Kind)
	require.Len(t, result.Events, 1)
	require.Equal(t, "limited", result.Events[0].Status)
	require.NotEqual(t, "message", result.Events[0].Session["Error Scope"])
	result = session.Feed(1, time.Unix(3, 0), smb2QueryResponse(21, 17, 7, 0, smb2StandardOutput()))
	for _, event := range result.Events {
		require.NotContains(t, event.Session, "Output Information Type")
	}
	session.Close("FIN")
	require.Zero(t, session.Stats().BufferedBytes)
}

func TestSMB2SessionSetupSuccessContinuationAndBoundedError(t *testing.T) {
	for _, status := range []uint32{0, 0xc0000016, 0xc0000022} {
		state := &binSMB2{client: 0}
		request := smb2SessionSetup(99, false)
		_, err := state.consumeFrom(0, request, 64)
		require.NoError(t, err)
		response := smb2SessionSetup(99, true)
		binary.LittleEndian.PutUint32(response[12:], status)
		if status == 0xc0000022 {
			response = smb2ErrorResponse(1, 99, 17, 0, status, []byte{0xaa, 0xbb})
		}
		for _, change := range []string{"short", "structure", "byte-count"} {
			bad := bytes.Clone(response)
			if change == "short" {
				bad = smb2TCP(bad[4:75])
			}
			if change == "structure" {
				binary.LittleEndian.PutUint16(bad[68:], 8)
			}
			if change == "byte-count" {
				if status != 0xc0000022 {
					continue
				}
				binary.LittleEndian.PutUint32(bad[72:], 0xffffffff)
			}
			_, err = state.consumeFrom(1, bad, 64)
			require.Error(t, err)
			require.Len(t, state.pending, 1, "incomplete success/continuation/ERROR cannot retire the request")
			_, err = parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(bad[4:]), bits: uint64(len(bad)-4) * 8}, "application-layer.smb2", "SMB2")
			require.Error(t, err)
		}
		info, err := state.consumeFrom(1, response, 64)
		require.NoError(t, err)
		require.Empty(t, state.pending)
		_, err = parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(response[4:]), bits: uint64(len(response)-4) * 8}, "application-layer.smb2", "SMB2")
		require.NoError(t, err)
		if status == 0xc0000022 {
			require.Equal(t, true, info["Error Response"])
			require.NotContains(t, info, "Guest Session")
			require.NotContains(t, info, "Authentication")
			body := info["Body"].(map[string]any)
			require.Equal(t, []byte{0xaa, 0xbb}, body["Error Data"])
			require.Equal(t, []any{uint64(76), uint64(78)}, body["Wire Field Ranges"].(map[string]any)["Error Data"])
		}
	}
}

func TestSMB2BodyRecoveryPrivateRefusalSurvivesPublicEdits(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	result := session.Feed(0, time.Unix(1, 0), smb2UnsupportedSetup())
	require.Len(t, result.Events, 1)
	event := result.Events[0]
	require.Equal(t, "context-required", event.Status)
	event.Error, event.Status = "", "decoded"
	event.Fields = map[string]any{"Authentication": map[string]any{"Verified": true}}
	event.Structured = map[string]any{"fields": event.Fields}
	event.Session = map[string]any{"Output Information Type": "FileStandardInformation", "Error Scope": "connection"}
	clear(event.Raw)
	_, err = event.GetFields()
	require.Error(t, err)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrUnsupportedVersion, typed.Kind)
}

func TestSMB2PendingAsyncIdentityCannotBorrowAnotherMessage(t *testing.T) {
	state := &binSMB2{client: 0}
	for _, request := range [][]byte{smb2QueryRequest(21, 17, 7, 1, 5, nil), smb2QueryRequest(22, 17, 7, 1, 7, nil)} {
		_, err := state.consumeFrom(0, request, 64)
		require.NoError(t, err)
	}
	pending := func(mid, async uint64) []byte {
		return smb2AsyncWire(smb2ErrorResponse(16, mid, 17, 0, 0x103, nil), async)
	}
	// A later malformed body cannot hide two new aliases colliding inside
	// this record, even when no alias existed before the record arrived.
	badBody := smb2QueryResponse(33, 17, 7, 0, nil)
	binary.LittleEndian.PutUint16(badBody[68:], 8)
	within := smb2CompoundWire(pending(21, 99), pending(22, 99), badBody)
	_, err := state.consumeFrom(1, within, 64)
	require.Error(t, err)
	withinDiagnostic := smb2RecoverBodyRecord(within, 64, err)
	require.NotNil(t, withinDiagnostic)
	require.False(t, state.bodyRecoveryIdentitySafe(withinDiagnostic, 1))
	require.Empty(t, state.async)
	withinReassignment := smb2CompoundWire(pending(21, 99), pending(21, 100), badBody)
	_, err = state.consumeFrom(1, withinReassignment, 64)
	require.Error(t, err)
	withinDiagnostic = smb2RecoverBodyRecord(withinReassignment, 64, err)
	require.NotNil(t, withinDiagnostic)
	require.False(t, state.bodyRecoveryIdentitySafe(withinDiagnostic, 1))
	_, err = state.consumeFrom(1, pending(21, 99), 64)
	require.NoError(t, err)
	before := maps.Clone(state.pending)
	aliases := maps.Clone(state.async)
	_, err = state.consumeFrom(1, pending(21, 99), 64)
	require.NoError(t, err, "the same observed pending identity is idempotent")
	_, err = state.consumeFrom(1, pending(21, 100), 64)
	require.ErrorContains(t, err, "changed its AsyncId")
	require.Equal(t, before, state.pending)
	require.Equal(t, aliases, state.async)
	_, err = state.consumeFrom(1, pending(22, 99), 64)
	require.ErrorContains(t, err, "AsyncId collision")
	require.Equal(t, before, state.pending)
	require.Equal(t, aliases, state.async)
	// A bad later body must not hide this header-visible collision either.
	bad := smb2QueryResponse(33, 17, 7, 0, nil)
	binary.LittleEndian.PutUint16(bad[68:], 8)
	wire := smb2CompoundWire(pending(22, 99), bad)
	_, err = state.consumeFrom(1, wire, 64)
	require.Error(t, err)
	diagnostic := smb2RecoverBodyRecord(wire, 64, err)
	require.NotNil(t, diagnostic, "the enclosing headers are complete")
	require.False(t, state.bodyRecoveryIdentitySafe(diagnostic, 1))
	info, err := state.consumeFrom(1, smb2AsyncWire(smb2QueryResponse(^uint64(0), 17, 0, 0, smb2StandardOutput()), 99), 64)
	require.NoError(t, err)
	require.Equal(t, uint64(21), info["Request Message ID"])
	require.Equal(t, "FileStandardInformation", info["Output Information Type"])
	_, err = state.consumeFrom(1, pending(22, 100), 64)
	require.NoError(t, err)
	info, err = state.consumeFrom(1, smb2AsyncWire(smb2QueryResponse(^uint64(0), 17, 0, 0, []byte{41, 0, 0, 0}), 100), 64)
	require.NoError(t, err)
	require.Equal(t, uint64(22), info["Request Message ID"])
	require.Equal(t, "FileEaInformation", info["Output Information Type"])
	require.Empty(t, state.pending)
	require.Empty(t, state.async)
	// Reusing a completed MID cannot resurrect the retired AsyncId mapping.
	_, err = state.consumeFrom(0, smb2QueryRequest(21, 17, 7, 1, 7, nil), 64)
	require.NoError(t, err)
	late := smb2AsyncWire(smb2QueryResponse(^uint64(0), 17, 0, 0, []byte{41, 0, 0, 0}), 99)
	info, err = state.consumeFrom(1, late, 64)
	require.NoError(t, err)
	require.NotContains(t, info, "Output Information Type")
	require.Len(t, state.pending, 1)
	info, err = state.consumeFrom(1, smb2QueryResponse(21, 17, 7, 0, []byte{41, 0, 0, 0}), 64)
	require.NoError(t, err)
	require.Equal(t, "FileEaInformation", info["Output Information Type"])
	require.Empty(t, state.pending)
}

func TestSMB2AsyncHeaderRequiresNonzeroOwnedIdentifier(t *testing.T) {
	for _, id := range []uint64{0, 1, ^uint64(0)} {
		wire := smb2AsyncWire(smb2QueryResponse(21, 17, 0, 0, nil), id)
		_, nativeErr := (&binSMB2{}).consumeFrom(1, wire, 64)
		_, canonicalErr := parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(wire[4:]), bits: uint64(len(wire)-4) * 8}, "application-layer.smb2", "SMB2")
		if id == 0 {
			require.Error(t, nativeErr)
			require.Error(t, canonicalErr)
		} else {
			require.NoError(t, nativeErr)
			require.NoError(t, canonicalErr)
		}
	}
}
