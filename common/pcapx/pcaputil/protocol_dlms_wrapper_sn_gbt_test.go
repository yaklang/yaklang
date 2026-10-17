package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snGBTIndependentControls(t *testing.T) []snWriteControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-gbt-source", "DLMS_SN_GBT_TEST_ROOT", "controls.json")
	require.Equal(t, "0dd4e538b4354de6520cf20162cd117f2f1479925e21d48a1d4df6b3e9bb7dd4", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct {
		Cases  []map[string]any
		Reject []map[string]any `json:"fresh_rejection_cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 11)
	require.Len(t, doc.Reject, 6)
	cases := append(doc.Cases, doc.Reject...)
	out := make([]snWriteControl, 0, len(cases))
	for _, c := range cases {
		c["metadata"] = c["steps"]
		encoded, err := json.Marshal(c)
		require.NoError(t, err)
		var controls snReadControl
		require.NoError(t, json.Unmarshal(encoded, &controls))
		require.NotEmpty(t, controls.Steps)
		out = append(out, snWriteControl{snReadControl: controls})
	}
	return out
}
func TestDLMSWrapperSNGBTNative(t *testing.T) { snWriteNative(t, snGBTIndependentControls(t)) }

func snGBTCapturedControls(t *testing.T) []snWriteControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-gbt", "DLMS_SN_GBT_TEST_ROOT", "captured-controls.json")
	require.Equal(t, "aef7acd931dddd1f2277df3e06a1d1d7debc76de333bbc427a04abc60a148df2", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 17)
	out := make([]snWriteControl, 0, 17)
	for _, c := range doc.Cases {
		c["metadata"] = c["steps"]
		encoded, err := json.Marshal(c)
		require.NoError(t, err)
		var control snReadControl
		require.NoError(t, json.Unmarshal(encoded, &control))
		require.NotEmpty(t, control.Steps)
		out = append(out, snWriteControl{control, c["sha256"].(string), c["tcp_sha256"].(string), "dlms-sn-gbt-captures"})
	}
	return out
}
func TestDLMSWrapperSNGBTMatrix(t *testing.T) { snWriteMatrix(t, snGBTCapturedControls(t)) }

func snGBTDepthControls(t *testing.T, captured bool) []snWriteControl {
	t.Helper()
	folder, name, pin := "dlms-sn-gbt-source", "depth-boundary-supplement.json", "c1cb64a55e40b4b87b94bb7cf00dc90e7ada27383549dc5adf1751bb96fb2143"
	if captured {
		folder, name, pin = "dlms-sn-gbt", "captured-depth-controls.json", "db075f3982770e545ff8624f49116748798060f24592094f9c863441419fda07"
	}
	raw := finsReadBatch(t, folder, "DLMS_SN_GBT_TEST_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 1)
	c := doc.Cases[0]
	c["metadata"] = c["steps"]
	encoded, err := json.Marshal(c)
	require.NoError(t, err)
	var control snReadControl
	require.NoError(t, json.Unmarshal(encoded, &control))
	require.Len(t, control.Steps, 3)
	out := snWriteControl{snReadControl: control}
	if captured {
		out.UDPHash, out.TCPHash, out.Folder = c["sha256"].(string), c["tcp_sha256"].(string), "dlms-sn-gbt-captures"
	}
	return []snWriteControl{out}
}
func TestDLMSWrapperSNGBTDepthNative(t *testing.T) { snWriteNative(t, snGBTDepthControls(t, false)) }
func TestDLMSWrapperSNGBTDepthMatrix(t *testing.T) { snWriteMatrix(t, snGBTDepthControls(t, true)) }
