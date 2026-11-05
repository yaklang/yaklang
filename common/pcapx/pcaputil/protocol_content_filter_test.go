package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestContentPredicatesPreserveUnknownAndExactBytes(t *testing.T) {
	for _, expr := range []string{`pdu.content contains "GET /"`, `pdu.content hex:00ff`, `pdu.content contains "GET /" and ip.src == 10.0.0.1`} {
		f, err := CompileDisplayFilter(expr)
		require.NoError(t, err)
		e := &ProtocolEvent{Protocol: "http", Status: "decoded", Completeness: "message", Source: "10.0.0.1:80", Raw: []byte("GET /\x00\xff")}
		require.True(t, f.EvaluateEvents([]*ProtocolEvent{e}).Matched)
		for _, completeness := range []string{"encrypted", "incomplete", "headers", "evicted"} {
			e.Completeness = completeness
			result := f.EvaluateEvents([]*ProtocolEvent{e})
			require.False(t, result.Matched)
			require.False(t, result.Complete)
			require.True(t, result.Partial, completeness)
		}
	}
	for _, expr := range []string{`pdu.content hex:0`, `pdu.content hex:zz`, `pdu.content contains ""`, `pdu.content contains bare`, `pdu.content == "GET"`} {
		_, err := CompileDisplayFilter(expr)
		require.Error(t, err, expr)
	}
	f, err := CompileDisplayFilter(`not (pdu.content contains "missing")`)
	require.NoError(t, err)
	result := f.EvaluateEvents([]*ProtocolEvent{{Status: "decoded", Completeness: "message", Raw: []byte("present")}})
	require.True(t, result.Matched)
	require.True(t, result.Complete)
}

func TestStreamContentObserverCrossesOnlyContinuousSameDirectionBytes(t *testing.T) {
	f, err := CompileDisplayFilter(`stream.content contains "example"`)
	require.NoError(t, err)
	observer := f.NewObserver()
	e := &ProtocolEvent{Status: "decoded", Completeness: "message", Transport: "tcp", TCPFlowID: 1, Source: "192.0.2.1:1", Destination: "192.0.2.2:2", Raw: []byte("ex")}
	first := observer.Observe(e)
	require.False(t, f.EvaluateSnapshots([]DisplayFieldSnapshot{first}).Matched)
	e.Offset = 2
	e.Raw = []byte("ample")
	second := observer.Observe(e)
	require.True(t, f.EvaluateSnapshots([]DisplayFieldSnapshot{second}).Matched)
	// Earlier output owns the original decision even after stream progress.
	require.False(t, f.EvaluateSnapshots([]DisplayFieldSnapshot{first}).Matched)
	observer = f.NewObserver()
	e.Offset = 0
	e.Raw = []byte("ex")
	observer.Observe(e)
	e.Offset = 3
	e.Raw = []byte("ample")
	result := f.EvaluateSnapshots([]DisplayFieldSnapshot{observer.Observe(e)})
	require.False(t, result.Complete)
	require.True(t, result.Partial)
	e.TCPFlowID = 2
	e.Offset = 0
	e.Raw = []byte("ample")
	require.False(t, f.EvaluateSnapshots([]DisplayFieldSnapshot{observer.Observe(e)}).Matched)
	for i := uint64(3); i < 1200; i++ {
		e.TCPFlowID = i
		observer.Observe(e)
	}
	require.LessOrEqual(t, len(observer.streams), 1024)
	e.TCPFlowID = 1
	e.Offset = 0
	e.Raw = []byte("missing")
	result = f.EvaluateSnapshots([]DisplayFieldSnapshot{observer.Observe(e)})
	require.False(t, result.Complete, "history eviction cannot certify a stream's missing content")
}
