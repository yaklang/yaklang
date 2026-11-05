package sharkcli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestHTTP3QPACKFieldFailureDoesNotDowngradeLaterPacket(t *testing.T) {
	fault := map[string]any{"Error Name": "QPACK_DECOMPRESSION_FAILED", "Scope": "field section", "Original Field PDU ID": uint64(9)}
	e := &pcaputil.ProtocolEvent{Protocol: "http3", Transport: "udp", Status: "malformed", Error: "QPACK field section decoding failed", Session: map[string]any{"QPACK Field Error": fault}}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{})
	require.Equal(t, "malformed", row.ProtocolStatus)
	require.NotContains(t, row.Info, "connection violation")
	fields := protocolEventFields([]*pcaputil.ProtocolEvent{e})
	require.Contains(t, detailLines(fields), "QPACK Field Error")
	normal := &pcaputil.ProtocolEvent{Protocol: "http3", Transport: "udp", Status: "decoded"}
	row = presentEvents([]*pcaputil.ProtocolEvent{normal}).apply(packetSummary{})
	require.Equal(t, "decoded", row.ProtocolStatus)
	require.NotContains(t, row.Info, "QPACK_DECOMPRESSION_FAILED")
}

func TestHTTP3RejectedQPACKBlockOwnsPlaintextAfterIPC(t *testing.T) {
	want := []byte{3, 0xff, 0x7f, 0x10, 0x11}
	e := &pcaputil.ProtocolEvent{ID: 9, Protocol: "http3", Transport: "udp", Status: "malformed", Raw: []byte("captured ciphertext"), Session: map[string]any{"Authentication Verified": true, "Frames": []map[string]any{{"Frame Type": "STREAM", "Stream ID": uint64(4), "QPACK Field Error": map[string]any{"Error Name": "QPACK_DECOMPRESSION_FAILED", "Stream ID": uint64(4), "Rejected QPACK Block": want}}}}}
	wire, err := json.Marshal(e)
	require.NoError(t, err)
	var worker pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(wire, &worker))
	for _, current := range []*pcaputil.ProtocolEvent{e, &worker} {
		fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{current})
		found := false
		for _, f := range fields {
			if strings.HasPrefix(f.text, "Rejected QPACK Block:") {
				require.True(t, f.hasRange)
				require.Greater(t, f.source, 1)
				require.Equal(t, want, sources[f.source-1].Data)
				require.Equal(t, 0, f.start)
				require.Equal(t, len(want), f.end)
				require.Contains(t, sources[f.source-1].Name, "rejected QPACK")
				found = true
			}
		}
		require.True(t, found)
	}
	e.Session["Authentication Verified"] = false
	fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	for _, f := range fields {
		if strings.HasPrefix(f.text, "Rejected QPACK Block:") {
			require.False(t, f.hasRange, "an unverified diagnostic must not manufacture a plaintext arena")
		}
	}
}
