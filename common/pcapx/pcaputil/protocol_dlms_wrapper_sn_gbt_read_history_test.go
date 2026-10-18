package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snGBTReadHistoricalRecords(t *testing.T) []map[string]any {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-gbt-legacy", "DLMS_SN_GBT_TEST_ROOT", "historical-gbt-sn-read-upgrades.json")
	require.Equal(t, "2d2a2a1d7da7b5424a2eb6b34d2367f772cf62e05b2b9806f25eaf3233a67c3d", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Records []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Records, 2)
	for ci, r := range doc.Records {
		require.Equal(t, "validation-dlms-wrapper-sn-read.zip", r["original_archive"])
		require.Equal(t, "bb588f49ed993af49d8d97c13370ee051c511fc5f81b09b127dbc9fdd6848873", r["original_archive_sha256"])
		require.Equal(t, "dlms-wrapper-sn-read/gbt-supplement.json", r["original_member"])
		require.Equal(t, "bc77182c122011ab4d5aab9c06f962309bf5806a28104590d92733e44db9fbea", r["original_member_sha256"])
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
			for _, i := range []int{0, 1} {
				require.Equal(t, a[i], b[i])
			}
		}
		// Reapply exactly the independently listed28 leaf changes to a fresh copy.
		// Only two exact output steps may change; all inputs/carrier/budget/Close remain literal.
		copyRaw, err := json.Marshal(before)
		require.NoError(t, err)
		var reconstructed map[string]any
		require.NoError(t, json.Unmarshal(copyRaw, &reconstructed))
		diffs := r["exact_changed_leaves"].([]any)
		require.Len(t, diffs, 28)
		seen := map[string]bool{}
		for _, entry := range diffs {
			diff := entry.(map[string]any)
			path := diff["path"].([]any)
			require.GreaterOrEqual(t, len(path), 3)
			top := path[0].(string)
			require.Contains(t, []string{"answers", "metadata", "metadata_steps"}, top)
			index := int(path[1].(float64))
			require.Contains(t, []int{2, 3}, index)
			key := path[2].(string)
			allowed := []string{"fields", "session", "Session", "GetFields", "Decode", "error", "association", "outstanding", "status", "completeness", "query_error", "Transaction_index", "Response_index", "transaction_index", "response_index", "transaction_owner_step", "response_to_step"}
			require.Contains(t, allowed, key)
			if index == 3 && len(path) > 3 {
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
func snGBTUpgradeHistoricalRead(t *testing.T, raw []byte, out []snReadControl) []snReadControl {
	t.Helper()
	records := snGBTReadHistoricalRecords(t)
	require.Len(t, out, 2)
	require.Equal(t, "bc77182c122011ab4d5aab9c06f962309bf5806a28104590d92733e44db9fbea", fmt.Sprintf("%x", sha256.Sum256(raw)))
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
		for _, si := range []int{2, 3} {
			out[ci].Steps[si] = next.Steps[si]
		}
	}
	return out
}
func snGBTCheckReadHistoricalBinding(t *testing.T) {
	for _, r := range snGBTReadHistoricalRecords(t) {
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

func snGBTLegacyBoundaryCase(t *testing.T, command byte) snReadControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-gbt-legacy", "DLMS_SN_GBT_TEST_ROOT", "inline-gbt-sn-boundary-upgrades.json")
	require.Equal(t, "859ca7f04f6b625637eaed20a65a7a1932ea54d215578bf735f7b9c62f1a7e0d", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct {
		Records []struct {
			Command byte
			Case    snReadControl
			SHA     []string `json:"input_sha256"`
		}
		Cases []snReadControl
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 4)
	require.Len(t, d.Records, 4)
	for i, r := range d.Records {
		require.Equal(t, r.Case, d.Cases[i])
		require.Len(t, r.SHA, 3)
		require.Len(t, r.Case.Steps, 3)
		for j, st := range r.Case.Steps {
			require.Equal(t, r.SHA[j], st["input_sha256"])
		}
		if r.Command == command {
			return r.Case
		}
	}
	t.Fatal("Missing fixed command", command)
	return snReadControl{}
}
func snGBTLegacyBoundaryEvent(t *testing.T, e *ProtocolEvent, given map[string]any, tr string, deferred bool, flow uint64, ids []uint64, wire []byte) {
	t.Helper()
	want := snReadStep(t, given, tr)
	require.Equal(t, tr, e.Transport)
	require.Equal(t, CaptureDomain{}, e.Domain)
	require.Equal(t, want["input_sha256"], fmt.Sprintf("%x", sha256.Sum256(wire)))
	require.NotZero(t, e.ID)
	require.Equal(t, ids[len(ids)-1], e.ID)
	for _, id := range ids[:len(ids)-1] {
		require.NotEqual(t, id, e.ID)
	}
	snReadCheckEvent(t, e, want, ids, deferred, flow)
	snWriteQueries(t, e, want, ids)
	wrapperClosePoison(e)
	snReadCheckSaved(t, e, want, ids)
}
