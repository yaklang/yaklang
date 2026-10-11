package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func mediaReviewRTCPRR(reportBlocks int) []byte {
	wire := make([]byte, 8+24*reportBlocks)
	wire[0] = 0x80 | byte(reportBlocks)
	wire[1] = 201
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)/4-1))
	return wire
}

func mediaReviewRTCPBYE(sources int) []byte {
	wire := make([]byte, 4+4*sources)
	wire[0] = 0x80 | byte(sources)
	wire[1] = 203
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)/4-1))
	return wire
}

func requireResourceExceeded(t *testing.T, err error) {
	t.Helper()
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, ErrResourceExceeded, pe.Kind)
}

func TestProtocolSessionRTCPCollectionBudgets(t *testing.T) {
	rtp := &binRTP{sources: map[uint32]*rtpSource{}}
	_, err := rtp.consumeRTCPBounded(mediaReviewRTCPRR(2), 1)
	requireResourceExceeded(t, err)

	_, err = rtp.consumeRTCPBounded(mediaReviewRTCPBYE(2), 1)
	requireResourceExceeded(t, err)

	compound := append(rtcpSR(1, 1, 2, 3, 4), rtcpSR(5, 6, 7, 8, 9)...)
	_, err = rtp.consumeRTCPCompound(compound, 1)
	requireResourceExceeded(t, err)
}

func TestProtocolSessionRTPInstalledReservationCallbackChargesSourceHistory(t *testing.T) {
	const sourceStateBytes = int64(512 + 96 + 128*4)
	reserved := int64(0)
	sources := map[uint32]*rtpSource{}
	rtp := &binRTP{
		sources:          sources,
		maxBufferedBytes: int(sourceStateBytes),
		reserveSessionMemory: func(target int64) error {
			require.Empty(t, sources, "source history must be reserved before the source is allocated")
			reserved = target
			return nil
		},
	}

	_, err := rtp.consumeRTP(rtpPkt(96, 1, 0, 0x12345678), time.Unix(1, 0), 8)
	require.NoError(t, err)
	require.Equal(t, sourceStateBytes, reserved)
	require.Len(t, sources[0x12345678].recent, 1)
	require.Equal(t, 128, cap(sources[0x12345678].recent))
}

func TestProtocolSessionRTPAddsSDPMapsWithinRetainedMemoryBudget(t *testing.T) {
	s, err := NewProtocolSession(ParserBudget{MaxMessageBytes: 64, MaxBufferedBytes: 1200})
	require.NoError(t, err)
	parser := s.(*captureSession).f.a

	const (
		ssrc = uint32(0x12345678)
		src  = "192.0.2.10:5004"
		dst  = "192.0.2.20:5004"
	)
	ts := time.Unix(1, 0)
	first := &ProtocolEvent{Source: src, Destination: dst, Timestamp: ts}
	require.True(t, parser.decodeRTPDatagram(first, rtpPkt(96, 1, 0, ssrc), false, false, sipMediaMatch{}))
	require.Nil(t, first.sessionError, "%v", first.sessionError)
	require.EqualValues(t, 512+96+128*4, parser.buffered.Load(), "the pre-SDP source and its history fit the budget")

	var flow *binFlow
	for _, el := range parser.udpSessions.entries {
		flow = el.Value.(*binUDPEntry).flow
		break
	}
	require.NotNil(t, flow)
	require.EqualValues(t, 1, flow.rtp.sources[ssrc].received)

	second := &ProtocolEvent{Source: src, Destination: dst, Timestamp: ts.Add(time.Second)}
	match := sipMediaMatch{status: "matched", payload: 96, encoding: "opus", clockRate: 48000}
	require.True(t, parser.decodeRTPDatagram(second, rtpPkt(96, 2, 480, ssrc), false, false, match))
	require.NotNil(t, second.sessionError)
	requireResourceExceeded(t, second.sessionError)
	require.Nil(t, flow.rtp.payload, "a rejected map growth must not allocate or retain the payload map")
	require.Nil(t, flow.rtp.clocks, "a rejected map growth must not allocate or retain the clock map")
	require.EqualValues(t, 1, flow.rtp.sources[ssrc].received, "the rejected configuration does not consume RTP history")
	require.EqualValues(t, 512+96+128*4, parser.buffered.Load(), "the shared reservation stays at the pre-SDP high-water mark")
}

// RFC3550 section6.6 supplies the literal BYE source list. This is a byte
// observation through the existing stream API, not a claim that an isolated
// BYE is a compliant compound exchange or that a source is an authenticated peer.
func rtcpBYEHistorySourcesOwnership(t *testing.T) {
	t.Helper()
	rtcpSourceHistoryBudget(t)
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("test")
	wire := mediaReviewRTCPBYE(1)
	binary.BigEndian.PutUint32(wire[4:8], 0x11223344)
	r := s.Feed(0, time.Unix(1, 0), wire)
	clear(wire)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	expected := map[string]any{"Packet Name": "BYE", "Version": 2, "Packet Type": byte(203), "SSRC": uint32(0x11223344), "Report Count": 1, "Context Level": "observed", "Network Loss": false, "Loss Kind": "none", "Sources": []uint32{0x11223344}}
	require.Equal(t, expected, e.Session)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	require.Equal(t, []byte{0x81, 0xcb, 0, 1, 0x11, 0x22, 0x33, 0x44}, e.Raw)
	history, err := NewBinParserInspector(1, 1<<20)
	require.NoError(t, err)
	history.OnEvent(e)
	e.Session["Sources"].([]uint32)[0] = 999999
	testOwnedSessionQueries(t, e, expected)
	detail, err := history.Details(e.ID)
	require.NoError(t, err)
	require.Equal(t, expected, detail.Session)
	detail.Session["Sources"].([]uint32)[0] = 999999
	decoded, err := detail.Decode()
	require.NoError(t, err)
	decoded["session"].(map[string]any)["Sources"].([]uint32)[0] = 999999
	again, err := history.Details(e.ID)
	require.NoError(t, err)
	require.Equal(t, expected, again.Session)
	s.Close("test")
	s.Close("again")
	require.Zero(t, s.Stats().BufferedBytes)
}

func rtcpSourceHistoryBudget(t *testing.T) {
	t.Helper()
	// Logical charge64 + key7 + slot32 + slice24 + three32-bit sources12 =139.
	e := &ProtocolEvent{ID: 1, Protocol: "rtp", Session: map[string]any{"Sources": []uint32{1, 2, 3}}}
	for _, limit := range []int{138, 139} {
		history, err := NewBinParserInspector(1, limit)
		require.NoError(t, err)
		history.OnEvent(e)
		if limit == 138 {
			require.Empty(t, history.Rows("rtp", 0))
			require.EqualValues(t, 1, history.Evicted())
		} else {
			require.Len(t, history.Rows("rtp", 0), 1)
			detail, err := history.Details(1)
			require.NoError(t, err)
			require.Equal(t, e.Session, detail.Session)
			require.Zero(t, history.Evicted())
		}
	}
}
func TestProtocolInspectorRTCPSourceVectorBudget(t *testing.T) {
	rtcpSourceHistoryBudget(t)
}

type mediaOwnedQueryControl struct {
	ID       string            `json:"id"`
	Input    string            `json:"input"`
	SHA256   string            `json:"sha256"`
	WireHex  string            `json:"wire_hex"`
	Fields   map[string]any    `json:"expected_fields"`
	Session  map[string]any    `json:"expected_session"`
	Error    ProtocolErrorKind `json:"expected_error"`
	Buffered int               `json:"buffered"`
}

func mediaOwnedQueryCase(t *testing.T, id string) (mediaOwnedQueryControl, []byte, []byte) {
	t.Helper()
	answer, err := trafficfixture.ReadFile("testdata/media-owned-queries/cases.json")
	require.NoError(t, err)
	var inventory struct {
		Version int                      `json:"version"`
		Cases   []mediaOwnedQueryControl `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(answer, &inventory))
	require.Equal(t, 1, inventory.Version)
	require.Len(t, inventory.Cases, 3)
	seen := map[string]bool{}
	var selected mediaOwnedQueryControl
	for _, c := range inventory.Cases {
		require.False(t, seen[c.ID], "duplicate case")
		seen[c.ID] = true
		if c.ID == id {
			selected = c
		}
	}
	require.Equal(t, id, selected.ID)
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err, "new captures must remain usable by the shared frozen corpus gates")
	bound := false
	for _, inventory := range inventories {
		for _, c := range inventory.Cases {
			if c.Input.OriginalPath == "common/pcapx/pcaputil/testdata/media-owned-queries/"+selected.Input {
				require.False(t, bound)
				bound = true
				require.Equal(t, selected.SHA256, c.Input.SHA256)
				require.Equal(t, 1, c.Facts.PacketCount)
			}
		}
	}
	require.True(t, bound, "each control capture must have a unified answer binding")
	raw, err := trafficfixture.ReadFile("testdata/media-owned-queries/" + selected.Input)
	require.NoError(t, err)
	require.Equal(t, selected.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	wire, err := hex.DecodeString(selected.WireHex)
	require.NoError(t, err)
	return selected, raw, wire
}

func rtcpNativeQueryOwnership(t *testing.T) {
	t.Helper()
	// RFC3550 section6.6 BYE SC1 and SSRC11223344; the existing independent
	// TShark answer validates the eight wire bytes. UDP context remains selected,
	// not authenticated or associated with an observed SDP exchange.
	manual := &ProtocolEvent{Protocol: "rtp", Transport: "udp", Profile: "rtp-udp", Fields: map[string]any{"caller": "manual field cache"}}
	manualFields, err := manual.GetFields()
	require.NoError(t, err)
	require.Equal(t, manual.Fields, manualFields, "manually constructed events retain their public field-cache contract")
	c, raw, wire := mediaOwnedQueryCase(t, "bye")
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		events, stats := discoveryReplay(t, raw, workers, deferred, observe, WithProtocolDecodeAs("udp", 5005, "rtp"))
		require.Len(t, events, 1)
		require.Zero(t, stats.BufferedBytes)
		e := events[0]
		want := c.Session
		rocEqualFields(t, want, e.Session)
		require.NotZero(t, e.ID)
		require.Equal(t, wire, e.Raw)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		require.Equal(t, "192.0.2.1:5005", e.Source)
		require.Equal(t, "192.0.2.2:6005", e.Destination)
		testExactObservationSessionQueries(t, e, want)
		e.Fields = map[string]any{"Packet Name": "caller-forged"}
		e.Structured = map[string]any{"fields": e.Fields}
		fields, err := e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, c.Fields, fields)
	})
}

func rtcpNativeRefusalQueries(t *testing.T) {
	t.Helper()
	for _, id := range []string{"count-width-mismatch", "media-flow-budget"} {
		t.Run(id, func(t *testing.T) {
			c, raw, wire := mediaOwnedQueryCase(t, id)
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe, WithProtocolDecodeAs("udp", 5005, "rtp"), WithProtocolBudget(64, c.Buffered))
				require.Len(t, events, 1)
				require.Zero(t, stats.BufferedBytes)
				e := events[0]
				require.Equal(t, wire, e.Raw)
				require.Nil(t, e.Session)
				require.Equal(t, string(c.Error), e.ExpertCode)
				initial, err := e.Decode()
				require.Nil(t, initial)
				var original *ProtocolError
				require.ErrorAs(t, err, &original)
				require.Equal(t, c.Error, original.Kind)
				message := original.Message
				require.NotEmpty(t, message)
				id, flow, response, transaction := e.ID, e.FlowID, e.ResponseTo, e.TransactionID
				require.Zero(t, response)
				require.Zero(t, transaction)
				e.Status, e.Error = "decoded", ""
				e.Session = map[string]any{"Association Status": "verified"}
				e.Fields = map[string]any{"caller-forged": true}
				e.Structured = map[string]any{"fields": e.Fields}
				for i := 0; i < 2; i++ {
					result, err := e.Decode()
					require.Nil(t, result)
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, c.Error, typed.Kind)
					require.Equal(t, message, typed.Message)
					typed.Kind, typed.Message = ErrFatalSessionError, "caller-forged"
					fields, err := e.GetFields()
					require.Nil(t, fields)
					require.ErrorAs(t, err, &typed)
					require.Equal(t, c.Error, typed.Kind)
					require.Equal(t, message, typed.Message)
					require.Equal(t, id, e.ID)
					require.Equal(t, flow, e.FlowID)
					require.Equal(t, response, e.ResponseTo)
					require.Equal(t, transaction, e.TransactionID)
				}
			})
		})
	}
}

func TestRTCPNativeOwnedQueries(t *testing.T)  { rtcpNativeQueryOwnership(t) }
func TestRTCPNativeOwnedRefusals(t *testing.T) { rtcpNativeRefusalQueries(t) }
