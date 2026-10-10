package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDLMSWrapperSmallGraphProjectionExistingAPIBaseline(t *testing.T) {
	for _, item := range []struct {
		prefix, name string
		count        int
	}{
		{"dlms-normal-data", "normal-bits3-udp", 29},
		{"dlms-normal-graph", "normal-graph-basic-udp", 24},
		{"dlms-normal-access", "normal-access-structure-udp", 31},
	} {
		t.Run(item.name, func(t *testing.T) {
			var chosen *wrapperBlockControl
			for _, c := range wrapperBlockControlsFrom(t, item.prefix, item.count) {
				if c.Name == item.name {
					copy := c
					chosen = &copy
					break
				}
			}
			require.NotNil(t, chosen)
			wrapperSmallBudgetReplay(t, *chosen)
			for _, deferred := range []bool{false, true} {
				b := DefaultParserBudget()
				b.MaxBufferedBytes = 65536
				b.MaxMessageBytes, b.MaxFrameBytes = 4096, 4096
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				var es []*ProtocolEvent
				for _, st := range chosen.Steps {
					out := s.Feed(st.Dir, time.Unix(1, 0), wrapperWire(t, st.Hex))
					require.Nil(t, out.Err)
					es = append(es, out.Events...)
				}
				assertWrapperBlocks(t, *chosen, es, deferred)
				require.Equal(t, es[0].ID, es[1].ResponseTo)
				require.Equal(t, es[0].ID, es[1].TransactionID)
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(65536))
				t.Logf("%s deferred=%v accounted_peak_bytes=%d buffer_limit=65536", chosen.Name, deferred, s.Stats().PeakBufferedBytes)
				require.Empty(t, s.Close("budget"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			}
		})
	}
}

func TestDLMSWrapperSmallGraphUnprovenCount(t *testing.T) {
	cases := wrapperBlockControlsFrom(t, "dlms-small-graph-budget", 1)
	wrapperBlocksReplay(t, cases)
	for _, deferred := range []bool{false, true} {
		b := DefaultParserBudget()
		b.MaxBufferedBytes = 65536
		b.MaxFrameBytes, b.MaxMessageBytes = 4096, 4096
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		s.(*captureSession).f.a.config.Deferred = deferred
		var es []*ProtocolEvent
		for i, st := range cases[0].Steps {
			out := s.Feed(st.Dir, time.Unix(1, 0), wrapperWire(t, st.Hex))
			if i == 1 {
				rocTypedError(t, "MalformedMessage", out.Err)
			} else {
				require.Nil(t, out.Err)
			}
			es = append(es, out.Events...)
		}
		assertWrapperBlocks(t, cases[0], es, deferred)
		require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(65536))
		require.Empty(t, s.Close("bounded"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
	wrapperBlocksOwnership(t, cases)
}

func wrapperSmallBudgetReplay(t *testing.T, c wrapperBlockControl) {
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		es, stats := discoveryReplay(t, wrapperStructuredInput(t, c.Alias), workers, deferred, observe, WithProtocolBudget(4096, 65536), WithProtocolDecodeAs("udp", 4059, "dlms-wrapper"))
		assertWrapperBlocks(t, c, es, deferred)
		require.Zero(t, stats.BufferedBytes)
		require.LessOrEqual(t, stats.PeakBufferedBytes, int64(65536))
		require.EqualValues(t, len(c.Answers), stats.Messages)
		for i, e := range es {
			require.Equal(t, c.Answers[i].Dir, e.Direction)
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.Equal(t, c.Answers[i].Refs[0], e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
		}
	})
}
