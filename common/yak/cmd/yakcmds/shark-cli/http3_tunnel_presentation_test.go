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

func TestHTTP3TunnelFindingsPrecedeWitnessesAndKeepObservationPhases(t *testing.T) {
	for _, worker := range []bool{false, true} {
		for _, kind := range []string{"HTTP3 Tunnel Observations", "HTTP3 Resolved Tunnel Observations"} {
			t.Run(fmt.Sprintf("worker%v/%s", worker, kind), func(t *testing.T) {
				refs := make([]map[string]any, 2000)
				for i := range refs {
					refs[i] = map[string]any{"Packet": uint64(i + 1)}
				}
				fault := map[string]any{"Error Name": "H3_FRAME_UNEXPECTED", "Error Code": uint64(0x105), "Error Scope": "connection", "Observation Verdict": "violation-observed", "Frame PDU ID": uint64(11), "Observed In PDU ID": uint64(15), "Frame Type": "HEADERS", "Frame Type Value": uint64(1), "Frame Offset": uint64(8), "Frame Header Bytes": []byte{1}, "Byte Source": map[string]any{"Packet References": refs}}
				session := map[string]any{kind: []map[string]any{fault}, "Headers": []map[string]any{{"Name": ":path", "Value": strings.Repeat("/long", 100)}}}
				if worker {
					data, err := json.Marshal(session)
					require.NoError(t, err)
					d := json.NewDecoder(bytes.NewReader(data))
					d.UseNumber()
					require.NoError(t, d.Decode(&session))
				}
				e := &pcaputil.ProtocolEvent{ID: 15, Protocol: "http3", Status: "decoded", Raw: []byte("current valid packet"), Session: session}
				fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
				found := map[string]bool{}
				for i, field := range fields {
					for _, value := range []string{"Error Name: H3_FRAME_UNEXPECTED", "Error Code: 261", "Error Scope: connection", "Frame PDU ID: 11", "Observed In PDU ID: 15"} {
						if field.text == value {
							found[value] = true
							require.False(t, field.hasRange)
							require.Less(t, i, 30)
						}
					}
				}
				require.Len(t, found, 5)
				require.True(t, strings.HasPrefix(applicationEventSummary(e), "CONNECT H3_FRAME_UNEXPECTED · frame PDU #11 · observed PDU #15"))
				require.Equal(t, "decoded", e.Status)
				require.Empty(t, e.Error)
				overview := http3ConversationEvent(e)
				row := mapList(overview.Session[kind])[0]
				require.Equal(t, uint64(11), number(row["Frame PDU ID"]))
				require.NotContains(t, row, "Byte Source")
				require.NotContains(t, row, "Frame Header Bytes")
				row["Frame PDU ID"] = uint64(999)
				require.Equal(t, uint64(11), number(mapList(e.Session[kind])[0]["Frame PDU ID"]))
			})
		}
	}
}

func TestHTTP3TunnelUnverifiedOrderIsNotDisplayedAsAConnectionError(t *testing.T) {
	for _, worker := range []bool{false, true} {
		t.Run(fmt.Sprintf("worker%v", worker), func(t *testing.T) {
			session := map[string]any{"HTTP3 Resolved Tunnel Observations": []map[string]any{{"Observation Verdict": "order-unverified", "Frame Type": "HEADERS", "Frame PDU ID": uint64(11), "Observed In PDU ID": uint64(15), "Frame Header Bytes": []byte{1}}}}
			if worker {
				data, err := json.Marshal(session)
				require.NoError(t, err)
				d := json.NewDecoder(bytes.NewReader(data))
				d.UseNumber()
				require.NoError(t, d.Decode(&session))
			}
			e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Session: session}
			require.True(t, strings.HasPrefix(applicationEventSummary(e), "CONNECT order unverified · frame PDU #11 · success context PDU #15"))
			require.NotContains(t, applicationEventSummary(e), "H3_FRAME_UNEXPECTED")
			fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, field := range fields {
				require.False(t, strings.HasPrefix(field.text, "Error Code:"))
			}
			require.NotContains(t, mapList(http3ConversationEvent(e).Session["HTTP3 Resolved Tunnel Observations"])[0], "Frame Header Bytes")
		})
	}
}
