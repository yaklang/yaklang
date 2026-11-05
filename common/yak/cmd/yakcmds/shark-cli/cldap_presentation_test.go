package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func cldapPresentationEvent(t *testing.T, combined bool) *pcaputil.ProtocolEvent {
	t.Helper()
	w := []byte{0x30, 0x0c, 0x02, 0x01, 0x09, 0x65, 0x07, 0x0a, 0x01, 0x00, 0x04, 0x00, 0x04, 0x00}
	if combined {
		w = append([]byte{0x30, 0x09, 0x02, 0x01, 0x09, 0x64, 0x04, 0x04, 0x00, 0x30, 0x00}, w...)
	}
	var event *pcaputil.ProtocolEvent
	a, err := pcaputil.NewPacketAnalyzer(pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { event = e }))
	require.NoError(t, err)
	defer a.Close()
	frame := udpPacket(t, 389, w)
	require.NoError(t, a.Feed(frame, gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 0), Length: len(frame), CaptureLength: len(frame)}, layers.LinkTypeEthernet))
	require.NotNil(t, event)
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	var ipc pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(encoded, &ipc))
	return &ipc
}

func TestCLDAPUnsupportedResponseKeepsItsValidatedEnvelopeIdentity(t *testing.T) {
	for _, combined := range []bool{false, true} {
		t.Run(map[bool]string{true: "entry-and-done", false: "done"}[combined], func(t *testing.T) {
			e := cldapPresentationEvent(t, combined)
			require.Equal(t, "context-required", e.Status)
			r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv4", "UDP"}})
			require.Equal(t, "CLDAP", r.Protocol)
			require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "CLDAP"}, r.ProtocolStack)
			require.Equal(t, "headers", r.Visibility)
			require.Equal(t, "context-required", r.ProtocolStatus)
			require.Contains(t, r.Info, "SearchResultDone")
			require.Contains(t, r.Info, "response body unparsed")
			require.Contains(t, r.ProtocolEvidence, "peer unverified")
			require.Zero(t, e.FlowID)
			require.Zero(t, e.ResponseTo)
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.NotEmpty(t, fields)
			require.NotEmpty(t, sources)
			for _, f := range fields {
				if strings.Contains(f.text, "Opaque:") || strings.Contains(f.text, "Response Decoded:") {
					require.False(t, f.hasRange, "unsupported-body metadata must not fabricate a byte range")
				}
			}
			packet := detail(&capturedPacket{number: 1, events: []*pcaputil.ProtocolEvent{e}})
			worker := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false)
			require.Equal(t, packet.fields, worker.fields)
			require.Equal(t, packet.sources, worker.sources)
		})
	}
}

func TestCLDAPEnvelopePresentationRejectsUnprovedOrContradictoryMetadata(t *testing.T) {
	for _, kind := range []string{"protocol", "profile", "transport", "status", "expert", "completeness", "truncated", "tail", "fields", "decoded", "flow", "response", "transaction"} {
		t.Run(kind, func(t *testing.T) {
			e := cldapPresentationEvent(t, true)
			switch kind {
			case "protocol":
				e.Protocol = "ldap"
			case "profile":
				e.Profile = "cldap-port-only"
			case "transport":
				e.Transport = "tcp"
			case "status":
				e.Status = "malformed"
			case "expert":
				e.ExpertCode = ""
			case "completeness":
				e.Completeness = "message"
			case "truncated":
				e.Raw = e.Raw[:len(e.Raw)-1]
			case "tail":
				e.Raw = append(bytes.Clone(e.Raw), 0)
			case "fields":
				e.Session["Message ID"] = 123
			case "decoded":
				e.Session["Response Decoded"] = true
			case "flow":
				e.TCPFlowID = 7
			case "response":
				e.ResponseTo = 8
			case "transaction":
				e.TransactionID = 9
			}
			require.Empty(t, cldapResponseEnvelopeSummary(e))
		})
	}
}
