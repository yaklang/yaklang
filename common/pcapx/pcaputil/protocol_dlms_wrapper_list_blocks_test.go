package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDLMSWrapperListBlocksExistingAPIBaseline(t *testing.T) {
	for _, c := range []struct {
		name, reply string
		block       bool
	}{
		{"adjacent-list", "000100010010000ac403c102001201000103", false},
		{"single-block-list", "0001000100100011c402c10100000001000702001201000103", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "0001001000010018c003c102000100002a0000ff0200000100002a0000ff0300"))
			require.Nil(t, q.Err)
			require.Len(t, q.Events, 1)
			out := s.Feed(1, time.Unix(2, 0), wrapperWire(t, c.reply))
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			f, err := e.GetFields()
			require.NoError(t, err)
			if c.block {
				require.Equal(t, true, f["transfer_complete"])
				require.Len(t, f["assembled_results"], 2)
			} else {
				require.Len(t, f["results"], 2)
			}
			require.Equal(t, q.Events[0].ID, e.ResponseTo)
			require.Equal(t, q.Events[0].ID, e.TransactionID)
			require.Empty(t, s.Close("baseline"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSWrapperListBlockConflictLifecycle(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, err)
	for i, w := range []string{
		"0001001000010018c003c102000100002a0000ff0200000100002a0000ff0300",
		"000100010010000dc402c100000000010003020012",
		"0001001000010007c002c100000001",
		"000100010010000ac403c102001201000103",
		"000100010010000dc402c101000000020003010001",
	} {
		dir := 1
		if i == 0 || i == 2 {
			dir = 0
		}
		out := s.Feed(dir, time.Unix(int64(1+i), 0), wrapperWire(t, w))
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		if i >= 3 {
			e := out.Events[0]
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			require.Equal(t, 0, e.Session["Outstanding"])
		}
	}
	require.Empty(t, s.Close("conflict"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDLMSWrapperListBlocksSealedMatrix(t *testing.T) {
	wrapperBlocksReplay(t, wrapperBlockControlsFrom(t, "dlms-list-blocks", 35))
}
func TestDLMSWrapperListBlocksBudgetsOwnership(t *testing.T) {
	wrapperBlocksOwnership(t, wrapperBlockControlsFrom(t, "dlms-list-blocks", 35))
}

func TestDLMSWrapperListBlockProjectionBoundary(t *testing.T) {
	q := wrapperWire(t, "0001001000010018c003c102000100002a0000ff0200000100002a0000ff0300")
	reply := wrapperWire(t, "0001000100100011c402c10100000001000702001201000103")
	// 512-byte conversation, one 256-byte retained identity, owned block Data,
	// graph projections and two top-level result maps. Adjacent budgets differ
	// by one byte; a refusal must tombstone the identity before a late response.
	need := 512 + 256 + int(wrapperProjection(reply, len(reply))) + wrapperBlockBytes + 2*8192
	for _, delta := range []int{-1, 0} {
		for _, deferred := range []bool{false, true} {
			b := DefaultParserBudget()
			b.MaxFrameBytes, b.MaxMessageBytes = 4096, 4096
			b.MaxBufferedBytes = need + delta
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			req := s.Feed(0, time.Unix(1, 0), q)
			require.Nil(t, req.Err)
			require.Len(t, req.Events, 1)
			out := s.Feed(1, time.Unix(2, 0), reply)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			f, err := e.GetFields()
			if delta == 0 {
				require.Nil(t, out.Err)
				require.NoError(t, err)
				require.Equal(t, true, f["transfer_complete"])
				require.Equal(t, req.Events[0].ID, e.TransactionID)
				require.Equal(t, req.Events[0].ID, e.ResponseTo)
				require.Len(t, f["assembled_results"], 2)
			} else {
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				require.Nil(t, e.Session)
				require.Nil(t, e.Raw)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
				// Available bytes after refusal cannot revive an old request generation.
				late := s.Feed(1, time.Unix(3, 0), reply)
				require.Nil(t, late.Err)
				require.Len(t, late.Events, 1)
				require.Zero(t, late.Events[0].TransactionID)
				require.Zero(t, late.Events[0].ResponseTo)
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
