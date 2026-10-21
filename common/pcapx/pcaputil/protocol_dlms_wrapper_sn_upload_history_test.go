package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snUploadHistoricalDownloadItem(t *testing.T) (map[string]any, map[string]any) {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-read-upload", "DLMS_SN_UPLOAD_TEST_ROOT", "historical-upload-echo-upgrade.json")
	require.Equal(t, "bc47935e6520eb0d1c7988f5cdebe627bef784bd00468f0fc38a99fe5571e901", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Records []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Records, 1)
	r := d.Records[0]
	require.Equal(t, "validation-dlms-sn-read-service-block.zip", r["original_archive"])
	require.Equal(t, "77deeffa7a3976f329f42e5fc96213bd22c76acdcf3d85c70c16a4fe6597b66b", r["original_archive_sha256"])
	require.Equal(t, "dlms-sn-read-service-block/controls.json", r["original_member"])
	require.Equal(t, "8b81bcf95895c6cce04c8ddbaab958bc3df734507a2e79abde2f806f8f8c2eeb", r["original_member_sha256"])
	require.EqualValues(t, 7, r["original_case_index0"])
	require.EqualValues(t, 2, r["original_step_index0"])
	require.Equal(t, "orphan-block-next-uploadACK-and-normal-SN-neighbor", r["original_case_name"])
	require.Equal(t, "145575a8b2591b465b7a3980f9b2a1b25dff7d4a3fe61d1d25bb874a01beb328", r["original_input_sha256"])
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
			if i != 2 {
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
				require.Equal(t, "orphan-sn-upload-echo", bm["association"])
			} else {
				require.Equal(t, 10, changes)
				require.Equal(t, "decoded", bm["status"])
				require.Equal(t, "message", bm["completeness"])
				require.Nil(t, bm["query_error"])
				require.Equal(t, float64(0), bm["FlowID"])
				require.Nil(t, bm["fields"].(map[string]any)["assembled_request_body"])
			}
			require.Nil(t, bm["error"])
			require.Equal(t, "dlms-wrapper-v1-sn-read-upload", bm["profile"])
		}
	}
	return r, next
}
func snUploadUpgradeHistoricalDownload(t *testing.T, raw []byte, out []snReadControl) []snReadControl {
	t.Helper()
	r, next := snUploadHistoricalDownloadItem(t)
	require.Equal(t, r["original_member_sha256"], fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Len(t, out, 26)
	var old struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &old))
	require.Equal(t, r["original_case"], old.Cases[7])
	before, err := json.Marshal(r["original_case"])
	require.NoError(t, err)
	var original snReadControl
	require.NoError(t, json.Unmarshal(before, &original))
	require.Equal(t, original, out[7])
	updated, err := json.Marshal(next)
	require.NoError(t, err)
	var c snReadControl
	require.NoError(t, json.Unmarshal(updated, &c))
	out[7] = c
	return out
}
func TestDLMSWrapperSNUploadHistoricalBinding(t *testing.T) {
	r, _ := snUploadHistoricalDownloadItem(t)
	raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+r["original_member"].(string))
	require.Equal(t, r["original_member_sha256"], fmt.Sprintf("%x", sha256.Sum256(raw)))
	caps := r["capture"].(map[string]any)
	for _, tr := range []string{"udp", "tcp"} {
		c := caps[tr].(map[string]any)
		p := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+c["member"].(string))
		require.Equal(t, c["sha256"], fmt.Sprintf("%x", sha256.Sum256(p)))
	}
}
