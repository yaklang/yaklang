package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestHTTP3QPACKWaitingReasonPrecedesPacketAndFrameSummary(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value any
		want  string
	}{
		{"QPACK Field Context", "peer SETTINGS not observed", "QPACK fields pending: peer SETTINGS not observed"},
		{"QPACK Field Context", "0-RTT remembered SETTINGS unconfirmed", "QPACK fields pending: 0-RTT remembered SETTINGS unconfirmed"},
		{"QPACK Blocked", true, "QPACK fields pending: encoder insertions missing from capture"},
		{"QPACK Decoder Context", "peer SETTINGS not observed", "QPACK encoder pending: peer SETTINGS not observed"},
		{"QPACK Resumed Streams", []map[string]any{{"HTTP3 Stream ID": uint64(0)}}, "QPACK replayed retained fields on 1 stream(s)"},
	} {
		t.Run(tc.key+"/"+tc.want, func(t *testing.T) {
			e := &pcaputil.ProtocolEvent{Protocol: "http3", Session: map[string]any{"Authentication Verified": true, "Packet Name": "1-RTT", "Packet Number": uint64(991), tc.key: tc.value}}
			require.True(t, strings.HasPrefix(secureTransportSummary(e), tc.want))
			e.Session["Authentication Verified"] = false
			require.NotContains(t, secureTransportSummary(e), tc.want)
		})
	}
}

func TestHTTP3RetainedQPACKFieldBytesKeepOwnArenaAcrossWorker(t *testing.T) {
	for _, worker := range []bool{false, true} {
		for _, authenticated := range []bool{false, true} {
			t.Run(fmt.Sprintf("worker%v/auth%v", worker, authenticated), func(t *testing.T) {
				session := map[string]any{"Authentication Verified": authenticated,
					"QPACK Resumed Field Contexts": []map[string]any{{"HTTP3 Stream ID": uint64(4), "Status": "context-required", "QPACK Field Context": "required insertions not yet observed", "Field Section Frame Bytes": []byte{1, 4, 3, 0, 0x81, 0x80},
						"Byte Source": map[string]any{"Field Section Frame Bytes": []byte{9, 8, 7}, "Ranges": make([]any, 2000)}}},
					"Unrelated": map[string]any{"Field Section Frame Bytes": []byte{6, 5, 4}}}
				if worker {
					wire, err := json.Marshal(session)
					require.NoError(t, err)
					decoder := json.NewDecoder(bytes.NewReader(wire))
					decoder.UseNumber()
					require.NoError(t, decoder.Decode(&session))
				}
				e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Raw: []byte("later SETTINGS ciphertext"), Session: session}
				fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
				found := 0
				for i, f := range fields {
					if strings.HasPrefix(f.text, "Field Section Frame Bytes:") && f.hasRange {
						found++
						require.True(t, authenticated)
						require.Less(t, i, 30)
						require.Greater(t, f.source, 0)
						s := sources[f.source-1]
						require.Equal(t, "HTTP/3 QPACK field bytes · retained stream 4 · SETTINGS is context", s.Name)
						require.Equal(t, []byte{1, 4, 3, 0, 0x81, 0x80}, s.Data[f.start:f.end])
						require.NotEqual(t, e.Raw, s.Data)
					}
				}
				want := 0
				if authenticated {
					want = 1
				}
				require.Equal(t, want, found)
			})
		}
	}
}
