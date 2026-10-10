package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDLMSWrapperNormalDataExistingAPIBaseline(t *testing.T) {
	for _, c := range []struct {
		name, wire string
		data       map[string]any
	}{
		{"adjacent-uint16", "0001000100100007c401c100120100", map[string]any{"type": float64(18), "raw_hex": "120100", "value": float64(256)}},
		{"bit-string", "0001000100100007c401c1000403a0", map[string]any{"type": float64(4), "raw_hex": "0403a0", "bit_length": float64(3), "length_encoding_hex": "03", "value_hex": "a0", "unused_bits": float64(5)}},
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

func TestDLMSWrapperNormalExtendedSealedMatrix(t *testing.T) {
	wrapperBlocksReplay(t, wrapperBlockControlsFrom(t, "dlms-normal-data", 29))
}

func TestDLMSWrapperNormalExtendedBudgetsOwnership(t *testing.T) {
	wrapperBlocksOwnership(t, wrapperBlockControlsFrom(t, "dlms-normal-data", 29))
}

func TestDLMSWrapperNormalExtendedProjectionBoundary(t *testing.T) {
	q := wrapperWire(t, "000100100001000dc001c1000100002a0000ff0200")
	reply := wrapperWire(t, "0001000100100009c401c100173fc00000")
	// Reserve the owned scalar projections while one request identity is live.
	// The adjacent one-byte refusal must retire that identity before any late,
	// smaller reply that fits the same configured shared-byte budget.
	need := 512 + 256 + int(wrapperProjection(reply, len(reply))) + 256
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
			out := s.Feed(1, time.Unix(2, 0), reply)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			f, err := e.GetFields()
			if delta == 0 {
				require.Nil(t, out.Err)
				require.NoError(t, err)
				data := f["data"].(map[string]any)
				rocEqualFields(t, map[string]any{"type": float64(23), "raw_hex": "173fc00000", "value_hex": "3fc00000", "float_width": float64(32), "negative": false, "float_class": "normal", "value": 1.5}, data)
				require.Equal(t, request.Events[0].ID, e.ResponseTo)
				require.Equal(t, request.Events[0].ID, e.TransactionID)
				require.Equal(t, "observed-response", e.Session["Association"])
				require.Equal(t, 0, e.Session["Outstanding"])
			} else {
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				require.Nil(t, e.Session)
				require.Nil(t, e.Raw)
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
