package sharkcli

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"strings"
	"testing"
)

func TestHTTP3ReassembledPayloadOwnsBytesAcrossWorkerJSON(t *testing.T) {
	for _, worker := range []bool{false, true} {
		payload := bytes.Repeat([]byte("body"), 1444) //5776 bytes, much larger than the completing packet
		session := map[string]any{"Authentication Verified": true, "HTTP3 Stream ID": uint64(99), "Frames": []map[string]any{
			{"Frame Type": "STREAM", "Stream ID": uint64(8), "HTTP3 Frames": []map[string]any{{"Frame Type": "DATA", "DATA Length": uint64(len(payload)), "Payload": payload}}},
			{"Frame Type": "STREAM", "Stream ID": uint64(12), "HTTP3 Frames": []map[string]any{{"Frame Type": "HEADERS", "QPACK Block": []byte{0, 0, 0xd9}}}},
		}}
		if worker {
			w, err := json.Marshal(session)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(w, &session))
		}
		e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Raw: []byte("completing ciphertext packet"), Session: session}
		fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
		found, headers := false, false
		for _, f := range fs {
			if strings.HasPrefix(f.text, "Payload:") {
				found = true
				require.Contains(t, f.text, "5776 bytes")
				require.NotContains(t, f.text, "7704 bytes")
				require.True(t, f.hasRange)
				s := sources[f.source-1]
				require.Equal(t, "HTTP/3 DATA · reassembled · stream 8", s.Name)
				require.Equal(t, payload, s.Data)
				require.Equal(t, 0, f.start)
				require.Equal(t, 5776, f.end)
				clear(payload)
				require.False(t, bytes.Equal(payload, s.Data), "the byte source owns the reassembled payload")
			}
			if strings.HasPrefix(f.text, "QPACK Block:") {
				headers = true
				require.True(t, f.hasRange)
				s := sources[f.source-1]
				require.Equal(t, "HTTP/3 HEADERS · QPACK · stream 12", s.Name)
				require.Equal(t, []byte{0, 0, 0xd9}, s.Data)
			}
		}
		require.True(t, found)
		require.True(t, headers)
	}
}
func TestHTTP3ByteSourcesRequireAuthenticatedAndCorrectFrameContext(t *testing.T) {
	for _, auth := range []bool{false, true} {
		session := map[string]any{"Authentication Verified": auth, "HTTP3 Stream ID": uint64(7), "HTTP3 Frames": []map[string]any{{"Frame Type": "CANCEL_PUSH", "Payload": []byte{1, 2, 3}}, {"Frame Type": "DATA", "Payload": []byte{4, 5, 6}}}}
		e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Session: session, Raw: []byte{9, 9, 9}}
		fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
		n := 0
		for _, f := range fs {
			if strings.HasPrefix(f.text, "Payload:") && f.hasRange {
				n++
				require.Equal(t, "HTTP/3 DATA · reassembled · stream 7", sources[f.source-1].Name)
				require.Equal(t, []byte{4, 5, 6}, sources[f.source-1].Data)
			}
		}
		if auth {
			require.Equal(t, 1, n)
		} else {
			require.Zero(t, n, "unauthenticated bytes must not acquire a plaintext source")
		}
	}
}

func TestQUICWireOctetListsStayCompactAndSeparateFromPlaintext(t *testing.T) {
	wire := make([]any, 4096)
	for i := range wire {
		wire[i] = float64(i % 256)
	}
	e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Fields: map[string]any{"Protected Payload": wire}, Session: map[string]any{"Native Carrier": true, "Authentication Verified": true}, Raw: []byte{1, 2, 3}, SourceBytes: pcaputil.ByteSource{CompletedIn: &pcaputil.PacketReference{Number: 42}, PacketRefs: []pcaputil.PacketReference{{Number: 42}}}}
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	found := false
	for _, f := range fs {
		require.NotContains(t, f.text, "Field view limited")
		require.NotContains(t, f.text, "Protected Payload[")
		if strings.HasPrefix(f.text, "Protected Payload:") {
			found = true
			require.Contains(t, f.text, "4096 bytes")
			require.True(t, f.hasRange)
			s := sources[f.source-1]
			require.Equal(t, "QUIC wire remainder · extracted octets", s.Name)
			require.Len(t, s.Data, 4096)
			require.Equal(t, byte(255), s.Data[255])
		}
	}
	require.True(t, found)
	require.Less(t, len(fs), 40, "opaque bytes do not become thousands of protocol fields")
	for _, v := range []any{float64(-1), float64(256), float64(1.5), "1", nil, json.Number("256")} {
		_, ok := fieldOctetList([]any{v})
		require.False(t, ok)
	}
}
