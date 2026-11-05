package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Projection fixtures exercise the event/worker representation, not wire
// admission. Native protocol authentication has independent packet tests.
func TestDetailScalarSequencesSurviveEventWorkerRepresentation(t *testing.T) {
	for _, protocol := range []string{"http3", "quic", "ssh", "bgp", "dns", "ntp"} {
		t.Run(protocol, func(t *testing.T) {
			e := &pcaputil.ProtocolEvent{Protocol: protocol, Status: "decoded", Session: map[string]any{
				"Authentication Verified": true,
				"Diagnostic Collections": map[string]any{
					"Words":          []string{"HEADERS", "DATA"},
					"Flags":          []bool{true, false},
					"IDs":            []uint64{4, 1<<62 - 4},
					"Signed":         []int32{-1, 2},
					"Float32":        []float32{0.1, 0.5},
					"Array":          [2]uint16{9, 10},
					"Explicit Empty": []string{},
					"Null List":      []string(nil),
					"Empty Rows":     []map[string]any{},
					"Null Rows":      []map[string]any(nil),
				},
			}}
			before, err := json.Marshal(e)
			require.NoError(t, err)
			local, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			var remote pcaputil.ProtocolEvent
			decoder := json.NewDecoder(bytes.NewReader(before))
			decoder.UseNumber() // the actual decode worker preserves exact integers
			require.NoError(t, decoder.Decode(&remote))
			fields, remoteSources := protocolEventDetail([]*pcaputil.ProtocolEvent{&remote})
			require.Equal(t, local, fields)
			require.Equal(t, sources, remoteSources)
			text := detailLines(fields)
			require.Contains(t, text, "Words[0]: HEADERS")
			require.Contains(t, text, "Words[1]: DATA")
			require.Contains(t, text, "Explicit Empty: []")
			require.Contains(t, text, "Empty Rows: []")
			require.NotContains(t, text, "<nil>")
			require.Contains(t, text, "IDs[1]: 4611686018427387900")
			after, err := json.Marshal(e)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestDetailScalarSequenceBoundsAndBinaryOwnership(t *testing.T) {
	for _, limit := range []int{-1, 0, 1, 7, 4096} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			values := make([]uint64, 5000)
			for i := range values {
				values[i] = uint64(i)
			}
			normalized, limited := detailScalarSequence(values, limit)
			require.True(t, limited)
			items := normalized.([]any)
			require.Len(t, items, min(max(0, limit), len(values)))
			for i, item := range items {
				require.Equal(t, uint64(i), item)
			}
			if len(items) > 0 {
				items[0] = uint64(999)
				require.Zero(t, values[0])
			}
		})
	}
	for _, wire := range [][]byte{nil, {}, {0, 0xff, 1}} {
		normalized, limited := detailScalarSequence(wire, 0)
		require.False(t, limited)
		got, ok := normalized.([]byte)
		require.True(t, ok)
		require.Equal(t, wire, got)
		require.True(t, bytes.Equal(wire, got))
	}
}
