package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolHistoryCostAndOwnership(t *testing.T) {
	event := &BinParserEvent{ID: 9, Raw: []byte("raw"), Session: reviewSession(4)}
	storage := make([]byte, 1, 1<<20)
	storage[0] = 7
	event.Session["bytes"] = storage
	cost := protocolHistoryBytes(event)
	view, err := NewBinParserInspector(2, cost*2)
	require.NoError(t, err)
	view.OnEvent(event)
	require.Equal(t, cost, view.bytes)
	event.Raw[0] = 'X'
	storage[0] = 9
	detail, err := view.Details(9)
	require.NoError(t, err)
	require.Equal(t, []byte("raw"), detail.Raw)
	require.Equal(t, byte(7), detail.Session["bytes"].([]byte)[0])
	// Caller mutation after arrival cannot change the cost subtracted at eviction.
	event.Session["extra"] = bytes.Repeat([]byte{'y'}, cost)
	view.OnEvent(event)
	require.EqualValues(t, 1, view.Evicted())
	require.Equal(t, cost, view.bytes)
	for _, id := range []uint64{2, 1, 4} {
		view.OnEvent(&BinParserEvent{ID: id, Raw: make([]byte, cost)})
	}
	require.Equal(t, cost*2, view.bytes)
	require.EqualValues(t, 3, view.Evicted())
	rows, cursor, omitted := view.RowsAfter(0, 10, "", 0)
	require.EqualValues(t, 5, cursor)
	require.EqualValues(t, 3, omitted)
	require.Len(t, rows, 2)
	require.EqualValues(t, 1, rows[0].ID)
	require.EqualValues(t, 4, rows[1].ID)
	oversized := &BinParserEvent{Session: reviewSession(64)}
	small, _ := NewBinParserInspector(2, 128)
	require.Zero(t, testing.AllocsPerRun(100, func() { small.OnEvent(oversized) }), "oversized sessions must be rejected before cloning")
}

func TestHTTPIncrementalFramingPipeline(t *testing.T) {
	first := "POST / HTTP/1.1\r\nHost: example.test\r\nTransfer-Encoding: chunked\r\n\r\n3;long=" + string(bytes.Repeat([]byte{'x'}, 64)) + "\r\nabc\r\n0\r\nX-Trailer: " + string(bytes.Repeat([]byte{'t'}, 128)) + "\r\n\r\n"
	next := "GET /next HTTP/1.1\r\nHost: example.test\r\n\r\n"
	wire := first + next
	for _, step := range []int{1, 2, 3, 7, 31, len(first) - 2} {
		steps := []tcpStep{{syn: true, seq: 0}}
		for at := 0; at < len(wire); at += step {
			end := min(at+step, len(wire))
			steps = append(steps, tcpStep{seq: uint32(at + 1), data: wire[at:end]})
		}
		events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 2)
		require.NoError(t, err)
		require.EqualValues(t, 2, stats.Decoded)
		require.Zero(t, stats.Malformed)
		require.Len(t, events, 2)
		require.Equal(t, first, string(events[0].Raw))
		require.Equal(t, next, string(events[1].Raw))
	}
}
