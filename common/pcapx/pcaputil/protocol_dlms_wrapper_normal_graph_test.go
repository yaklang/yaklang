package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDLMSWrapperNormalGraphExistingAPIBaseline(t *testing.T) {
	for _, c := range []struct {
		name, wire string
		data       map[string]any
	}{
		{"adjacent-bit-string", "0001000100100007c401c1000403a0", map[string]any{"type": float64(4), "raw_hex": "0403a0", "bit_length": float64(3), "length_encoding_hex": "03", "value_hex": "a0", "unused_bits": float64(5)}},
		{"array", "000100010010000cc401c10001021201000901ff", map[string]any{"type": float64(1), "raw_hex": "01021201000901ff", "length": float64(2), "length_encoding_hex": "02", "elements": []any{map[string]any{"type": float64(18), "raw_hex": "120100", "value": float64(256)}, map[string]any{"type": float64(9), "raw_hex": "0901ff", "length": float64(1), "length_encoding_hex": "01", "value_hex": "ff"}}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("baseline")
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "000100100001000dc001c1000100002a0000ff0200"))
			require.Nil(t, q.Err)
			require.Len(t, q.Events, 1)
			r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, c.wire))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			f, err := r.Events[0].GetFields()
			require.NoError(t, err)
			data, ok := f["data"].(map[string]any)
			require.True(t, ok)
			rocEqualFields(t, c.data, data)
			require.Equal(t, q.Events[0].ID, r.Events[0].TransactionID)
			require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
			require.Equal(t, "observed-response", r.Events[0].Session["Association"])
			require.Equal(t, 0, r.Events[0].Session["Outstanding"])
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func wrapperNormalExtendedControls(t *testing.T) []wrapperBlockControl {
	old := wrapperBlockControlsFrom(t, "dlms-normal-data", 29)
	graph := wrapperBlockControlsFrom(t, "dlms-normal-graph", 24)
	// The original array boundary was a missing-capability refusal, not a wire
	// malformation. Preserve its immutable answer, but validate the now-supported
	// exact same capture against this batch's independent complete answer.
	updated := 0
	for i := range old {
		if old[i].Name != "normal-array-boundary" {
			continue
		}
		for _, c := range graph {
			if c.Name != "normal-graph-empty-array" {
				continue
			}
			require.Equal(t, old[i].SHA, c.SHA)
			require.Equal(t, old[i].Transport, c.Transport)
			require.Equal(t, old[i].Steps, c.Steps)
			old[i].Answers = c.Answers
			updated++
		}
	}
	require.Equal(t, 1, updated)
	return old
}

func TestDLMSWrapperNormalGraphSealedMatrix(t *testing.T) {
	wrapperBlocksReplay(t, wrapperBlockControlsFrom(t, "dlms-normal-graph", 24))
}

func TestDLMSWrapperNormalGraphBudgetsOwnership(t *testing.T) {
	wrapperBlocksOwnership(t, wrapperBlockControlsFrom(t, "dlms-normal-graph", 24))
	for _, depth := range []int{3, 4} {
		b := DefaultParserBudget()
		b.MaxRecursionDepth = depth
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "000100100001000dc001c1000100002a0000ff0200"))
		require.Nil(t, q.Err)
		require.Len(t, q.Events, 1)
		r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "0001000100100006c401c1000100"))
		require.Len(t, r.Events, 1)
		fields, err := r.Events[0].GetFields()
		if depth == 4 {
			require.NoError(t, err)
			require.Nil(t, r.Err)
			rocEqualFields(t, map[string]any{"type": float64(1), "raw_hex": "0100", "length": float64(0), "length_encoding_hex": "00", "elements": []any{}}, fields["data"].(map[string]any))
			require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
			require.Equal(t, q.Events[0].ID, r.Events[0].TransactionID)
		} else {
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, fields)
			require.Nil(t, r.Events[0].Session)
			require.Zero(t, r.Events[0].ResponseTo)
			require.Zero(t, r.Events[0].TransactionID)
		}
		require.Empty(t, s.Close("depth"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestDLMSWrapperNormalGraphProjectionBoundary(t *testing.T) {
	q := wrapperWire(t, "000100100001000dc001c1000100002a0000ff0200")
	w := wrapperWire(t, "000100010010000cc401c10001021201000901ff")
	// Conversation, live invoke, owned graph and reservation slack. The selected
	// graph has a fixed node cap256 and wire byte projections, independent of
	// how many children happen to be present in this one valid response.
	need := 512 + 256 + 32768 + 512*len(w) + 2048*256 + 256
	for _, delta := range []int{-1, 0} {
		for _, deferred := range []bool{false, true} {
			b := DefaultParserBudget()
			b.MaxFrameBytes, b.MaxMessageBytes = 4096, 4096
			b.MaxBufferedBytes = need + delta
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			request := s.Feed(0, time.Unix(1, 0), q)
			require.Nil(t, request.Err)
			require.Len(t, request.Events, 1)
			out := s.Feed(1, time.Unix(2, 0), w)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			f, err := e.GetFields()
			if delta == 0 {
				require.Nil(t, out.Err)
				require.NoError(t, err)
				require.Len(t, f["data"].(map[string]any)["elements"], 2)
				require.Equal(t, request.Events[0].ID, e.TransactionID)
				require.Equal(t, request.Events[0].ID, e.ResponseTo)
				require.Equal(t, "observed-response", e.Session["Association"])
			} else {
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				require.Nil(t, e.Raw)
				require.Nil(t, e.Session)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
				late := s.Feed(1, time.Unix(3, 0), wrapperWire(t, "0001000100100007c401c1000403a0"))
				require.Nil(t, late.Err)
				require.Len(t, late.Events, 1)
				require.Zero(t, late.Events[0].ResponseTo)
				require.Zero(t, late.Events[0].TransactionID)
				require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
				require.Equal(t, 0, late.Events[0].Session["Outstanding"])
			}
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			require.Empty(t, s.Close("projection"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
}
