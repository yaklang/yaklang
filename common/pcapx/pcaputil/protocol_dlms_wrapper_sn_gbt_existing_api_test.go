package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Complete source-derived answers through existing public session/query APIs.
func TestDLMSWrapperSNGBTExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-gbt-source", "DLMS_SN_GBT_TEST_ROOT", "baseline-controls.json")
	require.Equal(t, "00cbbd68fbcd332829b74c0c27cf07fab1643ebd67dc83f39f7014838f844e9a", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 3)
	out := make([]snWriteControl, 0, 3)
	for _, c := range doc.Cases {
		// Explicit loader projection only: the strict reusable API helper names its
		// source steps metadata. No fields, answer or transport expectations change.
		c["metadata"] = c["steps"]
		encoded, err := json.Marshal(c)
		require.NoError(t, err)
		var controls snReadControl
		require.NoError(t, json.Unmarshal(encoded, &controls))
		require.NotEmpty(t, controls.Steps)
		out = append(out, snWriteControl{snReadControl: controls})
	}
	snWriteNative(t, out)
}
