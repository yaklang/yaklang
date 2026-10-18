package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

type gbtActionListHistoricalRecord struct {
	Member    string `json:"original_member"`
	MemberSHA string `json:"original_member_sha256"`
	Index     int    `json:"original_case_index0"`
	Name      string `json:"original_case_name"`
	InputSHA  string `json:"original_input_sha256"`
	Capture   struct {
		Member string
		SHA    string `json:"sha256"`
	}
	Case json.RawMessage
}

func gbtActionListHistoricalRecords(t *testing.T) []gbtActionListHistoricalRecord {
	t.Helper()
	raw := finsReadBatch(t, "dlms-wrapper-gbt-action-list-historical", "DLMS_WRAPPER_GBT_ACTION_LIST_HISTORY_TEST_ROOT", "upgrades.json")
	require.Equal(t, "695e0e4926e01945b6091a1611bcab07ae5c7fb2ad86744d15b0ffc619553643", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct {
		Records []gbtActionListHistoricalRecord
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Records, 2)
	return doc.Records
}

// Only two exact immutable missing-capability witnesses gain full service answers.
// Every input, time, direction, Raw, length, budget and unknown key stays exact.
func gbtActionListHistoricalUpgrade(t *testing.T, original []byte, cases []actionCase, member string) []actionCase {
	t.Helper()
	var old struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(original, &old))
	require.Len(t, old.Cases, len(cases))
	found := 0
	for _, r := range gbtActionListHistoricalRecords(t) {
		if r.Member != member {
			continue
		}
		found++
		require.Equal(t, r.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(original)))
		require.GreaterOrEqual(t, r.Index, 0)
		require.Less(t, r.Index, len(cases))
		c := cases[r.Index]
		require.Equal(t, r.Name, c.Name)
		require.Len(t, c.Steps, 1)
		require.Equal(t, "UnsupportedFeature", c.Steps[0].Error)
		require.Equal(t, r.InputSHA, c.Steps[0].SHA)
		require.Equal(t, r.InputSHA, fmt.Sprintf("%x", sha256.Sum256(wrapperWire(t, c.Steps[0].Wire))))
		var next map[string]any
		require.NoError(t, json.Unmarshal(r.Case, &next))
		before := old.Cases[r.Index]
		bs := before["steps"].([]any)
		ns := next["steps"].([]any)
		require.Len(t, bs, 1)
		require.Len(t, ns, 1)
		b, n := bs[0].(map[string]any), ns[0].(map[string]any)
		for _, key := range []string{"profile", "status", "completeness", "error", "fields", "session", "Transaction_index", "FlowID", "query_error", "GetFields", "Decode"} {
			delete(b, key)
			delete(n, key)
		}
		require.Equal(t, b, n, "all input/context/Raw/length/response/unknown fields stay unchanged")
		delete(before, "Close")
		delete(next, "Close")
		require.Equal(t, before, next, "only exact observed outcomes and Close may change")
		var replacement actionCase
		require.NoError(t, json.Unmarshal(r.Case, &replacement))
		require.Empty(t, replacement.Steps[0].Error)
		require.Equal(t, "dlms-wrapper-v1-gbt", replacement.Steps[0].Profile)
		if strings.Contains(r.Name, "request") {
			require.Equal(t, 1, replacement.Steps[0].Transaction)
			require.Len(t, replacement.Close.Events, 1)
		} else {
			require.Zero(t, replacement.Steps[0].Transaction)
			require.Zero(t, replacement.Steps[0].Response)
			require.Empty(t, replacement.Close.Events)
		}
		cases[r.Index] = replacement
	}
	require.Equal(t, 2, found)
	return cases
}
func TestDLMSWrapperGBTActionListHistoricalBinding(t *testing.T) {
	for _, r := range gbtActionListHistoricalRecords(t) {
		t.Run(r.Name, func(t *testing.T) {
			path := strings.SplitN(r.Member, "/", 2)
			require.Len(t, path, 2)
			original := finsReadBatch(t, path[0], "", path[1])
			require.Equal(t, r.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(original)))
			var old struct{ Cases []actionCase }
			require.NoError(t, json.Unmarshal(original, &old))
			require.Len(t, gbtActionListHistoricalUpgrade(t, original, old.Cases, r.Member), len(old.Cases))
			path = strings.SplitN(r.Capture.Member, "/", 2)
			require.Len(t, path, 2)
			capture := finsReadBatch(t, path[0], "", path[1])
			require.Equal(t, r.Capture.SHA, fmt.Sprintf("%x", sha256.Sum256(capture)))
		})
	}
}

func gbtActionListHistoricalCarrierCheck(t *testing.T, c actionCase, es []*ProtocolEvent, tr string, captured, closing bool, flow uint64) uint64 {
	t.Helper()
	var doc struct {
		Records []struct {
			Name     string `json:"original_case_name"`
			InputSHA string `json:"original_input_sha256"`
			Carriers map[string]struct {
				Steps []map[string]any
				Close struct{ Events []map[string]any }
			} `json:"carrier_metadata"`
		}
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-wrapper-gbt-action-list-historical", "DLMS_WRAPPER_GBT_ACTION_LIST_HISTORY_TEST_ROOT", "upgrades.json"), &doc))
	for _, r := range doc.Records {
		if c.Name != r.Name {
			continue
		}
		require.Len(t, c.Steps, 1)
		require.Equal(t, r.InputSHA, c.Steps[0].SHA)
		key := "native_" + tr
		if captured {
			key = "captured_" + tr
		}
		meta, ok := r.Carriers[key]
		require.True(t, ok)
		wants := meta.Steps
		if closing {
			wants = meta.Close.Events
		}
		require.Len(t, es, len(wants))
		for i, w := range wants {
			e := es[i]
			require.Equal(t, "dlms-wrapper", e.Protocol)
			require.Equal(t, CaptureDomain{}, e.Domain)
			require.Equal(t, tr, e.Transport)
			require.Equal(t, int(w["direction"].(float64)), e.Direction)
			require.Equal(t, time.Unix(0, int64(w["timestamp_unix_ns"].(float64))), e.Timestamp)
			require.Equal(t, uint64(w["offset"].(float64)), e.Offset)
			require.Equal(t, w["source"], e.Source)
			require.Equal(t, w["destination"], e.Destination)
			require.Equal(t, int(w["Length"].(float64)), e.Length)
			if w["FlowID"] == float64(0) {
				require.Zero(t, e.FlowID)
			} else {
				require.NotZero(t, e.FlowID)
				if closing {
					require.Equal(t, flow, e.FlowID)
				}
			}
			if !closing {
				flow = e.FlowID
				f, err := e.GetFields()
				require.NoError(t, err)
				gbtActionListTypes(t, f)
				gbtActionListTypes(t, e.Session)
			} else {
				gbtActionListTypes(t, e.Session)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
				require.Nil(t, e.Raw)
			}
		}
		return flow
	}
	return flow
}

// Rebuild only the two hash-bound answers, including their independent FlowID
// projections, so the existing list checker sees no stale unsupported metadata.
func gbtActionListHistoricalDocument(t *testing.T, original []byte) []byte {
	t.Helper()
	var typed struct{ Cases []actionCase }
	require.NoError(t, json.Unmarshal(original, &typed))
	gbtActionListHistoricalUpgrade(t, original, typed.Cases, "dlms-action-list/controls.json")
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(original, &doc))
	var rows []json.RawMessage
	require.NoError(t, json.Unmarshal(doc["cases"], &rows))
	for _, r := range gbtActionListHistoricalRecords(t) {
		require.Equal(t, "dlms-action-list/controls.json", r.Member)
		rows[r.Index] = append(json.RawMessage(nil), r.Case...)
	}
	var err error
	doc["cases"], err = json.Marshal(rows)
	require.NoError(t, err)
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	return out
}
