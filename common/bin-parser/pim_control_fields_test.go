package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPIMCanonicalControlFields(t *testing.T) {
	// RFC7761 Hello: Holdtime, DR priority and Generation ID, not adjacency.
	w := []byte{0x20, 0, 0, 0, 0, 1, 0, 2, 0, 105, 0, 19, 0, 4, 0, 0, 0, 7, 0, 20, 0, 4, 0x12, 0x34, 0x56, 0x78}
	v, err := ParseStructured(w, "pim_control", "PIMControlFields")
	require.NoError(t, err)
	m := v["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, uint64(2), m["Version"])
	require.Equal(t, "Hello", m["Packet Name"])
	require.False(t, m["Checksum Verified"].(bool), "stateless entry has no enclosing IPv6 pseudo-header")
	options := m["Options"].([]map[string]any)
	require.Equal(t, uint64(0x12345678), options[2]["Generation ID"])
	for _, cut := range []int{0, 1, 2, 3, 5, 7, 9, 11, 17, 25} {
		_, err = ParseStructured(w[:cut], "pim_control", "PIMControlFields")
		require.Error(t, err, "cut %d", cut)
	}
	w[0] = 0x10
	_, err = ParseStructured(w, "pim_control", "PIMControlFields")
	require.Error(t, err)
}
