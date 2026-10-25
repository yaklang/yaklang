package sharkcli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestRTMPReassembledCommandPrecedesContributingChunks(t *testing.T) {
	es := []*pcaputil.ProtocolEvent{
		{ID: 1, Protocol: "rtmp", Status: "decoded", Summary: "RTMP Echo Handshake", Session: map[string]any{"Packet Name": "Echo Handshake"}},
		{ID: 2, Protocol: "rtmp", Status: "decoded", Summary: "RTMP Chunk", Session: map[string]any{"Packet Name": "Chunk"}},
		{ID: 3, Protocol: "amf0", Status: "decoded", Profile: "rtmp-message", Summary: "RTMP AMF0 Command · connect", CarrierLayers: []string{"RTMP"}, Session: map[string]any{"Packet Name": "AMF0 Command", "Command": "connect", "Message Stream ID": uint32(0), "Chunk Stream ID": uint32(3)}, Raw: []byte{2, 0, 7, 'c', 'o', 'n', 'n', 'e', 'c', 't'}, SourceBytes: pcaputil.ByteSource{Kind: "reassembled", ParentPDUs: []uint64{2}}},
	}
	row := presentEvents(es).apply(packetSummary{Protocol: "TCP", Layers: []string{"Ethernet", "IPv4", "TCP"}})
	require.Equal(t, "AMF0", row.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv4", "TCP", "RTMP", "AMF0"}, row.ProtocolStack)
	require.Contains(t, row.Info, "connect")
	fields, sources := protocolEventDetail(es)
	require.Contains(t, fields[0].text, "AMF0")
	require.Equal(t, es[2].Raw, sources[0].Data)
	command, echo := -1, -1
	for i, f := range fields {
		if strings.HasPrefix(f.text, "Command:") {
			command = i
		}
		if strings.Contains(f.text, "Echo Handshake") {
			echo = i
		}
	}
	require.GreaterOrEqual(t, command, 0)
	require.Greater(t, echo, command)
	// Sorting the details must never mutate capture chronology or parent links.
	require.Equal(t, uint64(1), es[0].ID)
	require.Equal(t, []uint64{2}, es[2].SourceBytes.ParentPDUs)
}
