package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

// Full independent source+adopted observer baseline. All methods/helpers exist
// in the fixed parent. No new decoder symbol is required for an old runtime
// failure. The existing native helper verifies complete fields/Session/private
// GetFields+Decode/poison/Raw/offset/caller/Flow/IDs/Close/release across eight
// UDP/TCP full/deferred/chunk configurations per case.
func TestDLMSHDLCSNBlocksExistingAPI(t *testing.T) {
	var raw []byte
	if root := os.Getenv("DLMS_HDLC_SN_BLOCK_SOURCE_ROOT"); root != "" {
		var err error
		raw, err = os.ReadFile(filepath.Join(root, "baseline-controls.json"))
		require.NoError(t, err)
	} else {
		raw = finsReadBatch(t, "dlms-hdlc-sn-block-source", "DLMS_HDLC_SN_BLOCK_SOURCE_ROOT", "baseline-controls.json")
	}
	require.Equal(t, "d106ccb1477dd3fb492f54bc1551aa7984683c2d3855c67ef56410aa3e480583", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var typed struct{ Cases []hdlcOpCase }
	var meta struct {
		Cases []struct{ Steps []map[string]any }
	}
	require.NoError(t, json.Unmarshal(raw, &typed))
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Len(t, typed.Cases, 3)
	require.Len(t, meta.Cases, 3)
	cases := make([]hdlcSNReadCase, len(typed.Cases))
	for i, c := range typed.Cases {
		require.Len(t, c.Steps, len(meta.Cases[i].Steps))
		cases[i] = hdlcSNReadCase{hdlcOpCase: c, Metadata: meta.Cases[i].Steps}
	}
	// Each helper-created native session has a fresh analyzer. Observation event
	// references in nested SN fields are logical 1-based Feed IDs here, never SN
	// TransactionID/ResponseTo. Shared-analyzer captures must explicitly remap.
	hdlcSNReadNative(t, cases, 41000, 4059)
}
