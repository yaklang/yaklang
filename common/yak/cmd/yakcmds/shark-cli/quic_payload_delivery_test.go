package sharkcli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestQUICRejectedPlaintextSourceDoesNotInventWholePacketRollback(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		for _, empty := range []bool{false, true} {
			for _, ipc := range []bool{false, true} {
				t.Run(fmt.Sprintf("stopped%v/empty%v/ipc%v", stopped, empty, ipc), func(t *testing.T) {
					e := quicCoalescedDiagnosticEvents(string(pcaputil.ErrMalformedMessage), "TLS CRYPTO message refused")[2]
					e.Session["Payload Delivery"] = "unqualified public metadata"
					want := "delivery not certified"
					if stopped {
						e.Session["Payload Delivery"] = "packet parsing stopped at refused frame; earlier validated frames may have been applied"
						want = "parsing stopped"
					}
					if empty {
						e.Session["Rejected Packet Payload"] = []byte{}
					}
					e = quicDiagnosticIPC(t, []*pcaputil.ProtocolEvent{e}, ipc)[0]
					fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
					found := false
					for _, f := range fields {
						if !strings.HasPrefix(f.text, "Rejected Packet Payload:") {
							continue
						}
						found = true
						if empty {
							require.Contains(t, f.text, want)
						} else {
							require.True(t, f.hasRange)
							require.Contains(t, sources[f.source-1].Name, want)
							require.NotContains(t, sources[f.source-1].Name, "not delivered")
						}
					}
					require.True(t, found)
					row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
					require.NotContains(t, row.ProtocolEvidence, "payload not delivered")
				})
			}
		}
	}
}
