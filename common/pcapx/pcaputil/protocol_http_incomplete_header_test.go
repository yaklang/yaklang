package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPIncompleteMessageKeepsValidatedResponseHeader(t *testing.T) {
	for _, framing := range []string{"Connection: close\r\n", "Content-Length: 8\r\n", "Transfer-Encoding: chunked\r\n"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deferred%t", framing, deferred), func(t *testing.T) {
				s, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				f := s.(*captureSession).f
				f.a.config.Deferred = deferred
				s.Feed(0, time.Unix(1, 0), []byte("GET / HTTP/1.1\r\nHost: owned.invalid\r\n\r\n"))
				header := "HTTP/1.1 200 OK\r\n" + framing + "X-Repeat: one\r\nX-Repeat: two\r\n\r\n"
				body := "body"
				if framing == "Transfer-Encoding: chunked\r\n" {
					body = "4\r\nab"
				}
				wire := []byte(header + body)
				r := s.Feed(1, time.Unix(2, 0), wire)
				require.Empty(t, r.Events)
				events := s.Close("TLS carrier closed")
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "incomplete", e.Status)
				require.Equal(t, "headers", e.Completeness)
				require.Equal(t, "http-incomplete-message-header-observation", e.Profile)
				require.Equal(t, wire, e.Raw)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				require.Nil(t, e.SourceBytes.CompletedIn)
				fields, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, 200, fields["Status Code"])
				require.Equal(t, []string{"one", "two"}, fields["Headers"].(map[string]any)["X-Repeat"])
				require.NotContains(t, fields, "Body")
				e.Fields = map[string]any{"Status Code": 204, "Body": "forged"}
				e.Raw, e.Status, e.Protocol = []byte("forged"), "decoded", "other"
				fields, err = e.GetFields()
				require.NoError(t, err)
				require.Equal(t, 200, fields["Status Code"])
				require.NotContains(t, fields, "Body")
			})
		}
	}
}
