package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Existing production API and preexisting strict normal-operation helpers only.
// This external draft has not been compiled or executed by its author.
func TestDLMSLNAssembledOpaqueFinalRetryExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-ln-assembled-retry-source", "DLMS_LN_ASSEMBLED_RETRY_ROOT", "controls.json")
	require.Equal(t, "649a95bb7887ce7258095a4789cacf5acb9a2a686ecc2a4a55b21908894c8a2b", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &gold))
	require.Len(t, gold.Cases, 2)
	for _, c := range gold.Cases {
		require.Len(t, c.Steps, 4)
		require.Equal(t, c.Steps[1].Wire, c.Steps[2].Wire)
		require.Equal(t, "retransmitted-request", c.Steps[2].Fields["Association"])
		require.Equal(t, 2, c.Steps[3].Response)
		require.Empty(t, c.Close.Events)
	}
	hdlcOpNative(t, gold.Cases, 41000, 4059)
}
