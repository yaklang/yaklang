package sharkcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func gpePresentationEvent() *pcaputil.ProtocolEvent {
	return &pcaputil.ProtocolEvent{Protocol: "ip", Transport: "network", Profile: "network-encapsulation-refusal", Status: "limited", Completeness: "headers", ExpertCode: "VXLANProfileUnselected", Summary: "VXLAN-GPE v0 header VNI=42 next=3; payload profile not selected; inner identity unconfirmed", Session: map[string]any{"Observed Header Layers": []string{"Ethernet", "IPv4"}, "Unselected Carrier": "VXLAN-GPE", "Child Header Identity": "unconfirmed", "VXLAN-GPE Version": uint8(0), "VXLAN-GPE Flags": uint8(12), "VXLAN-GPE VNI": uint32(42), "VXLAN-GPE Next Protocol Present": true, "VXLAN-GPE Next Protocol": uint8(3), "Payload Profile Implemented": false}}
}

func TestVXLANGPEHeaderIdentityAndLimitedPayloadAgreeAcrossInspectors(t *testing.T) {
	wire := udpPacket(t, 4790, append([]byte{12, 0, 0, 3, 0, 0, 42, 0}, []byte("opaque-inner-payload")...))
	child := &pcaputil.ProtocolEvent{Protocol: "dns", Status: "decoded", Summary: "speculative child", Session: map[string]any{"QNAME": "unproved.invalid"}}
	for _, events := range [][]*pcaputil.ProtocolEvent{{gpePresentationEvent()}, {gpePresentationEvent(), child}, {child, gpePresentationEvent()}} {
		raw := &capturedPacket{number: 1, data: wire, ci: gopacket.CaptureInfo{Length: len(wire), CaptureLength: len(wire)}, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
		row := summarize(raw)
		require.Equal(t, "VXLAN-GPE", row.Protocol)
		require.Contains(t, row.Info, "inner identity unconfirmed")
		require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "VXLAN-GPE"}, row.ProtocolStack)
		require.Equal(t, "limited", row.ProtocolStatus)
		require.Equal(t, "tunnel-header", row.Visibility)
		require.Contains(t, row.ProtocolScope, "payload profile unselected")
		require.Empty(t, row.ConnectionProtocol)
		require.Zero(t, row.StreamID)
		request, err := marshalDecodeRequest(decodeRequest{Number: 1, Events: events, Data: wire, Length: len(wire), Link: raw.link})
		require.NoError(t, err)
		var output bytes.Buffer
		require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
		worker, err := decodeWorkerOutput(output.Bytes(), raw)
		require.NoError(t, err)
		for _, got := range []packetDetail{detail(raw), quickDetail(raw), worker} {
			require.Equal(t, "VXLAN-GPE", got.protocol)
			var text strings.Builder
			for _, field := range got.fields {
				text.WriteString(field.text + "\n")
			}
			require.Contains(t, text.String(), "VXLAN-GPE VNI: 42")
			require.Contains(t, text.String(), "Payload Profile Implemented: false")
			for _, unproved := range []string{"unproved.invalid", "DNS", "YAML dissector"} {
				require.NotContains(t, text.String(), unproved)
			}
		}
	}
}

func TestVXLANGPENameRequiresCompleteCoherentNativeHeaderEvidence(t *testing.T) {
	mutations := []func(*pcaputil.ProtocolEvent){
		func(e *pcaputil.ProtocolEvent) { e.ExpertCode = "VXLANHeaderAdmission" },
		func(e *pcaputil.ProtocolEvent) { e.Completeness = "limited" },
		func(e *pcaputil.ProtocolEvent) { e.Session["VXLAN-GPE Flags"] = uint8(4) },
		func(e *pcaputil.ProtocolEvent) { e.Session["VXLAN-GPE Flags"] = uint8(0x1c) },
		func(e *pcaputil.ProtocolEvent) { e.Session["VXLAN-GPE Version"] = uint8(1) },
		func(e *pcaputil.ProtocolEvent) { e.Session["VXLAN-GPE Version"] = "unknown" },
		func(e *pcaputil.ProtocolEvent) { e.Session["VXLAN-GPE VNI"] = 42.5 },
		func(e *pcaputil.ProtocolEvent) { e.Session["VXLAN-GPE Next Protocol"] = uint8(0) },
		func(e *pcaputil.ProtocolEvent) { e.Session["VXLAN-GPE Next Protocol Present"] = false },
		func(e *pcaputil.ProtocolEvent) { e.Session["VXLAN-GPE VNI"] = uint32(0x1000000) },
		func(e *pcaputil.ProtocolEvent) { delete(e.Session, "VXLAN-GPE VNI") },
		func(e *pcaputil.ProtocolEvent) { e.Session["Child Header Identity"] = "confirmed" },
		func(e *pcaputil.ProtocolEvent) { e.Session["Payload Profile Implemented"] = true },
	}
	for i, mutate := range mutations {
		e := gpePresentationEvent()
		mutate(e)
		require.False(t, validVXLANGPEHeaderObservation(e), "control %d", i)
		require.NotEqual(t, "VXLAN-GPE", presentEvents([]*pcaputil.ProtocolEvent{e}).protocol)
	}
}
