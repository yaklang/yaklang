package pcaputil

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	yaklang "github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func TestProtocolDumpLineReplay(t *testing.T) {
	request := "GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: request}, {seq: 1, data: response, reverse: true}}
	events, _, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 2)
	for i, event := range events {
		line := event.DumpLine()
		require.NotContains(t, line, "\n")
		require.Contains(t, line, `proto="http"`)
		require.Contains(t, line, `status="decoded"`)
		require.Contains(t, line, `src="`+event.Source+`"`)
		require.Contains(t, line, `dst="`+event.Destination+`"`)
		require.Contains(t, line, "offset=0")
		if i == 0 {
			require.Contains(t, line, `direction="A->B"`)
			require.Contains(t, line, "GET / HTTP/1.1")
		} else {
			require.Contains(t, line, `direction="B->A"`)
			require.Contains(t, line, "response_to=")
			require.Contains(t, line, "HTTP/1.1 200 OK")
		}
	}
}

func TestProtocolDumpLineYakMethod(t *testing.T) {
	input := binTestPcap(t, []tcpStep{{seq: 0, syn: true}, {seq: 1, data: string(binMQTTConnect)}}, 1883, false, false)
	events, _, err := binReplay(t, input, 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	var got string
	engine := yaklang.New()
	engine.SetVars(map[string]any{
		"message": events[0],
		"record":  func(line string) { got = line },
	})
	require.NoError(t, engine.SafeEval(context.Background(), `record(message.DumpLine())`))
	require.Equal(t, events[0].DumpLine(), got)
	require.Contains(t, got, `proto="mqtt"`)
}

func TestProtocolDumpLineEscapingAndOwnership(t *testing.T) {
	e := &ProtocolEvent{
		Timestamp: time.Unix(1, 123).UTC(), ID: 7, FlowID: 3,
		Transport: "udp", Status: "context-required", Source: "[2001:db8::1]:53", Destination: "192.0.2.1:40000",
		Direction: 1, Offset: 123, Length: 20, TransactionID: 5, ResponseTo: 4,
		ExpertCode: "ContextRequired", Summary: "quoted \"value\"\r\nnext\tline\x1b[31m", Error: "missing\ncontext",
		Session: map[string]any{"Packet Name": "query\nwith newline"},
	}
	line := e.DumpLine()
	require.False(t, strings.ContainsAny(line, "\r\n\t\x1b"))
	require.Contains(t, line, `proto="unknown"`)
	require.Contains(t, line, `direction="source->destination"`, "UDP has no persistent A/B connection direction")
	require.Contains(t, line, `op="query\nwith newline"`)
	require.Contains(t, line, `error="missing\ncontext"`)
	require.Contains(t, line, "transaction=5 response_to=4")
	require.Equal(t, line, e.DumpLine(), "formatting must not mutate the event")
	require.Equal(t, "query\nwith newline", e.Session["Packet Name"])
	require.Nil(t, e.Fields, "formatting must not invoke deferred decoding")
	var absent *ProtocolEvent
	require.Equal(t, "<nil ProtocolEvent>", absent.DumpLine())
}
