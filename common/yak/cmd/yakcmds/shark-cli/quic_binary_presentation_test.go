package sharkcli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestQUICCIDDetailsKeepBinaryOctetsAndShortHeaderAbsence(t *testing.T) {
	for _, protocol := range []string{"quic", "http3", "doq"} {
		e := &pcaputil.ProtocolEvent{Protocol: protocol, Transport: "udp", Status: "decoded", Raw: []byte{0x40, 0xff, 0, 0xc3}, Session: map[string]any{"Header Form": "short", "Version": uint32(0), "DCID": []byte{0xff, 0, 0xc3}, "SCID": []byte(nil), "Wire Field Ranges": map[string]any{"DCID": []any{1, 4}}}}
		encoded, err := json.Marshal(e)
		require.NoError(t, err)
		var ipc pcaputil.ProtocolEvent
		require.NoError(t, json.Unmarshal(encoded, &ipc))
		fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{&ipc})
		seen := map[string]bool{}
		for _, f := range fields {
			if strings.HasPrefix(f.text, "DCID:") {
				require.Contains(t, f.text, "ff 00 c3", "binary CID survives worker IPC and is readable as octets")
				require.True(t, f.hasRange)
				require.Equal(t, []byte{0xff, 0, 0xc3}, sources[f.source-1].Data[f.start:f.end])
				seen["CID"] = true
			}
			for _, name := range []string{"Version", "SCID"} {
				if strings.HasPrefix(f.text, name+":") {
					require.Contains(t, f.text, "not carried in short header")
					require.False(t, f.hasRange)
					seen[name] = true
				}
			}
		}
		require.Len(t, seen, 3)
	}
}

func TestQUICLongHeaderEmptyCIDIsPresentRatherThanUnknown(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "quic", Transport: "udp", Status: "context-required", Raw: []byte{0xc0, 0, 0, 0, 1, 0, 0}, Session: map[string]any{"Header Form": "long", "Version": uint32(1), "DCID": []byte(nil), "SCID": []byte(nil), "Wire Field Ranges": map[string]any{"DCID": []any{6, 6}, "SCID": []any{7, 7}}}}
	fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	seen := 0
	for _, f := range fields {
		if strings.HasPrefix(f.text, "DCID:") || strings.HasPrefix(f.text, "SCID:") {
			require.Contains(t, f.text, "0 bytes (empty connection ID)")
			require.False(t, f.hasRange, "an empty identifier does not highlight unrelated bytes")
			seen++
		}
	}
	require.Equal(t, 2, seen)
}

func TestQUICWireVersionDetailsStayLiteralAcrossWorkerIPC(t *testing.T) {
	for _, tc := range []struct {
		version uint32
		text    string
	}{{1, "QUIC v1 (0x00000001)"}, {0x6b3343cf, "QUIC v2 (0x6b3343cf)"}, {0, "Version Negotiation (0x00000000)"}, {2, "Unknown QUIC version (0x00000002)"}} {
		e := &pcaputil.ProtocolEvent{Protocol: "quic", Raw: []byte{0xc0, byte(tc.version >> 24), byte(tc.version >> 16), byte(tc.version >> 8), byte(tc.version), 0, 0}, Session: map[string]any{"Header Form": "long", "Version": tc.version, "Wire Field Ranges": map[string]any{"Version": []any{1, 5}}}}
		encoded, err := json.Marshal(e)
		require.NoError(t, err)
		var ipc pcaputil.ProtocolEvent
		require.NoError(t, json.Unmarshal(encoded, &ipc))
		fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{&ipc})
		seen := false
		for _, f := range fields {
			if strings.HasPrefix(f.text, "Version:") {
				require.Equal(t, "Version: "+tc.text, f.text)
				require.True(t, f.hasRange)
				require.Equal(t, e.Raw[1:5], sources[f.source-1].Data[f.start:f.end])
				seen = true
			}
		}
		require.True(t, seen)
	}
}
