package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Existing public API only. Independent source/adopted-policy answers, no
// new runtime symbol. Child has not compiled or executed this test.
func TestDLMSWrapperSNUploadGBTExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-upload-gbt-source", "DLMS_SN_UPLOAD_GBT_ROOT", "baseline-controls.json")
	require.Equal(t, "a30ec4e04287b5dfaaae041f5e05be885b2a5504d09870a6afe429338f954a4b", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 3)
	out := make([]snWriteControl, 0, len(doc.Cases))
	for _, c := range doc.Cases {
		c["metadata"] = c["steps"]
		b, err := json.Marshal(c)
		require.NoError(t, err)
		var control snReadControl
		require.NoError(t, json.Unmarshal(b, &control))
		require.NotEmpty(t, control.Steps)
		out = append(out, snWriteControl{snReadControl: control})
	}
	snWriteNative(t, out)
}
