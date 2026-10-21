package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Both messages use the existing session API. The selected START report has a
// URR ID and a report sequence distinct from the transaction header sequence.
func TestPFCPUsageExistingAPIBaseline(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(8805, 8805))
	require.NoError(t, err)
	q := doipDiscoveryWire(t, "2136000c112233445566778812345600")
	r := doipDiscoveryWire(t, "2137002b99aabbccddeeff00123456000013000101004f001600510004010203040068000400000012003f00021000")
	a := s.Feed(0, time.Unix(1, 0), q)
	require.Nil(t, a.Err)
	require.Len(t, a.Events, 1)
	b := s.Feed(1, time.Unix(2, 0), r)
	require.Nil(t, b.Err)
	require.Len(t, b.Events, 1)
	f, err := b.Events[0].GetFields()
	require.NoError(t, err)
	require.Equal(t, a.Events[0].ID, b.Events[0].ResponseTo)
	require.Equal(t, uint32(0x123456), f["sequence"])
	require.Contains(t, f, "usage_report_observations")
	observation := f["usage_report_observations"].([]map[string]any)[0]
	selected := observation["selected_fields"].(map[string]any)
	require.Equal(t, uint32(0x01020304), selected["urr_id"].(map[string]any)["unsigned32_raw"])
	require.Equal(t, uint32(18), selected["ur_sequence"])
	require.Equal(t, true, selected["trigger"].(map[string]any)["flags"].(map[string]any)["START"])
	require.Nil(t, selected["start_time"])
	require.Nil(t, selected["end_time"])
	require.Equal(t, false, observation["actual_usage_verified"])
	require.Equal(t, a.Events[0].ID, b.Events[0].TransactionID)
	require.Empty(t, s.Close("end"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
