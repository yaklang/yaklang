package bin_parser

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestISISCanonicalAndLegacyBoundaries(t *testing.T) {
	w, err := hex.DecodeString("831401001101000003010001000100001e001401")
	require.NoError(t, err)
	legacy, err := ParseStructured(w, "isis", "ISIS")
	require.NoError(t, err)
	require.NotEmpty(t, legacy)
	canonical, err := ParseStructured(w, "isis", "ISISFields")
	require.NoError(t, err)
	m := canonical["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, "IS-IS Hello", m["Protocol Name"])
	require.Equal(t, "0100.0100.0100", m["Source System ID"])
	require.EqualValues(t, 30, m["Holding Timer Seconds"])
	require.Equal(t, false, m["Authentication Verified"])
	for cut := range w {
		_, err = ParseStructured(w[:cut], "isis", "ISISFields")
		require.Error(t, err, "cut %d", cut)
	}
	_, err = ParseStructured(append(w, 0), "isis", "ISISFields")
	require.Error(t, err)
}
