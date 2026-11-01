package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestHTTP3ConnectionLayoutFaultRemainsVisibleAfterAnIndividuallyDecodedFrame(t *testing.T) {
	fault := map[string]any{"Error Name": "H3_FRAME_ERROR", "Error Code": uint64(0x106), "Error Scope": "connection", "First PDU ID": uint64(1)}
	e := &pcaputil.ProtocolEvent{Protocol: "http3", Transport: "udp", Status: "decoded", Session: map[string]any{"HTTP3 Connection Error": fault}}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Equal(t, "connection-violation-observed", row.ProtocolStatus)
	require.Equal(t, "connection protocol violation observed", row.ProtocolScope)
	require.Contains(t, row.ProtocolEvidence, "peer closure not inferred")
	require.Contains(t, row.Info[:45], "H3_FRAME_ERROR")
	fields := protocolEventFields([]*pcaputil.ProtocolEvent{e})
	require.GreaterOrEqual(t, len(fields), 2)
	require.Contains(t, fields[0].text, "H3_FRAME_ERROR")
	require.LessOrEqual(t, len(fields[0].text), 76)
	require.Contains(t, fields[1].text, "do not certify")
	var live packetPresentation
	live.add(e)
	live.add(&pcaputil.ProtocolEvent{Protocol: "http3", Transport: "udp", Status: "decoded"})
	row = live.apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Equal(t, "connection-violation-observed", row.ProtocolStatus, "incremental publication uses the same scope as batch rendering")
	require.Equal(t, 1, strings.Count(row.Info, "H3_FRAME_ERROR · connection"))
	e.Status, e.Error = "malformed", "HTTP3 H3_FRAME_ERROR"
	row = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Equal(t, "malformed", row.ProtocolStatus, "a new connection violation is not normalized to decoded")
}

func TestHTTP3PartialFrameIPCUsesItsOwnByteArena(t *testing.T) {
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "http3", Transport: "udp", Status: "malformed", Raw: []byte("ciphertext"), Session: map[string]any{"Authentication Verified": true, "Frames": []map[string]any{{"Frame Type": "STREAM", "Stream ID": uint64(4), "HTTP3 Frame Error": map[string]any{"Partial Frame Bytes": []byte{0x40}}}}}}
	wire, err := json.Marshal(e)
	require.NoError(t, err)
	var snapshot pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(wire, &snapshot))
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{&snapshot})
	found := false
	for _, f := range fields {
		if strings.HasPrefix(f.text, "Partial Frame Bytes:") {
			require.True(t, f.hasRange)
			require.Greater(t, f.source, 1, "partial plaintext is not a fabricated span in ciphertext")
			require.Equal(t, []byte{0x40}, sources[f.source-1].Data)
			require.Equal(t, 0, f.start)
			require.Equal(t, 1, f.end)
			require.NotContains(t, f.text, "QA==", "worker Base64 is an encoding, not the byte display")
			found = true
		}
	}
	require.True(t, found)
	packets, _ := top30Replay(t, top30Cases()[7])
	for _, packet := range packets {
		for _, native := range packet.protocolEvents() {
			if native.AuthenticatedQUICApplicationProtocol() != "http3" {
				continue
			}
			native.Session["HTTP3 Wire Only"] = true
			native.Session["HTTP3 Wire Observation"] = "retransmission"
			var live packetPresentation
			live.add(native)
			row := live.apply(packetSummary{Protocol: "UDP", Transport: "udp"})
			require.Equal(t, "HTTP/3", row.Protocol)
			require.Equal(t, "authenticated STREAM observation", row.ProtocolScope)
			require.Contains(t, row.Info, "no application frames replayed")
			require.False(t, bytes.Equal(sources[0].Data, []byte{0x40}))
			native.Session["HTTP3 Wire Observation"] = "prefix-missing"
			native.Session["HTTP3 Stream Kind"] = "client-bidirectional"
			live = packetPresentation{}
			live.add(native)
			row = live.apply(packetSummary{Protocol: "UDP", Transport: "udp"})
			require.Equal(t, "HTTP/3", row.Protocol)
			require.Contains(t, row.Info, "stream prefix missing")
			require.Contains(t, row.Info, "awaiting reassembly")
			require.Contains(t, row.ProtocolEvidence, "message and extension unknown")
			return
		}
	}
	t.Fatal("no native HTTP3 fixture event")
}
