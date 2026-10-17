package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snGBTHistoricalRecords(t *testing.T) []map[string]any {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-gbt", "DLMS_SN_GBT_TEST_ROOT", "historical-gbt-sn-write-upgrades.json")
	require.Equal(t, "a4c30eded8ba319ae34b62f6f0103e32dbde0ef9bd7593a0bbfa69d27956c931", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Records []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Records, 2)
	for ci, r := range doc.Records {
		require.Equal(t, "validation-dlms-wrapper-sn-write.zip", r["original_archive"])
		require.Equal(t, "4d786947a10dda6be15dad3fbadd73440ee8bbdbefc65c83f97b2c212766c7f9", r["original_archive_sha256"])
		require.Equal(t, "dlms-wrapper-sn-write/gbt-supplement.json", r["original_member"])
		require.Equal(t, "fd4308c0004ab0f653174b5b5f7ebc09db879ca354ea663273820f1939a03b94", r["original_member_sha256"])
		require.EqualValues(t, ci, r["original_case_index0"])
		before, next := r["original_case"].(map[string]any), r["case"].(map[string]any)
		require.Equal(t, before["name"], r["original_case_name"])
		require.Equal(t, before["name"], next["name"])
		require.Len(t, next, len(before))
		for k, v := range before {
			if k != "answers" && k != "metadata" && k != "metadata_steps" {
				require.Equal(t, v, next[k], k)
				continue
			}
			a, b := v.([]any), next[k].([]any)
			require.Len(t, a, 4)
			require.Len(t, b, 4)
			for _, i := range []int{0, 3} {
				require.Equal(t, a[i], b[i])
			}
		}
		// Reapply exactly the independently listed39 leaf changes to a fresh copy.
		// Only two output steps may change; all inputs/carrier/budget/Close remain literal.
		copyRaw, err := json.Marshal(before)
		require.NoError(t, err)
		var reconstructed map[string]any
		require.NoError(t, json.Unmarshal(copyRaw, &reconstructed))
		diffs := r["exact_changed_leaves"].([]any)
		require.Len(t, diffs, 39)
		seen := map[string]bool{}
		for _, entry := range diffs {
			diff := entry.(map[string]any)
			path := diff["path"].([]any)
			require.GreaterOrEqual(t, len(path), 3)
			top := path[0].(string)
			require.Contains(t, []string{"answers", "metadata", "metadata_steps"}, top)
			index := int(path[1].(float64))
			require.Contains(t, []int{1, 2}, index)
			key := path[2].(string)
			allowed := []string{"fields", "session", "Session", "GetFields", "Decode", "error", "association", "outstanding", "status", "completeness", "query_error", "Transaction_index", "Response_index", "transaction_index", "response_index", "transaction_owner_step", "response_to_step"}
			require.Contains(t, allowed, key)
			if index == 2 && len(path) > 3 {
				last := path[len(path)-1].(string)
				require.Contains(t, []string{"Association", "Outstanding"}, last)
			}
			pathRaw, err := json.Marshal(path)
			require.NoError(t, err)
			require.False(t, seen[string(pathRaw)])
			seen[string(pathRaw)] = true
			var node any = reconstructed
			for _, segment := range path[:len(path)-1] {
				switch k := segment.(type) {
				case string:
					node = node.(map[string]any)[k]
				case float64:
					node = node.([]any)[int(k)]
				default:
					t.Fatalf("invalid path segment%T", segment)
				}
			}
			final := path[len(path)-1].(string)
			leaf := node.(map[string]any)
			require.Equal(t, diff["old"], leaf[final])
			leaf[final] = diff["new"]
		}
		require.Equal(t, next, reconstructed)
		require.Len(t, r["steps"], 2)
	}
	return doc.Records
}
func snGBTUpgradeHistoricalWrite(t *testing.T, raw []byte, out []snWriteControl) []snWriteControl {
	t.Helper()
	records := snGBTHistoricalRecords(t)
	require.Len(t, out, 2)
	require.Equal(t, "fd4308c0004ab0f653174b5b5f7ebc09db879ca354ea663273820f1939a03b94", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var original struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &original))
	require.Len(t, original.Cases, 2)
	for ci, r := range records {
		require.Equal(t, r["original_case"], original.Cases[ci])
		b, err := json.Marshal(r["original_case"])
		require.NoError(t, err)
		var before snReadControl
		require.NoError(t, json.Unmarshal(b, &before))
		b, err = json.Marshal(r["case"])
		require.NoError(t, err)
		var next snReadControl
		require.NoError(t, json.Unmarshal(b, &next))
		require.Equal(t, before.Name, next.Name)
		require.Equal(t, before.Close, next.Close)
		require.Equal(t, before.TCPClose, next.TCPClose)
		require.Equal(t, before.Budget, next.Budget)
		require.Equal(t, before.Overrides, next.Overrides)
		require.Equal(t, before.Steps, out[ci].Steps)
		for _, si := range []int{1, 2} {
			out[ci].Steps[si] = next.Steps[si]
		}
	}
	return out
}
func TestDLMSWrapperSNGBTHistoricalBinding(t *testing.T) {
	for _, r := range snGBTHistoricalRecords(t) {
		raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+r["original_member"].(string))
		require.Equal(t, r["original_member_sha256"], fmt.Sprintf("%x", sha256.Sum256(raw)))
		for _, cap := range r["capture"].(map[string]any) {
			c := cap.(map[string]any)
			raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+c["member"].(string))
			require.Equal(t, c["sha256"], fmt.Sprintf("%x", sha256.Sum256(raw)))
			require.EqualValues(t, c["bytes"], len(raw))
		}
	}
}
