package pcaputil

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func wrapperNormalAccessHistoricalFields(t *testing.T, old wrapperControl) []map[string]any {
	raw, err := trafficfixture.ReadFile("dlms-normal-access/answers/historical-get-selective-access.json")
	require.NoError(t, err)
	var c wrapperBlockControl
	require.NoError(t, json.Unmarshal(raw, &c))
	require.Equal(t, old.SHA256, c.SHA)
	require.Len(t, old.CompleteFrames, 1)
	require.Len(t, c.Answers, 1)
	require.Equal(t, old.CompleteFrames[0], c.Answers[0].Wire)
	require.Nil(t, c.Answers[0].Error)
	return []map[string]any{c.Answers[0].Fields}
}

func TestDLMSWrapperNormalAccessExistingAPIBaseline(t *testing.T) {
	for _, selected := range []bool{false, true} {
		name := "adjacent-unselected"
		wire := "000100100001000dc001c1000100002a0000ff0200"
		if selected {
			name = "selected-structure"
			wire = "0001001000010016c001c1000100002a0000ff0201020202120001120002"
		}
		t.Run(name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("baseline")
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, wire))
			require.Nil(t, q.Err)
			require.Len(t, q.Events, 1)
			f, err := q.Events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, selected, f["selective_access"])
			require.Equal(t, "GetRequestNormal", f["kind"])
			require.EqualValues(t, 1, f["class_id"])
			require.Equal(t, "0.0.42.0.0.255", f["logical_name"])
			require.EqualValues(t, 2, f["attribute_id"])
			if selected {
				require.EqualValues(t, 1, f["access_selection_raw"])
				require.EqualValues(t, 2, f["access_selector"])
				require.Equal(t, false, f["selector_semantics_verified"])
				rocEqualFields(t, map[string]any{"type": float64(2), "raw_hex": "0202120001120002", "length": float64(2), "length_encoding_hex": "02", "elements": []any{map[string]any{"type": float64(18), "raw_hex": "120001", "value": float64(1)}, map[string]any{"type": float64(18), "raw_hex": "120002", "value": float64(2)}}}, f["access_parameters"].(map[string]any))
			} else {
				require.NotContains(t, f, "access_parameters")
			}
			response := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "0001000100100007c401c100120100"))
			require.Nil(t, response.Err)
			require.Len(t, response.Events, 1)
			e := response.Events[0]
			rf, err := e.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, map[string]any{"type": float64(18), "raw_hex": "120100", "value": float64(256)}, rf["data"].(map[string]any))
			require.Equal(t, q.Events[0].ID, e.ResponseTo)
			require.Equal(t, q.Events[0].ID, e.TransactionID)
			require.Equal(t, "observed-response", e.Session["Association"])
			require.Equal(t, 0, e.Session["Outstanding"])
			require.Empty(t, s.Close("baseline"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSWrapperNormalAccessSealedMatrix(t *testing.T) {
	wrapperBlocksReplay(t, wrapperBlockControlsFrom(t, "dlms-normal-access", 31))
}
func TestDLMSWrapperNormalAccessBudgetsOwnership(t *testing.T) {
	wrapperBlocksOwnership(t, wrapperBlockControlsFrom(t, "dlms-normal-access", 31))
}

func TestDLMSWrapperNormalAccessProjectionBoundary(t *testing.T) {
	q := wrapperWire(t, "000100100001000dc001c1000100002a0000ff0200")
	selected := wrapperWire(t, "0001001000010016c001c1000100002a0000ff0201020202120001120002")
	need := 512 + 256 + 32768 + 512*len(selected) + 2048*256 + 256
	for _, delta := range []int{-1, 0} {
		for _, deferred := range []bool{false, true} {
			b := DefaultParserBudget()
			b.MaxFrameBytes, b.MaxMessageBytes = 4096, 4096
			b.MaxBufferedBytes = need + delta
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			old := s.Feed(0, time.Unix(1, 0), q)
			require.Nil(t, old.Err)
			require.Len(t, old.Events, 1)
			out := s.Feed(0, time.Unix(2, 0), selected)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			f, err := e.GetFields()
			if delta == 0 {
				require.NoError(t, err)
				require.Nil(t, out.Err)
				require.Equal(t, true, f["selective_access"])
				require.Equal(t, "ambiguous-invoke", e.Session["Association"])
				require.Equal(t, 0, e.Session["Outstanding"])
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
			} else {
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				require.Nil(t, e.Raw)
				require.Nil(t, e.Session)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
			}
			late := s.Feed(1, time.Unix(3, 0), wrapperWire(t, "0001000100100007c401c100120100"))
			require.Nil(t, late.Err)
			require.Len(t, late.Events, 1)
			require.Zero(t, late.Events[0].ResponseTo)
			require.Zero(t, late.Events[0].TransactionID)
			want := "ambiguous-invoke"
			if delta < 0 {
				want = "ambiguous-conversation"
			}
			require.Equal(t, want, late.Events[0].Session["Association"])
			require.Equal(t, 0, late.Events[0].Session["Outstanding"])
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			require.Empty(t, s.Close("projection"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
	for _, depth := range []int{3, 4} {
		b := DefaultParserBudget()
		b.MaxRecursionDepth = depth
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		out := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "000100100001000fc001c1000100002a0000ff02010100"))
		require.Len(t, out.Events, 1)
		f, err := out.Events[0].GetFields()
		if depth == 4 {
			require.NoError(t, err)
			require.Nil(t, out.Err)
			rocEqualFields(t, map[string]any{"type": float64(0), "raw_hex": "00", "value": nil}, f["access_parameters"].(map[string]any))
			require.Equal(t, "observed-request", out.Events[0].Session["Association"])
			require.Equal(t, 1, out.Events[0].Session["Outstanding"])
			require.Equal(t, out.Events[0].ID, out.Events[0].TransactionID)
			require.Zero(t, out.Events[0].ResponseTo)
			require.Empty(t, s.Close("depth"))
		} else {
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, f)
			require.Nil(t, out.Events[0].Session)
			require.Zero(t, out.Events[0].ResponseTo)
			require.Zero(t, out.Events[0].TransactionID)
			require.Empty(t, s.Close("depth"))
		}
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}
