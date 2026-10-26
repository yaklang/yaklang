package sharkcli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestUDPCarrierFailureIsVisibleWithoutInventingAnApplication(t *testing.T) {
	e := &pcaputil.ProtocolEvent{ID: 1, Transport: "udp", Status: "malformed", Completeness: "malformed", ExpertCode: string(pcaputil.ErrMalformedMessage), Summary: "UDP length 24593 exceeds declared IP payload available to UDP (80 bytes)", Error: "MalformedMessage: UDP length 24593 exceeds declared IP payload available to UDP (80 bytes)"}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Transport: "udp", Layers: []string{"Ethernet", "IPv4", "UDP"}, Info: "9996 → 1 Payload=72"})
	require.Equal(t, "UDP", row.Protocol)
	require.Equal(t, "malformed", row.ProtocolStatus)
	require.Contains(t, row.Info, "UDP carrier")
	require.Contains(t, row.Info, "24593")
	require.Contains(t, row.Info, "80 bytes")
	require.NotContains(t, row.ProtocolStack, "NetFlow v5")
	fs, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	var labels []string
	for _, f := range fs {
		labels = append(labels, f.text)
	}
	detail := strings.Join(labels, "\n")
	require.Contains(t, detail, "UDP carrier diagnostic")
	require.Contains(t, detail, "Carrier failure: MalformedMessage")
	require.Contains(t, detail, "Application identity unavailable")
	p := &capturedPacket{observedFieldBudget: observedPacketFieldBytes}
	p.recordObservedPDU(e)
	b, err := json.Marshal(p.observedPDUs)
	require.NoError(t, err)
	require.Contains(t, string(b), `"error":"MalformedMessage: UDP length 24593`)
	require.Contains(t, string(b), `"protocol":""`)
}
