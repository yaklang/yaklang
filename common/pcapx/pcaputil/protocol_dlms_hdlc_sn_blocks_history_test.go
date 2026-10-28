package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcSNBlocksHistoryItem(t *testing.T) hdlcSNWriteHistory {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-read-block", "DLMS_HDLC_SN_BLOCK_TEST_ROOT", "historical-selected-block-diagnostic.json")
	require.Equal(t, "f12671b0a0e0266f8c6390febb0c894d9d6c6d19b7463b9df69e1335492ee77a", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct {
		Archive struct{ File, SHA256 string }
		Records []struct {
			Member    string         `json:"original_member"`
			MemberSHA string         `json:"original_member_sha256"`
			Index     int            `json:"original_case_index0"`
			Name      string         `json:"original_case_name"`
			StepIndex int            `json:"original_step_index0"`
			WireSHA   string         `json:"original_input_sha256"`
			Original  map[string]any `json:"original_step"`
			Next      map[string]any `json:"updated_step"`
			Allowed   []string       `json:"allowed_changed_keys"`
			Captures  []struct{ Member, SHA256 string }
		}
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Records, 1)
	x := d.Records[0]
	r := hdlcSNWriteHistory{OriginalArchive: d.Archive.File, ArchiveSHA: d.Archive.SHA256, Member: x.Member, MemberSHA: x.MemberSHA, CaseIndex: x.Index, Name: x.Name, StepIndex: x.StepIndex, WireSHA: x.WireSHA, Original: x.Original, Step: x.Next, Allowed: x.Allowed}
	require.Equal(t, "validation-dlms-hdlc-sn-get-close.zip", r.OriginalArchive)
	require.Equal(t, "5924169d9091f4a7671148b4c198f208d62bdb9ced0b739659c33ebdee23dcbb", r.ArchiveSHA)
	require.Equal(t, "dlms-hdlc-sn-read/controls-safe-owner.json", r.Member)
	require.Equal(t, "3bab1436ded0f609435ca0a0c6337f15274a6a04ee4f0c71949b5eb61a2dc704", r.MemberSHA)
	require.Equal(t, 10, r.CaseIndex)
	require.Equal(t, 12, r.StepIndex)
	require.Equal(t, "completeframe-malformed-and-unsupported-still-independent-SN", r.Name)
	require.Equal(t, "edcf5381e127cf504085a3f195b9e0bb11cac010cf1b15fb7efbfc38d87cdce5", r.WireSHA)
	require.Equal(t, []string{"profile", "status", "completeness", "error", "query_error"}, r.Allowed)
	require.Len(t, r.Step, len(r.Original))
	allowed := map[string]any{"profile": "dlms-hdlc-sn-read-block", "status": "malformed", "completeness": "malformed", "error": "MalformedMessage", "query_error": "MalformedMessage"}
	for k, v := range r.Original {
		next, present := r.Step[k]
		require.True(t, present, k)
		if want, ok := allowed[k]; ok {
			require.NotEqual(t, v, next, k)
			require.Equal(t, want, next, k)
		} else {
			require.Equal(t, v, next, k)
		}
	}
	require.Len(t, x.Captures, 2)
	for i, tr := range []string{"udp", "tcp"} {
		c := x.Captures[i]
		require.Equal(t, "dlms-hdlc-sn-read/"+r.Name+"."+tr+".pcap", c.Member)
		p := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+c.Member)
		require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(p)))
	}
	return r
}
func hdlcSNBlocksUpgradeHistoricalRead(t *testing.T, raw []byte, out []hdlcSNReadCase) []hdlcSNReadCase {
	t.Helper()
	r := hdlcSNBlocksHistoryItem(t)
	require.Equal(t, r.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Len(t, out, 15)
	c := &out[r.CaseIndex]
	require.Equal(t, r.Name, c.Name)
	require.Len(t, c.Steps, 16)
	require.Equal(t, r.Original, c.Metadata[r.StepIndex])
	require.Equal(t, r.WireSHA, c.Steps[r.StepIndex].SHA)
	require.Empty(t, c.Close.Events)
	b, err := json.Marshal(r.Step)
	require.NoError(t, err)
	var next hdlcOpStep
	require.NoError(t, json.Unmarshal(b, &next))
	c.Steps[r.StepIndex] = next
	c.Metadata[r.StepIndex] = r.Step
	return out
}
func TestDLMSHDLCSNBlocksHistoricalBinding(t *testing.T) {
	r := hdlcSNBlocksHistoryItem(t)
	raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+r.Member)
	got := hdlcSNReadControls(t)
	require.Equal(t, r.Step, got[r.CaseIndex].Metadata[r.StepIndex])
	require.Equal(t, r.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
}
