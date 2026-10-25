package bin_parser

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRoutingControlCanonicalEntries(t *testing.T) {
	for _, tc := range []struct {
		entry string
		wire  []byte
		key   string
		value uint64
	}{
		{"HSRPMessageFields", []byte{0, 0, 16, 3, 10, 100, 1, 0, 'c', 'i', 's', 'c', 'o', 0, 0, 0, 192, 0, 2, 1}, "Priority", 100},
		{"GLBPMessageFields", []byte{1, 0, 0, 1, 0, 0, 0, 1, 2, 3, 4, 5, 2, 20, 1, 32, 0, 100, 50, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0xb4, 0, 1, 1}, "Group", 1},
		{"BFDMessageFields", []byte{0x20, 0x40, 3, 24, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0, 20, 0, 0, 0, 30}, "My Discriminator", 1},
	} {
		t.Run(tc.entry, func(t *testing.T) {
			v, err := ParseStructured(tc.wire, "routing_control", tc.entry)
			require.NoError(t, err)
			m := v["fields"].(map[string]any)["Message"].(map[string]any)
			require.Equal(t, tc.value, m[tc.key])
			for i := 0; i < len(tc.wire); i++ {
				_, err = ParseStructured(tc.wire[:i], "routing_control", tc.entry)
				require.Error(t, err, "cut %d", i)
			}
			bad := append([]byte(nil), tc.wire...)
			bad[0] = 255
			_, err = ParseStructured(bad, "routing_control", tc.entry)
			require.Error(t, err)
		})
	}
}

// Each exception is tied to an executable bounded codec. Historical HSRP
// "done" and GLBP/BFD/LDP "todo" scores stay immutable, while today's coverage is
// accurately partial. No unrelated partial protocol bypasses roadmap checks.
func requireRoutingControlScopes(t *testing.T) map[string]bool {
	t.Helper()
	result := map[string]bool{}
	for _, name := range []string{"HSRP", "GLBP", "BFD", "LDP", "PIM"} {
		found := false
		for _, row := range ProtocolRoadmap {
			if row.Name == name {
				require.Equal(t, stPartial, row.Status)
				require.NotEmpty(t, row.Notes)
				found = true
			}
		}
		require.True(t, found)
		found = false
		for _, row := range ProtocolCatalog {
			if row.Name == name {
				require.Equal(t, statusPartial, row.Status)
				require.NotEmpty(t, row.Notes)
				found = true
			}
		}
		require.True(t, found)
		if name == "PIM" {
			result[name] = t.Run("PIMBoundedControlScope", TestPIMCanonicalControlFields)
		} else if name == "LDP" {
			result[name] = t.Run("LDPBoundedDiscoveryScope", TestLDPDiscoveryCanonicalCompleteMessagesAndBoundaries)
			result[name] = t.Run("LDPBoundedTCPScope", TestLDPTCPCanonicalCompleteMessages) && result[name]
		} else {
			result[name] = t.Run(name+"BoundedRoutingScope", TestRoutingControlCanonicalEntries)
		}
	}
	return result
}
