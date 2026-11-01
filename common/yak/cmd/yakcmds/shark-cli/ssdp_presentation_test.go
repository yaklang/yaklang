package sharkcli

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"strings"
	"testing"
)

func TestSSDPProgressivePresentationAndByteSources(t *testing.T) {
	w := []byte("NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nNT: upnp:rootdevice\r\nNTS: ssdp:alive\r\nUSN: uuid:test::upnp:rootdevice\r\nLOCATION: http://192.0.2.1/d.xml\r\nCACHE-CONTROL: max-age=66\r\nSERVER: Audit UPnP/1.0\r\n\r\n")
	s, err := stream_parser.DecodeSSDPMessage(w)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "ssdp", Status: "decoded", Raw: w, Session: s}
	r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv4", "UDP"}})
	require.Equal(t, "SSDP", r.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "SSDP"}, r.ProtocolStack)
	require.Contains(t, r.Info, "NOTIFY")
	require.Contains(t, r.Info, "max-age=66")
	require.NotContains(t, r.Info, "uuid:test")
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	found := false
	for _, f := range fields {
		if strings.HasPrefix(f.text, "LOCATION:") {
			require.True(t, f.hasRange)
			require.Equal(t, " http://192.0.2.1/d.xml", string(sources[f.source-1].Data[f.start:f.end]))
			found = true
		}
	}
	require.True(t, found)
	e.Status, e.Error = "incomplete", "SSDP header block incomplete"
	r = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP"})
	require.Equal(t, "UDP", r.Protocol)
}
func TestTLSRecoveredPlaintextRemainsAnExplicitIndependentByteSource(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "tls", Status: "decoded", Raw: []byte{23, 3, 3, 0, 17}, Session: map[string]any{"Negotiated Version": uint16(0x304), "Authentication Verified": true, "Authenticated Plaintext": []byte("intact after a gap"), "Application Decoding": "unavailable across prior plaintext gap"}}
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	found := false
	for _, f := range fields {
		if strings.HasPrefix(f.text, "Authenticated Plaintext:") {
			require.True(t, f.hasRange)
			require.Contains(t, sources[f.source-1].Name, "authenticated plaintext")
			require.Equal(t, []byte("intact after a gap"), sources[f.source-1].Data)
			found = true
		}
	}
	require.True(t, found)
	e.Session["Record Recovery Verified"] = true
	require.Contains(t, secureTransportSummary(e), "record recovered")
	delete(e.Session, "Record Recovery Verified")
	require.Contains(t, secureTransportSummary(e), "application gap")
}

func TestKnownQUICAuthenticationFailurePreservesOnlyTheCarrier(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "quic", FlowID: 3, Status: "malformed", ExpertCode: string(pcaputil.ErrAuthenticationFailed), Error: "QUIC packet protection authentication failed", Session: map[string]any{"Authentication Verified": false}}
	r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"IPv4", "UDP"}})
	require.Equal(t, "QUIC", r.Protocol)
	require.Contains(t, r.ProtocolEvidence, "authentication failed")
	require.Equal(t, []string{"IPv4", "UDP", "QUIC"}, r.ProtocolStack)
	require.Equal(t, "encrypted", r.Visibility)
	e.FlowID = 0
	r = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP"})
	require.Equal(t, "UDP", r.Protocol, "an unassociated failure cannot establish connection identity")
}

func TestTLSMixedRecordOutcomesAreExplicitInListAndDetails(t *testing.T) {
	bad := &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "malformed", ExpertCode: string(pcaputil.ErrAuthenticationFailed), Error: "bad tag", Session: map[string]any{"Negotiated Version": uint16(0x304), "Authentication Verified": false}}
	good := &pcaputil.ProtocolEvent{ID: 2, Protocol: "tls", Status: "decoded", Session: map[string]any{"Negotiated Version": uint16(0x304), "Authentication Verified": true, "Record Recovery Verified": true, "Record Type": 23}}
	events := []*pcaputil.ProtocolEvent{bad, good}
	r := presentEvents(events).apply(packetSummary{Protocol: "TCP"})
	require.Equal(t, "mixed-record-outcomes", r.ProtocolStatus)
	require.True(t, strings.HasPrefix(r.Info, "Includes rejected TLS record"))
	require.Contains(t, r.Info, "record recovered")
	fields, _ := protocolEventDetail(events)
	require.Contains(t, fields[0].text, "1 authenticated, 1 rejected")
	var text strings.Builder
	for _, f := range fields {
		text.WriteString(f.text + "\n")
	}
	require.Less(t, strings.Index(text.String(), "PDU #2"), strings.Index(text.String(), "PDU #1"))
}
