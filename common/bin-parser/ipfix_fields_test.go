package bin_parser

import (
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestIPFIXCanonicalCompleteSelfContainedMessage(t *testing.T) {
	w, err := hex.DecodeString("000a00246553f1000000025b000000130002000c010000010008000401000008c0000209")
	require.NoError(t, err)
	x, err := ParseStructured(w, "ipfix", "IPFIXFields")
	require.NoError(t, err)
	m := x["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, "IPFIX", m["Protocol Name"])
	require.Equal(t, uint64(1), m["Data Record Count"])
	for cut := range w {
		_, err = ParseStructured(w[:cut], "ipfix", "IPFIXFields")
		require.Error(t, err)
	}
}
