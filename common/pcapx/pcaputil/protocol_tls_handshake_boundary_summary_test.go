package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolSummaryTLSHandshakeBoundary(t *testing.T) {
	for _, c := range []struct {
		name        string
		auth        bool
		inner       byte
		parsing     bool
		boundary    bool
		unavailable bool
	}{
		{name: "recovered-epoch", auth: true, inner: 22, parsing: true, boundary: true, unavailable: true},
		{name: "later-handshake-record", auth: true, inner: 22, parsing: true, unavailable: true},
		{name: "unauthenticated", inner: 22, parsing: true, boundary: true},
		{name: "application-data", auth: true, inner: 23, parsing: true, boundary: true},
		{name: "no-boundary-gap", auth: true, inner: 22},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := &ProtocolEvent{Protocol: "tls", Status: "decoded", Session: map[string]any{
				"Record Type": byte(23), "Record Length": 32, "Content Visibility": "authenticated-plaintext",
				"Authentication Verified": c.auth, "Inner Content Type": c.inner,
			}}
			if c.parsing {
				e.Session["Handshake Parsing"] = "unavailable after missing protected handshake bytes"
			}
			if c.boundary {
				e.Session["Handshake Boundary"] = "unavailable after authenticated key-epoch recovery"
			}
			e.summarizeProtocol()
			if c.unavailable {
				require.Contains(t, e.Summary, "TLS Handshake · boundary unavailable")
				require.NotContains(t, e.Summary, "Application Data")
			} else {
				require.NotContains(t, e.Summary, "boundary unavailable")
			}
		})
	}
}
