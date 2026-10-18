package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcSNUploadHistoricalItem(t *testing.T) map[string]any {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-upload", "DLMS_HDLC_SN_UPLOAD_TEST_ROOT", "historical-upload-echo-upgrades.json")
	require.Equal(t, "9d788a23acfd58e0869ae07e1acef471abf65a3657e26e934358aa55182352f3", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Records []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Records, 1)
	r := doc.Records[0]
	require.Equal(t, "validation-dlms-hdlc-sn-write.zip", r["original_archive"])
	require.Equal(t, "ab50b337f6917b47628fa43cf39edf52d7704f8ad2273f6d82c93c4de0b94a54", r["original_archive_sha256"])
	require.Equal(t, "dlms-hdlc-sn-write/controls.json", r["original_member"])
	require.Equal(t, "2089396fca41e028327b03a0f762552ef8fb9f313d1d47ca81869dd7d806ed4c", r["original_member_sha256"])
	require.EqualValues(t, 15, r["original_case_index0"])
	require.EqualValues(t, 22, r["original_step_index0"])
	require.Equal(t, "known-carrier-bare-malformed-local-unsupported-hard17-release", r["original_case_name"])
	require.Equal(t, "fb9f1ac3650b55fd7727e9b00d2f14469cd1cf6c8d41c91018062083136feab8", r["original_case_sha256_canonical"])
	require.Equal(t, "76323fa25cbde3101a356928faa4456ff609848854bbf13e7b218d1551a950e4", r["original_step_sha256_canonical"])
	require.Equal(t, "87c96112f0ee818ed55dbf5dd78536583ab3a46fe396bbfa124c943fdcfea3a9", r["original_input_sha256"])
	old, next := r["original_case"].(map[string]any), r["case"].(map[string]any)
	require.Len(t, next, len(old))
	for k, v := range old {
		nv, present := next[k]
		require.True(t, present, k)
		if k != "steps" {
			require.Equal(t, v, nv, k)
			continue
		}
		a, b := v.([]any), nv.([]any)
		require.Len(t, a, 31)
		require.Len(t, b, len(a))
		for i, av := range a {
			if i != 22 {
				require.Equal(t, av, b[i])
				continue
			}
			require.Equal(t, r["original_step"], av)
			require.Equal(t, r["step"], b[i])
			am, bm := av.(map[string]any), b[i].(map[string]any)
			require.Len(t, bm, len(am))
			allowed := map[string]bool{"profile": true, "status": true, "completeness": true, "error": true, "fields": true, "session": true, "Session": true, "GetFields": true, "Decode": true, "query_error": true}
			changes := 0
			for key, x := range am {
				y, present := bm[key]
				require.True(t, present, key)
				if allowed[key] {
					require.NotEqual(t, x, y, key)
					changes++
				} else {
					require.Equal(t, x, y, key)
				}
			}
			require.Equal(t, 10, changes)
			require.Nil(t, bm["error"])
			require.Nil(t, bm["query_error"])
			require.Equal(t, "dlms-hdlc-sn-write-upload", bm["profile"])
			require.Equal(t, "decoded", bm["status"])
			require.Equal(t, "message", bm["completeness"])
			fields := bm["fields"].(map[string]any)
			require.Equal(t, "unverified-sn-block-after-conversation-invalidation", fields["Association"])
			sn := fields["SN Write"].(map[string]any)
			for _, key := range []string{"transfer_complete", "assembled_request_body", "transfer_start_event", "acknowledged_block_event"} {
				require.NotContains(t, sn, key)
			}
			require.EqualValues(t, 0, bm["TransactionID"])
			require.EqualValues(t, 0, bm["Response_index"])
		}
	}
	return r
}

// Replace only the exactly bound step, keeping every existing capability
// supplement and all unrelated typed/raw/capture/Close/resource assertions.
func hdlcSNUploadUpgradeHistoricalWrite(t *testing.T, raw []byte, out []hdlcSNReadCase) []hdlcSNReadCase {
	t.Helper()
	r := hdlcSNUploadHistoricalItem(t)
	require.Equal(t, r["original_member_sha256"], fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Len(t, out, 16)
	var doc struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Equal(t, r["original_case"], doc.Cases[15])
	c := &out[15]
	require.Equal(t, r["original_case_name"], c.Name)
	require.Len(t, c.Steps, 31)
	require.Len(t, c.Metadata, 31)
	originalBytes, err := json.Marshal(r["original_step"])
	require.NoError(t, err)
	var original hdlcOpStep
	require.NoError(t, json.Unmarshal(originalBytes, &original))
	require.Equal(t, original, c.Steps[22])
	require.Equal(t, r["original_step"], c.Metadata[22])
	wire, err := hex.DecodeString(original.Wire)
	require.NoError(t, err)
	require.Equal(t, r["original_input_sha256"], fmt.Sprintf("%x", sha256.Sum256(wire)))
	nextBytes, err := json.Marshal(r["step"])
	require.NoError(t, err)
	var step hdlcOpStep
	require.NoError(t, json.Unmarshal(nextBytes, &step))
	c.Steps[22] = step
	c.Metadata[22] = r["step"].(map[string]any)
	return out
}
func TestDLMSHDLCSNUploadHistoricalBinding(t *testing.T) {
	r := hdlcSNUploadHistoricalItem(t)
	raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+r["original_member"].(string))
	require.Equal(t, r["original_member_sha256"], fmt.Sprintf("%x", sha256.Sum256(raw)))
	caps := r["capture"].(map[string]any)
	for _, tr := range []string{"tcp", "udp"} {
		c := caps[tr].(map[string]any)
		p := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+c["member"].(string))
		require.Equal(t, c["sha256"], fmt.Sprintf("%x", sha256.Sum256(p)))
	}
}
