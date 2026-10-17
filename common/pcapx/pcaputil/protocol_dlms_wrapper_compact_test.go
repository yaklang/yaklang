package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDLMSWrapperCompactExistingAPIBaseline(t *testing.T) {
	for _, x := range []struct {
		name, wire string
		tag        byte
	}{
		{"adjacent-calendar", "000100010010000ac401c1001a07e8021d04", 26},
		{"compact-uint16", "000100010010000bc401c1001312040100ffff", 19},
	} {
		t.Run(x.name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("baseline")
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "000100100001000dc001c1000100002a0000ff0200"))
			require.Nil(t, q.Err)
			require.Len(t, q.Events, 1)
			out := s.Feed(1, time.Unix(2, 0), wrapperWire(t, x.wire))
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			f, err := out.Events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, x.tag, f["data"].(map[string]any)["type"])
			require.Equal(t, q.Events[0].ID, out.Events[0].TransactionID)
			require.Equal(t, q.Events[0].ID, out.Events[0].ResponseTo)
			require.Equal(t, "observed-response", out.Events[0].Session["Association"])
			require.Equal(t, 0, out.Events[0].Session["Outstanding"])
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSWrapperCompactSealedMatrix(t *testing.T) {
	wrapperBlocksReplay(t, wrapperBlockControlsFrom(t, "dlms-compact", 66))
}
func TestDLMSWrapperCompactBudgetsOwnership(t *testing.T) {
	wrapperBlocksOwnership(t, wrapperBlockControlsFrom(t, "dlms-compact", 66))
}
func TestDLMSWrapperCompactProjectionBoundary(t *testing.T) {
	q := wrapperWire(t, "000100100001000dc001c1000100002a0000ff0200")
	reply := wrapperWire(t, "000100010010000bc401c1001312040100ffff")
	// Reserve the complete compact schema and expanded value projection while one identity is live.
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
				rocEqualFields(t, compactCanonicalAnswer(t), data)
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

func compactCanonicalAnswer(t *testing.T) map[string]any {
	for _, c := range wrapperBlockControlsFrom(t, "dlms-compact", 66) {
		if c.Name == "compact-uint16-udp" {
			return c.Answers[1].Fields["data"].(map[string]any)
		}
	}
	t.Fatal("missing canonical compact answer")
	return nil
}

func TestDLMSWrapperCompactProbeAndLimits(t *testing.T) {
	cases := wrapperBlockControlsFrom(t, "dlms-compact", 66)
	for _, c := range cases {
		if c.Name != "compact-description-byte19" && c.Name != "compact-uint16-udp" {
			continue
		}
		for _, deferred := range []bool{false, true} {
			b := DefaultParserBudget()
			b.MaxFrameBytes, b.MaxMessageBytes = 4096, 4096
			b.MaxBufferedBytes = 64 << 10
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			var events []*ProtocolEvent
			for _, step := range c.Steps {
				out := s.Feed(step.Dir, time.Unix(1, 0), wrapperWire(t, step.Hex))
				events = append(events, out.Events...)
			}
			if c.Name == "compact-description-byte19" {
				assertWrapperBlocks(t, c, events, deferred)
			} else {
				require.Len(t, events, 2)
				fields, err := events[1].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, fields)
				require.Nil(t, events[1].Session)
				require.Zero(t, events[1].ResponseTo)
				require.Zero(t, events[1].TransactionID)
			}
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			require.Empty(t, s.Close("limits"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
	for _, limits := range []struct{ nodes, depth int }{{3, 64}, {4, 64}, {4096, 5}, {4096, 6}} {
		b := DefaultParserBudget()
		b.MaxCollectionElements, b.MaxRecursionDepth = limits.nodes, limits.depth
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		req := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "000100100001000dc001c1000100002a0000ff0200"))
		require.Nil(t, req.Err)
		out := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "000100010010000bc401c1001312040100ffff"))
		require.Len(t, out.Events, 1)
		fields, err := out.Events[0].GetFields()
		if limits.nodes == 3 || limits.depth == 5 {
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, fields)
			require.Nil(t, out.Events[0].Session)
			require.Zero(t, out.Events[0].ResponseTo)
			require.Zero(t, out.Events[0].TransactionID)
		} else {
			require.NoError(t, err)
			rocEqualFields(t, compactCanonicalAnswer(t), fields["data"].(map[string]any))
			require.Equal(t, req.Events[0].ID, out.Events[0].ResponseTo)
		}
		require.Empty(t, s.Close("limits"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}
