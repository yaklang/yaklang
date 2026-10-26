package sharkcli

import (
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"strings"
	"testing"
)

func TestIPFIXProgressiveFieldsAndPDUByteRanges(t *testing.T) {
	w, err := hex.DecodeString("000a00246553f1000000025b000000130002000c010000010008000401000008c0000209")
	require.NoError(t, err)
	d, err := stream_parser.DecodeIPFIXWire(w, nil, "udp", 128)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "ipfix", Status: "decoded", Profile: "ipfix-directed-UDP-template-observer", Raw: w, Session: d.Fields, Summary: "IPFIX · 1 template, 1 data record"}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"IPv4", "UDP"}})
	require.Equal(t, "IPFIX", row.Protocol)
	require.Equal(t, []string{"IPv4", "UDP", "IPFIX"}, row.ProtocolStack)
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	found := false
	for _, f := range fs {
		if strings.HasPrefix(f.text, "Reported Source IPv4: 192.0.2.9") {
			found = true
			require.True(t, f.hasRange)
			require.Equal(t, 32, f.start)
			require.Equal(t, 36, f.end)
			require.Equal(t, []byte{192, 0, 2, 9}, sources[f.source-1].Data[f.start:f.end])
		}
		if strings.Contains(f.text, "Template PDU SHA256") {
			require.False(t, f.hasRange)
		}
	}
	require.True(t, found)
	unknown := append([]byte(nil), w[:16]...)
	unknown = append(unknown, w[28:]...)
	unknown[3] = byte(len(unknown))
	d, err = stream_parser.DecodeIPFIXWire(unknown, nil, "udp", 128)
	require.Error(t, err)
	e.Raw, e.Session, e.Status, e.Error = unknown, d.Fields, "context-required", err.Error()
	row = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"IPv4", "UDP"}})
	require.Equal(t, "IPFIX", row.Protocol)
	require.Equal(t, "headers", row.Visibility)
}
