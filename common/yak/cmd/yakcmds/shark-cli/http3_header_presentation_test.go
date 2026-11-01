package sharkcli

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"strings"
	"testing"
)

func TestHTTP3LiteralHeaderHasIndependentByteArenaAndExactStreamID(t *testing.T) {
	for _, worker := range []bool{false, true} {
		session := map[string]any{"Authentication Verified": true, "HTTP3 Stream ID": uint64(99), "HTTP3 Frames": []map[string]any{{"Frame Type": "DATA", "Frame Type Value": uint64(0), "Payload Length": uint64(8), "Stream ID": uint64(1<<60) + 13, "Frame Header Bytes": []byte{0x40, 0, 0x40, 8}, "Frame Header Ranges": map[string]any{"Frame Type Value": []any{uint64(0), uint64(2)}, "Frame Type": []any{uint64(0), uint64(2)}, "Payload Length": []any{uint64(2), uint64(4)}}, "Payload": []byte("abcdefgh")}}}
		if worker {
			w, err := json.Marshal(session)
			require.NoError(t, err)
			decoder := json.NewDecoder(bytes.NewReader(w))
			decoder.UseNumber()
			require.NoError(t, decoder.Decode(&session))
		}
		e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Raw: []byte("unrelated ciphertext PDU"), Session: session}
		fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
		found := map[string]bool{}
		for _, f := range fs {
			var want []byte
			switch {
			case strings.HasPrefix(f.text, "Frame Type Value:"):
				want = []byte{0x40, 0}
			case strings.HasPrefix(f.text, "Payload Length:"):
				want = []byte{0x40, 8}
			default:
				continue
			}
			found[f.text] = true
			require.True(t, f.hasRange)
			s := sources[f.source-1]
			require.Equal(t, "HTTP/3 header · stream 1152921504606846989", s.Name)
			require.Equal(t, want, s.Data[f.start:f.end])
			require.NotEqual(t, e.Raw, s.Data)
		}
		require.Len(t, found, 2)
	}
}

func TestHTTP3ApplicationFieldsPrecedeLargeStreamWitnessTree(t *testing.T) {
	proof := make([]map[string]any, 2000)
	for i := range proof {
		proof[i] = map[string]any{"Offset": uint64(i), "Parent PDU": uint64(i + 1)}
	}
	e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Session: map[string]any{"Authentication Verified": true, "Frames": []map[string]any{{"Frame Type": "STREAM", "Stream ID": uint64(8), "Offset": uint64(16), "Delivered Bytes": uint64(10), "Delivered Byte Source": map[string]any{"Ranges": proof}, "HTTP3 Frames": []map[string]any{{"Frame Type": "DATA", "Frame Type Value": uint64(0), "Payload Length": uint64(8), "Stream ID": uint64(8), "Frame Header Bytes": []byte{0, 8}, "Payload": []byte("abcdefgh")}}}}}}
	fs, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	payload, witness := -1, -1
	for i, f := range fs {
		if strings.HasPrefix(f.text, "Payload:") {
			payload = i
		}
		if f.text == "Delivered Byte Source" {
			witness = i
		}
	}
	require.GreaterOrEqual(t, payload, 0, "application content must survive the bounded detail tree")
	require.Greater(t, witness, payload, "evidence follows the application frame")
}
