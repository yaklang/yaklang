package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRIPngCanonicalAndLegacyBoundaries(t *testing.T) {
	w := make([]byte, 24)
	w[0], w[1], w[23] = 1, 1, 16
	for _, entry := range []string{"RIPngFields", "RIPng"} {
		v, err := ParseStructured(w, "ripng", entry)
		require.NoError(t, err)
		require.NotEmpty(t, v)
		for cut := 0; cut < len(w); cut++ {
			if cut == 4 {
				continue
			}
			_, err = ParseStructured(w[:cut], "ripng", entry)
			require.Error(t, err)
		}
	}
	v, err := ParseStructured(w, "ripng", "RIPngFields")
	require.NoError(t, err)
	m := v["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, true, m["Whole Table Request"])
}
