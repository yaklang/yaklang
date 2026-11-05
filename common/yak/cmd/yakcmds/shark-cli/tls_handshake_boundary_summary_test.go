package sharkcli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Row projection of admitted observations; native AEAD/state checks live in pcaputil.
func TestTLSHandshakeBoundarySummarySurvivesCoalescedMessages(t *testing.T) {
	for _, name := range []string{"request-gap-first", "request-gap-last", "update", "application", "application-warning-text", "duplicate", "unauthenticated", "other-protocol", "malformed"} {
		t.Run(name, func(t *testing.T) {
			gap := &pcaputil.ProtocolEvent{Protocol: "tls", Status: "decoded", Session: map[string]any{
				"Record Type": byte(23), "Negotiated Version": uint16(0x304), "Cipher Suite": uint16(0x1301),
				"Authentication Verified": true, "Inner Content Type": byte(22), "Content Visibility": "authenticated-plaintext",
				"Handshake Parsing": "unavailable after missing protected handshake bytes",
			}}
			request := certificateRequestPresentation(t, true)
			events := []*pcaputil.ProtocolEvent{gap, request}
			valid := true
			switch name {
			case "request-gap-last":
				events = []*pcaputil.ProtocolEvent{request, gap}
			case "update":
				events = []*pcaputil.ProtocolEvent{gap, keyUpdatePresentationForTest(1)}
			case "application":
				events = []*pcaputil.ProtocolEvent{gap, {Protocol: "http", Status: "decoded", Summary: "GET / HTTP/1.1", Fields: map[string]any{"Protocol": "HTTP/1.1"}}}
			case "application-warning-text":
				events = []*pcaputil.ProtocolEvent{gap, {Protocol: "http", Status: "decoded", Summary: "GET /boundary unavailable HTTP/1.1", Fields: map[string]any{"Protocol": "HTTP/1.1"}}}
			case "duplicate":
				events = []*pcaputil.ProtocolEvent{gap, request, gap}
			case "unauthenticated":
				gap.Session["Authentication Verified"], valid = false, false
			case "other-protocol":
				gap.Protocol, valid = "ssh", false
			case "malformed":
				gap.Status, gap.Error, valid = "malformed", "refused", false
			}
			row := presentEvents(events).apply(packetSummary{})
			if !valid {
				require.NotContains(t, secureTransportSummary(gap), "boundary unavailable")
				require.NotContains(t, row.Info, "boundary unavailable")
				return
			}
			require.Contains(t, secureTransportSummary(gap), "Handshake · boundary unavailable")
			require.Equal(t, 1, strings.Count(row.Info, "TLS handshake boundary unavailable | "), row.Info)
			require.True(t, strings.HasPrefix(row.Info, "TLS handshake boundary unavailable | "), row.Info)
			if strings.HasPrefix(name, "application") {
				require.Equal(t, "HTTP/1.1", row.Protocol)
				require.Contains(t, row.Info, "GET /")
				if name == "application-warning-text" {
					require.Contains(t, row.Info, "GET /boundary unavailable HTTP/1.1")
				}
			} else {
				require.Equal(t, "TLSv1.3", row.Protocol)
				if name == "update" {
					require.Contains(t, row.Info, "Key Update · update_requested")
				} else {
					require.Contains(t, row.Info, "signature algorithms")
				}
			}
		})
	}
}
