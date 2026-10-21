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
func snServiceGBTControls(t *testing.T, name, pin string, count int, captured bool) []snWriteControl {
	t.Helper()
	folder := "dlms-sn-service-gbt-source"
	if captured {
		folder = "dlms-sn-service-gbt"
	}
	raw := finsReadBatch(t, folder, "DLMS_SN_SERVICE_GBT_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, count)
	out := make([]snWriteControl, 0, count)
	for _, c := range doc.Cases {
		c["metadata"] = c["steps"]
		encoded, err := json.Marshal(c)
		require.NoError(t, err)
		var control snReadControl
		require.NoError(t, json.Unmarshal(encoded, &control))
		require.NotEmpty(t, control.Steps)
		v := snWriteControl{snReadControl: control}
		if captured {
			v.UDPHash, v.TCPHash, v.Folder = c["sha256"].(string), c["tcp_sha256"].(string), "dlms-sn-service-gbt-captures"
		}
		out = append(out, v)
	}
	return out
}
func TestDLMSWrapperSNServiceGBTNative(t *testing.T) {
	snWriteNative(t, snServiceGBTControls(t, "controls.json", "550c56f1b21cdbae0cc5be9825c78219737926f75afe958013881b04704a7e26", 12, false))
}
func TestDLMSWrapperSNServiceGBTMatrix(t *testing.T) {
	snWriteMatrix(t, snServiceGBTControls(t, "captured-controls.json", "721d5bbaed130968cc6e012d059c060938b575215226468f0406317452c676b1", 16, true))
}

func TestDLMSWrapperSNServiceGBTDepthTrailingNative(t *testing.T) {
	snWriteNative(t, snServiceGBTControls(t, "depth-trailing-supplement.json", "64232e18320505fdf785ac81a77a96420a0b9f06a88b9e1b5d8891d50609e3b5", 4, false))
}

// Bind complete immutable historical inputs to an independently rebuilt current
// capability answer. All unrelated cases, fields and capture hashes survive.
func snServiceGBTUpgradeHistorical(t *testing.T, raw []byte, out []snWriteControl) []snWriteControl {
	t.Helper()
	pin := fmt.Sprintf("%x", sha256.Sum256(raw))
	captured := pin == "aef7acd931dddd1f2277df3e06a1d1d7debc76de333bbc427a04abc60a148df2"
	require.True(t, captured || pin == "0dd4e538b4354de6520cf20162cd117f2f1479925e21d48a1d4df6b3e9bb7dd4")
	var old struct {
		Cases []map[string]any
		Fresh []map[string]any `json:"fresh_rejection_cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &old))
	cases := append(old.Cases, old.Fresh...)
	require.Len(t, cases, 17)
	require.Len(t, out, 17)
	delta := finsReadBatch(t, "dlms-sn-service-gbt", "DLMS_SN_SERVICE_GBT_ROOT", "historical-SN-R2-Q5-in-GBT-upgrades.json")
	require.Equal(t, "33412aa1e743018ab841fb5b604f47e44c09ff036a74a4261bac76f05fe4ab54", fmt.Sprintf("%x", sha256.Sum256(delta)))
	var doc struct{ Records []map[string]any }
	require.NoError(t, json.Unmarshal(delta, &doc))
	require.Len(t, doc.Records, 2)
	for ri, r := range doc.Records {
		at := []int{6, 12}[ri]
		require.EqualValues(t, at, r["captured_case_index0"])
		which := "source"
		if captured {
			which = "captured"
		}
		before, next := r["original_"+which+"_case"].(map[string]any), r["updated_"+which+"_case"].(map[string]any)
		require.Equal(t, cases[at], before)
		require.Equal(t, before["name"], r["case_name"])
		require.Equal(t, before["name"], next["name"])
		for _, x := range []struct {
			value map[string]any
			key   string
		}{{before, "original_" + which + "_case_canonical_sha256"}, {next, "updated_" + which + "_case_canonical_sha256"}} {
			b, err := json.Marshal(x.value)
			require.NoError(t, err)
			require.Equal(t, r[x.key], fmt.Sprintf("%x", sha256.Sum256(b)))
		}
		require.Len(t, next, len(before))
		for k, v := range before {
			if k != "steps" {
				require.Equal(t, v, next[k], k)
			}
		}
		bs, ns := before["steps"].([]any), next["steps"].([]any)
		require.Len(t, ns, len(bs))
		for si, b := range bs {
			a := b.(map[string]any)
			z := ns[si].(map[string]any)
			require.Len(t, z, len(a))
			for k, v := range a {
				allowed := false
				for _, q := range []string{"status", "completeness", "error", "fields", "session", "query_error", "GetFields", "Decode", "Transaction_index", "Response_index"} {
					allowed = allowed || k == q
				}
				if ri == 1 {
					allowed = allowed || k == "FlowID" || k == "native_udp_flow_nonzero" || k == "captured_udp_flow_nonzero"
				}
				if !allowed || ri == 0 && si == 0 {
					require.Equal(t, v, z[k], fmt.Sprintf("case%d step%d %s", ri, si, k))
				}
			}
		}
		convert := func(c map[string]any) snReadControl {
			copy := make(map[string]any, len(c)+1)
			for k, v := range c {
				copy[k] = v
			}
			copy["metadata"] = c["steps"]
			b, err := json.Marshal(copy)
			require.NoError(t, err)
			var v snReadControl
			require.NoError(t, json.Unmarshal(b, &v))
			return v
		}
		original, updated := convert(before), convert(next)
		require.Equal(t, original, out[at].snReadControl)
		require.Equal(t, original.Name, updated.Name)
		require.Equal(t, original.Budget, updated.Budget)
		require.Equal(t, original.Close, updated.Close)
		require.Equal(t, original.TCPClose, updated.TCPClose)
		require.Equal(t, original.Overrides, updated.Overrides)
		out[at].snReadControl = updated
	}
	return out
}
