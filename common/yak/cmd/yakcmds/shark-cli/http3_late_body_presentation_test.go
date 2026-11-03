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

// Render owned context DTOs as received by a worker. Native validation and
// original-source ownership are tested separately in pcaputil.
func TestHTTP3LateBodyDiagnosticPrecedesWitnessAndKeepsCurrentWireValid(t *testing.T) {
	for _, worker := range []bool{false, true} {
		for _, kind := range []string{"HTTP3 Late Associations", "HTTP3 Resolved Associations"} {
			t.Run(fmt.Sprintf("worker%v/%s", worker, kind), func(t *testing.T) {
				witness := make([]map[string]any, 2000)
				for i := range witness {
					witness[i] = map[string]any{"PDU ID": uint64(i + 1)}
				}
				body := map[string]any{"Observation Verdict": "violation-observed", "Request Method Class": "HEAD", "Length Check": "malformed", "Error": "DATA content forbidden for this response", "Observed DATA Bytes": uint64(280), "Body Coverage Complete": true, "Observed In PDU ID": uint64(3), "Observed DATA Byte Source": map[string]any{"References": witness}}
				session := map[string]any{"Authentication Verified": true, kind: []map[string]any{{"Stream ID": uint64(0), "Request PDU ID": uint64(3), "Response PDU ID": uint64(1), "Response Message Revalidation": body}}, "Headers": []map[string]any{{"Name": ":path", "Value": strings.Repeat("/long", 200)}}}
				if worker {
					wire, err := json.Marshal(session)
					require.NoError(t, err)
					decoder := json.NewDecoder(bytes.NewReader(wire))
					decoder.UseNumber()
					require.NoError(t, decoder.Decode(&session))
				}
				e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Raw: []byte("original current wire"), Session: session}
				fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
				verdict, errorAt := -1, -1
				for i, field := range fields {
					if field.text == "Observation Verdict: violation-observed" {
						verdict = i
						require.False(t, field.hasRange)
					}
					if field.text == "Error: DATA content forbidden for this response" {
						errorAt = i
						require.False(t, field.hasRange)
					}
					if strings.HasPrefix(field.text, "Observed DATA Bytes:") {
						require.False(t, field.hasRange)
					}
				}
				require.GreaterOrEqual(t, verdict, 0)
				require.GreaterOrEqual(t, errorAt, 0)
				require.Less(t, verdict, 25)
				require.Less(t, errorAt, 25, "semantics must survive a large truncated witness tree")
				require.True(t, strings.HasPrefix(applicationEventSummary(e), "earlier response PDU #1 violation: DATA content forbidden"), "long current headers must not hide the earlier response finding")
				require.Equal(t, "decoded", e.Status)
				require.Empty(t, e.Error)
				overview := http3ConversationEvent(e)
				row := mapList(overview.Session[kind])[0]
				m := row["Response Message Revalidation"].(map[string]any)
				require.Equal(t, "violation-observed", m["Observation Verdict"])
				require.NotContains(t, m, "Observed DATA Byte Source")
				m["Observation Verdict"] = "edited"
				require.Equal(t, "violation-observed", mapList(e.Session[kind])[0]["Response Message Revalidation"].(map[string]any)["Observation Verdict"])
				require.Equal(t, []byte("original current wire"), e.Raw)
			})
		}
	}
}

func TestHTTP3AmbiguousLateBodyCannotDeclareAUniqueResponseViolation(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "http3", Session: map[string]any{"HTTP3 Resolved Associations": []map[string]any{{"Stream ID": uint64(0), "Response PDU ID": uint64(1), "Ambiguous": true, "Response Message Revalidation": map[string]any{"Observation Verdict": "violation-observed", "Error": "ambiguous method"}}}}}
	require.NotContains(t, applicationEventSummary(e), "earlier response PDU #1 violation")
	require.Empty(t, e.DisplayFieldValues().FieldMap()["http3.late_message.verdict"])
}
