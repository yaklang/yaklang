package pcaputil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPLongStartLineUsesDedicatedProbe(t *testing.T) {
	uri := "/download?" + strings.Repeat("a", 4096)
	longHeader := strings.Repeat("b", 12<<10)
	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: example.test\r\nX-Long: %s\r\n\r\n", uri, longHeader)
	response := "HTTP/1.1 204 No Content\r\n\r\n"
	steps := []tcpStep{
		{seq: 0, syn: true},
		{seq: 0, syn: true, reverse: true},
		{seq: 1, data: request},
		{seq: 1, data: response, reverse: true},
	}

	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "http", events[0].Protocol)
	require.NotEqual(t, "unrecognized", events[0].Status)
	require.Equal(t, request, string(events[0].Raw))
	require.Equal(t, request[:strings.IndexByte(request, '\r')], events[0].Summary)
	require.Equal(t, "http", events[1].Protocol)
	require.Equal(t, "decoded", events[1].Status)
	require.Zero(t, stats.Unknown)
}

func TestHTTPResponseWithoutRequestUsesResponseFraming(t *testing.T) {
	cases := []struct {
		name     string
		response string
		body     string
	}{
		{
			name:     "content-length",
			response: "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 4\r\n\r\nbody",
			body:     "body",
		},
		{
			name:     "close-delimited",
			response: "HTTP/1.0 200 OK\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\nThe contents of the response body.",
			body:     "The contents of the response body.",
		},
	}

	for _, tc := range cases {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deferred=%t", tc.name, deferred), func(t *testing.T) {
				steps := []tcpStep{
					{seq: 0, syn: true},
					{seq: 0, syn: true, reverse: true},
					{seq: 1, data: tc.response, reverse: true},
					{seq: 1 + uint32(len(tc.response)), fin: true, reverse: true},
					{seq: 1, fin: true},
				}
				events, stats, err := binReplay(t, binTestPcap(t, steps, 8080, false, false), 1,
					WithHTTPResponseWithoutRequest(true),
					WithProtocolDeferred(deferred),
				)
				require.NoError(t, err)
				require.Len(t, events, 1)
				event := events[0]
				require.Equal(t, "http", event.Protocol)
				require.Contains(t, []string{"decoded", "deferred"}, event.Status)
				require.Equal(t, tc.response, string(event.Raw))
				require.Equal(t, "message", event.Completeness)
				require.Equal(t, false, event.Session["Request Context Available"])
				require.Equal(t, "validated-response-without-request-context", event.Admission)
				require.Zero(t, event.ResponseTo)
				require.Zero(t, event.TransactionID)
				require.NotNil(t, event.SourceBytes.CompletedIn)
				fields, err := event.GetFields()
				require.NoError(t, err)
				responseFields := fields["Message"].(map[string]any)["HTTP Response"].(map[string]any)
				require.Equal(t, "200", responseFields["FirstLine"].(map[string]any)["Status"])
				body, err := event.DecodeHTTPBody(1024)
				require.NoError(t, err)
				require.Equal(t, tc.body, string(body.Data))
				require.EqualValues(t, 1, stats.Decoded+stats.Deferred)
				require.Zero(t, stats.ContextRequired)
			})
		}
	}
}

func TestHTTPResponseWithoutRequestIsExplicitOptIn(t *testing.T) {
	response := "HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nbody"
	steps := []tcpStep{
		{seq: 0, syn: true},
		{seq: 0, syn: true, reverse: true},
		{seq: 1, data: response, reverse: true},
	}

	events, _, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "context-required", events[0].Status)
	require.Nil(t, events[0].SourceBytes.CompletedIn)
	_, _, ok := events[0].HTTPResponseBody()
	require.False(t, ok)
}
