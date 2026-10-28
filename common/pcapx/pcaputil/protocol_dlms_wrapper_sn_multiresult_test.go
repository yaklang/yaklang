package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snMultiResultControls(t *testing.T) []snWriteControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-multiresult-source", "DLMS_SN_MULTIRESULT_ROOT", "controls.json")
	require.Equal(t, "6cbd4623ac896cfa2ec23212d3ab39c4af2178c1e4fece7fda2a0b7a8fc0e519", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []snReadControl }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 18)
	out := make([]snWriteControl, len(d.Cases))
	for i, c := range d.Cases {
		out[i] = snWriteControl{snReadControl: c}
	}
	return out
}

func TestDLMSWrapperSNMultiResultNative(t *testing.T) { snWriteNative(t, snMultiResultControls(t)) }
