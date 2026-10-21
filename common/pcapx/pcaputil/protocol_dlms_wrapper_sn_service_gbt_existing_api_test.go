package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Source-only draft: existing public Session/Feed/query/Close APIs are exercised
// through the already present strict helper. No new runtime symbol is required.
// This child has not compiled or executed this test.
func TestDLMSWrapperSNServiceGBTExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-service-gbt-source", "DLMS_SN_SERVICE_GBT_ROOT", "baseline-controls.json")
	require.Equal(t, "7460db2e6829dac1b878ee38ee93773ef411de51a478bf049961cf9e52eafda2", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 3)
	out := make([]snWriteControl, 0, len(doc.Cases))
	for _, c := range doc.Cases {
		c["metadata"] = c["steps"]
		encoded, err := json.Marshal(c)
		require.NoError(t, err)
		var control snReadControl
		require.NoError(t, json.Unmarshal(encoded, &control))
		require.NotEmpty(t, control.Steps)
		out = append(out, snWriteControl{snReadControl: control})
	}
	snWriteNative(t, out)
}
