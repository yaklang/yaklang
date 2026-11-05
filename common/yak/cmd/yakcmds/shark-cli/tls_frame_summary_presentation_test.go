package sharkcli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Authored projections of the independently captured Windows222 physical-frame
// cases. Native crypto and field-source acceptance remain separate contracts.
func tlsFragmentRowForTest(id uint64) *pcaputil.ProtocolEvent {
	e := keyUpdatePresentationForTest(0)
	e.ID = id
	delete(e.Session, "Handshake Messages")
	e.Session["Handshake Incomplete"] = true
	e.Session["SNI"] = fmt.Sprintf("fragment-%d.%s.example", id, strings.Repeat("a", 90))
	e.Session["ALPN"] = "http/1.1"
	return e
}

func TestTLSPhysicalFrameShowsCompletedHandshakeBeforeFragments(t *testing.T) {
	for _, count := range []int{1, 3, 5} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("fragments%d/reverse%v", count, reverse), func(t *testing.T) {
				ku := keyUpdatePresentationForTest(0)
				ku.ID = 20
				var events []*pcaputil.ProtocolEvent
				for i := 0; i < count; i++ {
					events = append(events, tlsFragmentRowForTest(uint64(i+1)))
				}
				if reverse {
					events = append([]*pcaputil.ProtocolEvent{ku}, events...)
				} else {
					events = append(events, ku)
				}
				before, err := json.Marshal(events)
				require.NoError(t, err)
				row := presentEvents(events).apply(packetSummary{})
				require.Equal(t, "TLSv1.3", row.Protocol)
				require.True(t, strings.HasPrefix(row.Info, "Key Update"), row.Info)
				require.Contains(t, clipText(row.Info, 80), "update_not_requested", "the selected completed message must survive list clipping")
				require.Contains(t, row.Info, "handshake reassembly incomplete", "keep the coalesced fragment evidence")
				after, err := json.Marshal(events)
				require.NoError(t, err)
				require.Equal(t, before, after, "presentation must not rewrite protocol events")
			})
		}
	}
}

func TestTLSPhysicalFrameKeepsMalformedReasonBeforeLaterContextFault(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse%v", reverse), func(t *testing.T) {
			fragment := tlsFragmentRowForTest(1)
			bad := &pcaputil.ProtocolEvent{ID: 2, Protocol: "tls", Status: "malformed", ExpertCode: string(pcaputil.ErrMalformedMessage), Error: "MalformedMessage: TLS 1.3 application data interleaves a fragmented handshake", Session: map[string]any{"Authentication Verified": true}}
			context := &pcaputil.ProtocolEvent{ID: 3, Protocol: "tls", Status: "context-required", ExpertCode: string(pcaputil.ErrContextRequired), Error: "ContextRequired: decode failure invalidated stream affinity"}
			events := []*pcaputil.ProtocolEvent{fragment, bad, context}
			if reverse {
				events = []*pcaputil.ProtocolEvent{bad, fragment, context}
			}
			row := presentEvents(events).apply(packetSummary{})
			require.Equal(t, "TLSv1.3", row.Protocol)
			require.Contains(t, clipText(row.Info, 100), "MalformedMessage")
			require.Contains(t, clipText(row.Info, 100), "interleaves")
			require.Contains(t, row.Info, "handshake reassembly incomplete")
			require.Contains(t, row.Info, "ContextRequired", "the later consequence remains separately visible")
			require.NotContains(t, row.Info, "Key Update")
		})
	}
}

func TestTLSPhysicalFrameDiagnosticDoesNotReplaceApplicationIdentity(t *testing.T) {
	bad := &pcaputil.ProtocolEvent{Protocol: "tls", Status: "malformed", ExpertCode: string(pcaputil.ErrMalformedMessage), Error: "MalformedMessage: TLS KeyUpdate does not end at the record boundary"}
	http := &pcaputil.ProtocolEvent{Protocol: "http", Status: "decoded", Summary: "GET / HTTP/1.1", Fields: map[string]any{"Protocol": "HTTP/1.1"}}
	for _, events := range [][]*pcaputil.ProtocolEvent{{tlsFragmentRowForTest(1), keyUpdatePresentationForTest(0), http, bad}, {bad, http, tlsFragmentRowForTest(1)}} {
		row := presentEvents(events).apply(packetSummary{})
		require.Equal(t, "HTTP/1.1", row.Protocol, "a TLS diagnostic is not an application identity")
		require.Contains(t, clipText(row.Info, 100), "MalformedMessage")
		require.Equal(t, 1, strings.Count(row.Info, "MalformedMessage"), "the first diagnosis is not repeated as the trailing diagnostic")
		require.Equal(t, "mixed-record-outcomes", row.ProtocolStatus)
	}
	context := &pcaputil.ProtocolEvent{Protocol: "tls", Status: "context-required", ExpertCode: string(pcaputil.ErrContextRequired), Error: "ContextRequired: decode failure invalidated stream affinity"}
	row := presentEvents([]*pcaputil.ProtocolEvent{bad, context}).apply(packetSummary{Protocol: "TCP", Transport: "tcp", Info: "wire TCP"})
	require.Equal(t, "TCP", row.Protocol, "diagnostics alone do not establish TLS identity")
	require.Equal(t, "malformed", row.ProtocolStatus)
	require.Contains(t, clipText(row.Info, 100), "MalformedMessage")
	require.Contains(t, row.Info, "ContextRequired")
}
