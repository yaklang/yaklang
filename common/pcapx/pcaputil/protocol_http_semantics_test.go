package pcaputil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolHTTPBodylessRepresentationLength(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint("deferred", deferred), func(t *testing.T) {
			request := "HEAD /large HTTP/1.1\r\nHost: example.test\r\n\r\nGET /cached HTTP/1.1\r\nHost: example.test\r\n\r\nGET /after HTTP/1.1\r\nHost: example.test\r\n\r\n"
			response := "HTTP/1.1 200 OK\r\nContent-Length: 83886080\r\n\r\nHTTP/1.1 304 Not Modified\r\nContent-Length: 9223372036854775807\r\nETag: \"abc\"\r\n\r\nHTTP/1.1 200 \r\nContent-Length: 2\r\n\r\nOK"
			steps := []tcpStep{{syn: true}, {syn: true, reverse: true}, {seq: 1, data: request}, {seq: 1, reverse: true, data: response}}
			events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 4, WithProtocolDeferred(deferred), WithProtocolBudget(1<<20, 32<<20))
			require.NoError(t, err)
			require.Len(t, events, 6)
			require.Zero(t, stats.Malformed)
			require.Zero(t, stats.LimitedBytes)
			require.Zero(t, stats.BufferedBytes)
			for i, e := range events[3:] {
				require.Equal(t, events[i].ID, e.ResponseTo)
				fields, err := e.GetFields()
				require.NoError(t, err, e.DumpLine())
				responseFields := fields["Message"].(map[string]any)["HTTP Response"].(map[string]any)
				if i < 2 {
					require.NotContains(t, responseFields, "Body")
					if i == 0 {
						require.Contains(t, responseFields["Headers"], "Content-Length: 83886080")
					} else {
						require.Contains(t, responseFields["Headers"], "Content-Length: 9223372036854775807")
						require.Contains(t, responseFields["Headers"], "ETag: \"abc\"")
					}
				}
				body, err := e.DecodeHTTPBody(1024)
				require.NoError(t, err)
				if i < 2 {
					require.Empty(t, body.Data)
				} else {
					require.Equal(t, "OK", string(body.Data))
				}
			}
		})
	}
}

func TestProtocolHTTPConfiguredBodyBudgetFields(t *testing.T) {
	body := strings.Repeat("budget-body-", (17<<20)/12+1)
	request := fmt.Sprintf("POST /large HTTP/1.1\r\nHost: example.test\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	response := "HTTP/1.0 200 OK\r\n\r\n" + body
	steps := []tcpStep{{syn: true}, {syn: true, reverse: true}}
	for at := 0; at < len(request); at += 16384 {
		steps = append(steps, tcpStep{seq: 1 + uint32(at), data: request[at:min(at+16384, len(request))]})
	}
	for at := 0; at < len(response); at += 16384 {
		steps = append(steps, tcpStep{seq: 1 + uint32(at), reverse: true, data: response[at:min(at+16384, len(response))]})
	}
	steps = append(steps, tcpStep{seq: 1 + uint32(len(response)), reverse: true, fin: true}, tcpStep{seq: 1 + uint32(len(request)), fin: true})
	wire := binTestPcap(t, steps, 80, false, false)
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint("deferred", deferred), func(t *testing.T) {
			events, stats, err := binReplay(t, wire, 4, WithProtocolDeferred(deferred), WithProtocolBudget(32<<20, 128<<20))
			require.NoError(t, err)
			require.Len(t, events, 2)
			require.Zero(t, stats.Malformed)
			require.Zero(t, stats.LimitedBytes)
			require.Zero(t, stats.BufferedBytes)
			for i, e := range events {
				fields, err := e.GetFields()
				require.NoError(t, err, e.DumpLine())
				message := fields["Message"].(map[string]any)
				kind := "HTTP Request"
				if i == 1 {
					kind = "HTTP Response"
					require.Equal(t, events[0].ID, e.ResponseTo)
				}
				require.Equal(t, body, message[kind].(map[string]any)["Body"].(map[string]any)["Octets"])
			}
		})
	}
}
