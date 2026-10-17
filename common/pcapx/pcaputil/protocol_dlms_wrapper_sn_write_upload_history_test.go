package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snWriteUploadHistoricalItem(t *testing.T) (map[string]any, map[string]any) {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-write-upload", "DLMS_SN_WRITE_UPLOAD_TEST_ROOT", "historical-write-upload-echo-upgrade.json")
	require.Equal(t, "8664d67004cf52460a1fbdf0ea65f3a0d5d6bdf5ca704d079bb02caaba0936b4", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Records []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Records, 1)
	r := d.Records[0]
	require.Equal(t, "validation-dlms-wrapper-sn-write.zip", r["original_archive"])
	require.Equal(t, "4d786947a10dda6be15dad3fbadd73440ee8bbdbefc65c83f97b2c212766c7f9", r["original_archive_sha256"])
	require.Equal(t, "dlms-wrapper-sn-write/controls.json", r["original_member"])
	require.Equal(t, "2ae6c6da52d6c30fd1a9c4fb5e86935d89794f16e53a665aacd85a00a0b024c3", r["original_member_sha256"])
	require.EqualValues(t, 12, r["original_case_index0"])
	require.EqualValues(t, 6, r["original_step_index0"])
	require.Equal(t, "unsupported-SDK04-unequal-unselected-and-hard17-Resource", r["original_case_name"])
	require.Equal(t, "1565560fe772dd398d37112787cdb0d6b903725a6e2e118fa67304cf7a16e044", r["original_input_sha256"])
	old, next := r["original_case"].(map[string]any), r["case"].(map[string]any)
	require.Len(t, next, len(old))
	semantic := map[string]bool{"profile": true, "status": true, "completeness": true, "fields": true, "GetFields": true, "session": true, "Session": true, "Decode": true, "error": true, "query_error": true}
	for k, v := range old {
		nv, present := next[k]
		require.True(t, present, k)
		if k != "metadata" && k != "metadata_steps" && k != "answers" {
			require.Equal(t, v, nv, k)
			continue
		}
		a, b := v.([]any), nv.([]any)
		require.Len(t, b, len(a))
		for i, av := range a {
			if i != 6 {
				require.Equal(t, av, b[i])
				continue
			}
			am, bm := av.(map[string]any), b[i].(map[string]any)
			require.Len(t, bm, len(am))
			changes := 0
			for key, x := range am {
				y, present := bm[key]
				require.True(t, present, key)
				allowed := semantic[key]
				if k == "answers" {
					allowed = key == "profile" || key == "fields" || key == "error" || key == "association"
				}
				if allowed {
					require.NotEqual(t, x, y, key)
					changes++
				} else {
					require.Equal(t, x, y, key)
				}
			}
			if k == "answers" {
				require.Equal(t, 4, changes)
				require.Equal(t, "unverified-sn-block-after-conversation-invalidation", bm["association"])
			} else {
				require.Equal(t, 10, changes)
				require.Equal(t, "decoded", bm["status"])
				require.Equal(t, "message", bm["completeness"])
				require.Nil(t, bm["query_error"])
				require.Equal(t, am["FlowID"], bm["FlowID"])
				require.Nil(t, bm["fields"].(map[string]any)["assembled_request_body"])
			}
			require.Nil(t, bm["error"])
			require.Equal(t, "dlms-wrapper-v1-sn-write-upload", bm["profile"])
		}
	}
	return r, next
}

// Apply only the new target step, preserving existing parameterized capability
// adjustments and every other assertion in the legacy loader.
func snWriteUploadUpgradeHistoricalWrite(t *testing.T, raw []byte, out []snWriteControl) []snWriteControl {
	t.Helper()
	r, next := snWriteUploadHistoricalItem(t)
	require.Equal(t, r["original_member_sha256"], fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Len(t, out, 15)
	var old struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &old))
	require.Equal(t, r["original_case"], old.Cases[12])
	before, err := json.Marshal(r["original_case"])
	require.NoError(t, err)
	var original snReadControl
	require.NoError(t, json.Unmarshal(before, &original))
	require.Equal(t, original.Steps[6], out[12].Steps[6])
	updated, err := json.Marshal(next)
	require.NoError(t, err)
	var c snReadControl
	require.NoError(t, json.Unmarshal(updated, &c))
	require.Len(t, c.Steps, len(original.Steps))
	out[12].Steps[6] = c.Steps[6]
	return out
}
func TestDLMSWrapperSNWriteUploadHistoricalBinding(t *testing.T) {
	r, _ := snWriteUploadHistoricalItem(t)
	raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+r["original_member"].(string))
	require.Equal(t, r["original_member_sha256"], fmt.Sprintf("%x", sha256.Sum256(raw)))
	caps := r["capture"].(map[string]any)
	for _, tr := range []string{"udp", "tcp"} {
		c := caps[tr].(map[string]any)
		p := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+c["member"].(string))
		require.Equal(t, c["sha256"], fmt.Sprintf("%x", sha256.Sum256(p)))
	}
}
