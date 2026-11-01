package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestESPCanonicalHeaderEntryAndLegacyCompatibility(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 0, 0x45, 0, 0, 20}
	v, err := ParseStructured(w, "ipsec_esp", "ESPHeaderFields")
	require.NoError(t, err)
	m := v["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, uint32(0x12345678), m["SPI"])
	require.Equal(t, uint32(0), m["Sequence"])
	require.Equal(t, w[8:], m["Opaque Body"])
	require.Equal(t, false, m["Authentication Verified"])
	require.NotContains(t, m, "Ciphertext")
	require.Equal(t, false, m["Algorithm Context Available"])
	require.Equal(t, false, m["Extended Sequence Reconstructed"])
	zeroSPI := append([]byte(nil), w...)
	clear(zeroSPI[:4])
	_, err = ParseStructured(zeroSPI, "ipsec_esp", "ESPHeaderFields")
	require.Error(t, err, "reserved SPI cannot become a successful ESP observation")
	for cut := 0; cut < 8; cut++ {
		_, err = ParseStructured(w[:cut], "ipsec_esp", "ESPHeaderFields")
		require.Error(t, err)
	}
	legacy, err := ParseStructured(w, "ipsec", "ESP")
	require.NoError(t, err)
	l := legacy["fields"].(map[string]any)
	require.Contains(t, l, "Ciphertext", "legacy opaque-body API remains available")
	require.NotContains(t, l, "ESPHeaderFields", "adding a top-level entry cannot extend the default package")
}
