package pcaputil

import (
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Owned HPAI byte control: Calimero v2.5.1 SearchRequest/HPAI fixed layout.
// Independent whole-message answer, followed by the public query invariant.
func knxSessionQueriesOwnAcceptedObservation(t *testing.T) {
	t.Helper()
	for _, service := range []string{"0201", "020b"} {
		t.Run(service, func(t *testing.T) {
			raw, err := hex.DecodeString("0610" + service + "000e0801c00002010e57")
			require.NoError(t, err)
			fields := map[string]any{"header_length": 6, "version": 16, "service_type": 513, "total_length": 14, "kind": "SearchRequest", "endpoint": map[string]any{"structure_length": 8, "host_protocol": 1, "ipv4": "192.0.2.1", "port": 3671, "route_back": false, "raw_hex": "0801c00002010e57"}}
			if service == "020b" {
				fields["service_type"] = 523
				fields["kind"] = "SearchRequestExtended"
				fields["srps"] = []any{}
			}
			blob, err := json.Marshal(fields)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(blob, &fields))
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 3671))
			require.NoError(t, err)
			defer s.Close("test")
			r := s.Feed(0, time.Unix(1, 0), raw)
			clear(raw)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Equal(t, "knx", e.Protocol)
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
			rocEqualFields(t, fields, e.Session)
			for _, edit := range []func(){func() { e.Session["kind"] = "forged"; e.Session["endpoint"].(map[string]any)["port"] = 999999 }, func() { e.Session = map[string]any{"forged": true} }, func() { e.Session = nil }} {
				edit()
				f, err := e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, fields, f)
				d, err := e.Decode()
				require.NoError(t, err)
				rocEqualFields(t, fields, d["fields"].(map[string]any))
				rocEqualFields(t, fields, d["session"].(map[string]any))
				clear(d["session"].(map[string]any))
				again, err := e.Decode()
				require.NoError(t, err)
				rocEqualFields(t, fields, again["session"].(map[string]any))
			}
			s.Close("test")
			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
