package pcaputil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSIPHTTPOptionsAdmission(t *testing.T) {
	for _, target := range []string{"sip:bob@example.test", "sips:bob@example.test", "SIP:bob@example.test", "*"} {
		for _, chunk := range []int{0, 1, 2, 7} {
			t.Run(fmt.Sprintf("%s/chunk=%d", target, chunk), func(t *testing.T) {
				req := sipMsg("OPTIONS "+target+" SIP/2.0", [][2]string{
					{"Via", "SIP/2.0/TCP example.test;branch=z9hG4bKoptions"},
					{"From", "<sip:alice@example.test>;tag=1"},
					{"To", "<sip:bob@example.test>"},
					{"Call-ID", "options@example.test"},
					{"CSeq", "1 OPTIONS"},
				}, "")
				resp := sipResp("200", "OK", "z9hG4bKoptions", "1 OPTIONS", "options@example.test", "<sip:alice@example.test>;tag=1", "<sip:bob@example.test>;tag=2")
				events, _ := sessionTestFlow(t, "sip", []sessionStep{{0, req}, {1, resp}}, chunk, false)
				require.Len(t, events, 2)
				assertSessionEvents(t, events, "sip", false)
			})
		}
	}
	for _, target := range []string{"*", "/path", "http://example.test/path"} {
		events, _ := sessionTestFlow(t, "http", []sessionStep{
			{0, []byte("OPTIONS " + target + " HTTP/1.1\r\nHost: example.test\r\n\r\n")},
			{1, []byte("HTTP/1.1 204 No Content\r\n\r\n")},
		}, 1, true)
		require.Len(t, events, 2)
		for _, e := range events {
			require.Equal(t, "http", e.Protocol)
			require.Equal(t, "deferred", e.Status)
			require.Empty(t, e.Error)
			result, err := e.Decode()
			require.NoError(t, err)
			require.NotEmpty(t, result)
		}
	}
}
