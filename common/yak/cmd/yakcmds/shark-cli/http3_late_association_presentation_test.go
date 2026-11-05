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

// These are presentation DTOs, not native authentication/association admission.
// The pcaputil tests exercise the native protected packets and retained view.
func TestHTTP3LateAssociationPresentationKeepsContextOutOfWireRanges(t *testing.T) {
	for _, worker := range []bool{false, true} {
		for _, resolved := range []bool{false, true} {
			t.Run(fmt.Sprintf("worker%v/resolved%v", worker, resolved), func(t *testing.T) {
				kind := "HTTP3 Late Associations"
				if resolved {
					kind = "HTTP3 Resolved Associations"
				}
				link := map[string]any{"Stream ID": uint64(4), "Request PDU ID": uint64(12), "Request Headers PDU ID": uint64(13), "Response PDU ID": uint64(11), "Observed In PDU ID": uint64(13), "Association Scope": "validated header fields only", "Response Header Packet References": []map[string]any{{"Packet": uint64(8)}}}
				session := map[string]any{"Authentication Verified": true, kind: []map[string]any{link}, "HTTP3 Association View": map[string]any{"Complete": true, "Scope": "later retained view"}}
				if worker {
					wire, err := json.Marshal(session)
					require.NoError(t, err)
					decoder := json.NewDecoder(bytes.NewReader(wire))
					decoder.UseNumber()
					require.NoError(t, decoder.Decode(&session))
				}
				e := &pcaputil.ProtocolEvent{Protocol: "http3", Status: "decoded", Raw: []byte("encrypted original wire"), Session: session}
				fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
				found := false
				for _, field := range fields {
					if strings.HasPrefix(field.text, "Request PDU ID:") || strings.HasPrefix(field.text, "Response PDU ID:") {
						found = true
						require.False(t, field.hasRange, "later context is not a ciphertext byte offset")
					}
				}
				require.True(t, found)
				info := applicationEventSummary(e)
				if resolved {
					require.Contains(t, info, "later request PDU #12: stream=4")
				} else {
					require.Contains(t, info, "late association: stream=4 response PDU #11")
				}
				overview := http3ConversationEvent(e)
				rows := mapList(overview.Session[kind])
				require.Len(t, rows, 1)
				require.Equal(t, uint64(12), number(rows[0]["Request PDU ID"]))
				require.NotContains(t, rows[0], "Response Header Packet References", "large byte witnesses stay in packet detail")
				rows[0]["Request PDU ID"] = uint64(999)
				require.Equal(t, uint64(12), number(mapList(e.Session[kind])[0]["Request PDU ID"]))
				require.Equal(t, []byte("encrypted original wire"), e.Raw)
			})
		}
	}
}

func TestHTTP3AmbiguousLateAssociationCannotPresentAUniqueRequest(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "http3", Session: map[string]any{"HTTP3 Resolved Associations": []map[string]any{{"Stream ID": uint64(4), "Request PDU ID": uint64(12), "Ambiguous": true}}}}
	require.Contains(t, applicationEventSummary(e), "ambiguous request association: stream=4")
	require.NotContains(t, applicationEventSummary(e), "later request PDU #12")
}
