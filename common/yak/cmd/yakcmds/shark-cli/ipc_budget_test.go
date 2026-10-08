package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestReviewDecoderProjectionPreservesNilEvents(t *testing.T) {
	for _, events := range [][]*pcaputil.ProtocolEvent{nil, {}} {
		wire, err := marshalDecodeRequest(decodeRequest{Events: events, Number: 5})
		require.NoError(t, err)
		var request decodeRequest
		require.NoError(t, json.Unmarshal(wire, &request))
		require.Equal(t, events == nil, request.Events == nil, "nil selects the packet dissector; empty selects event projection")
	}
}

func TestReviewDecoderRequestUsesEncodedBudget(t *testing.T) {
	fields := map[string]any{}
	for i := 0; i < 32; i++ {
		fields[fmt.Sprint(i)] = strings.Repeat("\x00", 16384)
	}
	e := &pcaputil.ProtocolEvent{ID: 7, Protocol: "http", Status: "decoded", Fields: fields, Session: fields}
	request := decodeRequest{Number: 3, Events: []*pcaputil.ProtocolEvent{e}}
	encoded, err := marshalDecodeRequest(request)
	require.NoError(t, err)
	// Shared Go values appear twice in JSON; control characters expand to six bytes.
	require.LessOrEqual(t, len(encoded), 2<<20)
	var bounded decodeRequest
	require.NoError(t, json.Unmarshal(encoded, &bounded))
	require.Equal(t, uint64(3), bounded.Number)
	require.Equal(t, "limited", bounded.Events[0].Status)
	require.Contains(t, bounded.Events[0].Summary, "IPC")
	require.Equal(t, "decoded", e.Status, "budget projection must not mutate the history")
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(encoded), &output))
	require.LessOrEqual(t, output.Len(), 8<<20)
}

func TestReviewDecoderResponseUsesEncodedBudget(t *testing.T) {
	// A repeated list label occurs once in the input and once per displayed leaf.
	// HTML escaping expands '<' to six bytes, while safeText preserves it.
	values := make([]any, 4096)
	for i := range values {
		values[i] = i
	}
	request := decodeRequest{Number: 4, Events: []*pcaputil.ProtocolEvent{{ID: 1, Protocol: "http", Fields: map[string]any{strings.Repeat("<", 2100): values}}}}
	input, err := json.Marshal(request)
	require.NoError(t, err)
	require.Less(t, len(input), 2<<20)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(input), &output))
	require.LessOrEqual(t, output.Len(), 8<<20)
	require.Contains(t, output.String(), "IPC")
	_, err = decodeWorkerOutput(output.Bytes(), &capturedPacket{number: 4})
	require.NoError(t, err)
}

func TestReviewDecoderRealHTTPNULBody(t *testing.T) {
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("FIN")
	body := bytes.Repeat([]byte{0}, 200<<10)
	wire := append([]byte(fmt.Sprintf("POST /ipc HTTP/1.1\r\nHost: example.test\r\nContent-Length: %d\r\n\r\n", len(body))), body...)
	r := s.Feed(0, time.Unix(1, 0), wire)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "decoded", r.Events[0].Status)
	request := decodeRequest{Number: 9, Events: r.Events}
	unbounded, err := json.Marshal(request)
	require.NoError(t, err)
	require.Greater(t, len(unbounded), decoderInputLimit, "raw bytes fit but repeated escaped field maps exceed IPC")
	encoded, err := marshalDecodeRequest(request)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), decoderInputLimit)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(encoded), &output))
	require.LessOrEqual(t, output.Len(), decoderOutputLimit)
	d, err := decodeWorkerOutput(output.Bytes(), &capturedPacket{number: 9})
	require.NoError(t, err)
	require.NotEmpty(t, d.fields)
	require.Contains(t, output.String(), "IPC")
	require.Equal(t, "decoded", r.Events[0].Status)
	require.Equal(t, wire, r.Events[0].Raw, "IPC projection must preserve owned event evidence")
}
