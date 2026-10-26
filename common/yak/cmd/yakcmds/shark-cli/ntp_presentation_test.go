package sharkcli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestNTPControlUnknownTrailerKeepsValidatedHeaderButMalformedCountDoesNot(t *testing.T) {
	var event *pcaputil.ProtocolEvent
	a, err := pcaputil.NewPacketAnalyzer(pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { event = e }))
	require.NoError(t, err)
	defer a.Close()
	w := []byte{0x16, 0x82, 0, 1, 0, 0, 0, 0, 0, 0, 0, 3, 'a', 'b', 'c', 0, '?'}
	feed := func(w []byte) *pcaputil.ProtocolEvent {
		frame := udpPacket(t, 123, w)
		require.NoError(t, a.Feed(frame, gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 0), Length: len(frame), CaptureLength: len(frame)}, layers.LinkTypeEthernet))
		require.NotNil(t, event)
		encoded, err := json.Marshal(event)
		require.NoError(t, err)
		var ipc pcaputil.ProtocolEvent
		require.NoError(t, json.Unmarshal(encoded, &ipc))
		return &ipc
	}
	e := feed(w)
	require.Equal(t, "context-required", e.Status)
	r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv4", "UDP"}})
	require.Equal(t, "NTP", r.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "NTP"}, r.ProtocolStack)
	require.Equal(t, "headers", r.Visibility)
	require.Equal(t, "context-required", r.ProtocolStatus)
	require.Contains(t, r.ProtocolEvidence, "sender unverified")
	require.Contains(t, r.Info, "trailer")
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	seen := false
	for _, f := range fields {
		if strings.HasPrefix(f.text, "Unselected Trailer Bytes:") {
			require.True(t, f.hasRange)
			require.Equal(t, []byte{'?'}, sources[f.source-1].Data[f.start:f.end])
			seen = true
		}
	}
	require.True(t, seen, "the unsupported tail remains inspectable at its literal offset")
	w[10], w[11] = 1, 213
	e = feed(w)
	require.Equal(t, "malformed", e.Status)
	r = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv4", "UDP"}})
	require.Equal(t, "UDP", r.Protocol)
	require.NotContains(t, r.ProtocolStack, "NTP", "a malformed count is not a complete validated envelope")
}

func TestNTPControlIPCFieldsAndReassembledByteArena(t *testing.T) {
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "ntp", Transport: "udp", Status: "decoded", Raw: []byte{0x16, 0x82, 0, 7, 0, 0, 0, 0, 0, 3, 0, 3, 'd', 'e', 'f'}, Session: map[string]any{"Packet Name": "Control", "Sequence Number": uint16(7), "Fragment Offset": uint16(3), "Data Count": uint16(3), "Control Data": []byte("def"), "Reassembled Control Data": []byte("abcdef"), "Wire Field Ranges": map[string]any{"Sequence Number": []any{2, 4}, "Control Data": []any{12, 15}}}}
	wire, err := json.Marshal(e)
	require.NoError(t, err)
	var snapshot pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(wire, &snapshot))
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{&snapshot})
	foundSequence, foundFragment, foundAssembly := false, false, false
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f.text, "Sequence Number:"):
			require.True(t, f.hasRange)
			require.Equal(t, 2, f.start)
			require.Equal(t, 4, f.end)
			foundSequence = true
		case strings.HasPrefix(f.text, "Control Data:"):
			require.Contains(t, f.text, "64 65 66")
			require.True(t, f.hasRange)
			require.Equal(t, 12, f.start)
			require.Equal(t, 15, f.end)
			require.Equal(t, e.Raw, sources[f.source-1].Data)
			foundFragment = true
		case strings.HasPrefix(f.text, "Reassembled Control Data:"):
			require.True(t, f.hasRange)
			require.Equal(t, []byte("abcdef"), sources[f.source-1].Data)
			require.Equal(t, 0, f.start)
			require.Equal(t, 6, f.end)
			require.Contains(t, sources[f.source-1].Name, "reassembled")
			foundAssembly = true
		}
	}
	require.True(t, foundSequence)
	require.True(t, foundFragment)
	require.True(t, foundAssembly)
}
