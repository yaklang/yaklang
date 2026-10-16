package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// These source-model traces keep the GET genuinely pending at the refusal.
// A previously matched GET is not evidence that a resource error retired an owner.
func snWriteResourceOwnerCases(t *testing.T, carrier string) []json.RawMessage {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-write", "DLMS_HDLC_SN_WRITE_TEST_ROOT", "resources.json")
	require.Equal(t, "0064a93d84af66f528e11e9881a71ccdc3e65282e35b5c81c7512a58552f44ab", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 4)
	var out []json.RawMessage
	for _, r := range d.Cases {
		var m struct{ Carrier string }
		require.NoError(t, json.Unmarshal(r, &m))
		require.Contains(t, []string{"wrapper", "hdlc"}, m.Carrier)
		if m.Carrier == carrier {
			out = append(out, r)
		}
	}
	require.Len(t, out, 2)
	return out
}
func snWriteResourceWrapperControls(t *testing.T) []snWriteControl {
	var cases []snWriteControl
	for _, raw := range snWriteResourceOwnerCases(t, "wrapper") {
		var c snReadControl
		require.NoError(t, json.Unmarshal(raw, &c))
		require.Len(t, c.Steps, 3)
		cases = append(cases, snWriteControl{snReadControl: c})
		// The resource traces use the fixed Get15 prime, not case2's Get1.
		// Reuse exactly the same normal pair from the selected original case.
		index := 8
		if c.Budget.Depth > 0 {
			require.Equal(t, 5, c.Budget.Depth)
			index = 9
		} else {
			require.Equal(t, 3, c.Budget.Collection)
		}
		adjacent := snWriteControls(t)[index]
		require.GreaterOrEqual(t, len(adjacent.Steps), 2)
		adjacent.Steps = append([]map[string]any(nil), adjacent.Steps[:2]...)
		adjacent.Overrides = nil
		require.Empty(t, adjacent.Close.Events)
		require.Empty(t, adjacent.TCPClose.Events)
		require.Len(t, adjacent.Steps, 2)
		require.Equal(t, c.Steps[0]["input_hex"], adjacent.Steps[0]["input_hex"])
		require.Equal(t, c.Steps[2]["input_hex"], adjacent.Steps[1]["input_hex"])
		adjacent.Name = c.Name + "-adjacent-GET-same-budget"
		adjacent.Budget = c.Budget
		cases = append(cases, adjacent)
	}
	return cases
}
func TestDLMSWrapperSNWriteResourcePendingOwner(t *testing.T) {
	snWriteNative(t, snWriteResourceWrapperControls(t))
}

func snWriteResourceHDLCControls(t *testing.T) []hdlcSNReadCase {
	var cases []hdlcSNReadCase
	for _, raw := range snWriteResourceOwnerCases(t, "hdlc") {
		var c hdlcOpCase
		var m struct{ Steps []map[string]any }
		require.NoError(t, json.Unmarshal(raw, &c))
		require.NoError(t, json.Unmarshal(raw, &m))
		require.Len(t, c.Steps, 3)
		require.Len(t, m.Steps, 3)
		cases = append(cases, hdlcSNReadCase{hdlcOpCase: c, Metadata: m.Steps})
		adjacent := hdlcSNWriteControls(t)[2]
		require.Len(t, adjacent.Steps, 2)
		require.Equal(t, c.Steps[0].Wire, adjacent.Steps[0].Wire)
		require.Equal(t, c.Steps[2].Wire, adjacent.Steps[1].Wire)
		adjacent.Name = c.Name + "-adjacent-GET-same-budget"
		adjacent.Budget = c.Budget
		cases = append(cases, adjacent)
	}
	return cases
}
func TestDLMSHDLCSNWriteResourcePendingOwner(t *testing.T) {
	hdlcSNReadNative(t, snWriteResourceHDLCControls(t), 41000, 4059)
}
