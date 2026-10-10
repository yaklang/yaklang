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

func TestProtocolHTTPRetainedHeadersPreserveWireFields(t *testing.T) {
	body := strings.Repeat("x", 4096)
	header := "POST /chunks HTTP/1.1\r\nHost: example.test\r\nTransfer-Encoding: chunked\r\nTrailer: X-End, X-Checksum\r\nX-Repeat: first\r\nx-repeat: second\r\n\r\n"
	request := header + fmt.Sprintf("%x;label=a\r\n%s\r\n0\r\nX-End: yes\r\nX-Checksum: good\r\n\r\n", len(body), body)
	fixedHeader := "POST /fixed HTTP/1.1\r\nHost: example.test\r\nContent-Length: 4096\r\nContent-Length: 4096\r\nX-Repeat: first\r\nx-repeat: second\r\n\r\n"
	responseHeader := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nTrailer: X-End\r\nSet-Cookie: a=1\r\nSet-Cookie: b=2\r\n\r\n"
	response := responseHeader + fmt.Sprintf("%x\r\n%s\r\n0\r\nX-End: yes\r\n\r\n", len(body), body)
	for _, deferred := range []bool{false, true} {
		for _, workers := range []int{1, 4} {
			t.Run(fmt.Sprintf("deferred%v/workers%d", deferred, workers), func(t *testing.T) {
				steps := []tcpStep{{syn: true}, {syn: true, reverse: true}, {seq: 1, data: request + fixedHeader + body}, {seq: 1, reverse: true, data: response + "HTTP/1.1 204 No Content\r\n\r\n"}}
				events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), workers, WithProtocolDeferred(deferred), WithProtocolBudget(512, 16384))
				require.NoError(t, err)
				require.Len(t, events, 4)
				require.Zero(t, stats.Malformed)
				require.Zero(t, stats.Incomplete)
				require.Zero(t, stats.BufferedBytes)
				for i, expectedRaw := range []string{header, fixedHeader, responseHeader} {
					event := events[i]
					require.Equal(t, "limited", event.Status)
					require.Equal(t, "headers", event.Completeness)
					require.Equal(t, expectedRaw, string(event.Raw))
					fields, err := event.GetFields()
					require.NoError(t, err)
					headers := fields["Headers"].(map[string]any)
					if i < 2 {
						require.Equal(t, []string{"example.test"}, headers["Host"])
						require.Equal(t, []string{"first", "second"}, headers["X-Repeat"])
					}
					if i == 1 {
						require.Equal(t, []string{"4096", "4096"}, headers["Content-Length"])
					} else {
						require.Equal(t, []string{"chunked"}, headers["Transfer-Encoding"])
					}
					if i == 0 {
						require.Equal(t, []string{"X-End, X-Checksum"}, headers["Trailer"])
					}
					if i == 2 {
						require.Equal(t, []string{"X-End"}, headers["Trailer"])
						require.Equal(t, []string{"a=1", "b=2"}, headers["Set-Cookie"])
						require.Equal(t, events[0].ID, event.ResponseTo)
					}
					view, err := NewProtocolInspector()
					require.NoError(t, err)
					view.OnEvent(event)
					details, err := view.Details(event.ID)
					require.NoError(t, err)
					require.Equal(t, fields, details.Fields)
				}
				require.Equal(t, events[1].ID, events[3].ResponseTo)
			})
		}
	}
	// Context diagnostics have the same complete header projection even without
	// a queued request; absent framing context must not erase observed headers.
	events, _, err := binReplay(t, binTestPcap(t, []tcpStep{{syn: true}, {seq: 1, data: responseHeader}}, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "context-required", events[0].Status)
	fields, err := events[0].GetFields()
	require.NoError(t, err)
	headers := fields["Headers"].(map[string]any)
	require.Equal(t, []string{"chunked"}, headers["Transfer-Encoding"])
	require.Equal(t, []string{"X-End"}, headers["Trailer"])
}
