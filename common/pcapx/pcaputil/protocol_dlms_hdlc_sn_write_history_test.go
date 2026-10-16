package pcaputil

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

type hdlcSNWriteHistory struct {
	OriginalArchive string         `json:"original_archive"`
	ArchiveSHA      string         `json:"original_archive_sha256"`
	Member          string         `json:"original_member"`
	MemberSHA       string         `json:"original_member_sha256"`
	CaseIndex       int            `json:"case_index0"`
	Name            string         `json:"original_case_name"`
	StepIndex       int            `json:"step_index0"`
	WireSHA         string         `json:"original_input_sha256"`
	Original        map[string]any `json:"original_step"`
	Step            map[string]any `json:"step"`
	Allowed         []string       `json:"allowed_changed_keys"`
	Captures        map[string]struct {
		Member string
		SHA    string `json:"sha256"`
	}
}

func hdlcSNWriteHistoryItem(t *testing.T) hdlcSNWriteHistory {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-write", "DLMS_HDLC_SN_WRITE_TEST_ROOT", "historical-SNWrite-upgrade.json")
	require.Equal(t, "bcd7aaf6556c0b6e03147db6ab6984e418edf99856c9b92f9e5dd46a91fb2589", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Records []hdlcSNWriteHistory }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Records, 1)
	r := d.Records[0]
	require.Len(t, r.Captures, 2)
	require.Equal(t, "validation-dlms-hdlc-sn-get-close.zip", r.OriginalArchive)
	require.Equal(t, "5924169d9091f4a7671148b4c198f208d62bdb9ced0b739659c33ebdee23dcbb", r.ArchiveSHA)
	require.Equal(t, "dlms-hdlc-sn-read/controls-safe-owner.json", r.Member)
	require.Equal(t, "3bab1436ded0f609435ca0a0c6337f15274a6a04ee4f0c71949b5eb61a2dc704", r.MemberSHA)
	require.Equal(t, 10, r.CaseIndex)
	require.Equal(t, 14, r.StepIndex)
	require.Equal(t, "completeframe-malformed-and-unsupported-still-independent-SN", r.Name)
	require.Equal(t, []string{"completeness", "error", "profile", "query_error", "status"}, r.Allowed)
	require.Equal(t, "5e88326c74ab58ba2a40df6027ac27991b0ceab812a5d1a17f067d02faee653a", r.WireSHA)
	require.Len(t, r.Step, len(r.Original))
	allowed := map[string]any{"profile": "dlms-hdlc-sn-write", "status": "malformed", "completeness": "malformed", "error": "MalformedMessage", "query_error": "MalformedMessage"}
	for k, v := range r.Original {
		_, exists := r.Step[k]
		require.True(t, exists, "every original step key must remain present: %s", k)
		if next, ok := allowed[k]; ok {
			require.NotEqual(t, v, r.Step[k])
			require.Equal(t, next, r.Step[k])
		} else {
			require.Equal(t, v, r.Step[k], k)
		}
	}
	return r
}
func hdlcSNWriteUpgradeHistoricalRead(t *testing.T, raw []byte, out []hdlcSNReadCase) []hdlcSNReadCase {
	t.Helper()
	r := hdlcSNWriteHistoryItem(t)
	require.Equal(t, r.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Len(t, out, 15)
	c := &out[r.CaseIndex]
	require.Equal(t, r.Name, c.Name)
	require.Len(t, c.Steps, 16)
	require.Len(t, c.Metadata, 16)
	require.Equal(t, r.Original, c.Metadata[r.StepIndex])
	require.Equal(t, r.WireSHA, c.Steps[r.StepIndex].SHA)
	require.Empty(t, c.Close.Events)
	require.Equal(t, r.Captures["udp"].SHA, c.UDPHash)
	require.Equal(t, r.Captures["tcp"].SHA, c.TCPHash)
	require.Equal(t, "dlms-hdlc-sn-read/"+r.Name+".udp.pcap", r.Captures["udp"].Member)
	require.Equal(t, "dlms-hdlc-sn-read/"+r.Name+".tcp.pcap", r.Captures["tcp"].Member)
	// Assign exactly the independently specified step. All other typed and metadata
	// rows, carrier bytes, ownership and Close stay in their original objects.
	b, err := json.Marshal(r.Step)
	require.NoError(t, err)
	var next hdlcOpStep
	require.NoError(t, json.Unmarshal(b, &next))
	c.Steps[r.StepIndex] = next
	c.Metadata[r.StepIndex] = r.Step
	return out
}
func TestDLMSHDLCSNWriteHistoricalBinding(t *testing.T) {
	r := hdlcSNWriteHistoryItem(t)
	raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+r.Member)
	require.Equal(t, r.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var old struct {
		Cases []struct {
			Name  string
			Steps []map[string]any
			Close map[string]any
		}
	}
	require.NoError(t, json.Unmarshal(raw, &old))
	var originalTyped struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &originalTyped))
	// Match the existing loader's byte representation normalization only. This
	// reference is decoded directly from the pinned original, without upgrades.
	for i := range originalTyped.Cases {
		for j := range originalTyped.Cases[i].Steps {
			st := &originalTyped.Cases[i].Steps[j]
			for _, f := range []map[string]any{st.Fields, st.Session} {
				if text, ok := f["LLC"].(string); ok {
					b, err := base64.StdEncoding.DecodeString(text)
					require.NoError(t, err)
					require.Len(t, b, 3)
					f["LLC"] = map[string]any{"hexbytes": fmt.Sprintf("%x", b)}
				}
			}
		}
	}
	got := hdlcSNReadControls(t)
	require.Len(t, got, len(old.Cases))
	for i, c := range old.Cases {
		require.Equal(t, c.Name, got[i].Name)
		require.Equal(t, originalTyped.Cases[i].Budget, got[i].Budget)
		require.Equal(t, originalTyped.Cases[i].Close, got[i].Close)
		require.Equal(t, originalTyped.Cases[i].TCPCloseOverride, got[i].TCPCloseOverride)
		require.Len(t, got[i].Steps, len(originalTyped.Cases[i].Steps))
		for j, want := range originalTyped.Cases[i].Steps {
			if i == r.CaseIndex && j == r.StepIndex {
				b, err := json.Marshal(r.Step)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(b, &want))
			}
			require.Equal(t, want, got[i].Steps[j], "only the single selected typed row may change")
		}
		require.Len(t, got[i].Metadata, len(c.Steps))
		for j, s := range c.Steps {
			if i == r.CaseIndex && j == r.StepIndex {
				require.Equal(t, r.Step, got[i].Metadata[j])
			} else {
				require.Equal(t, s, got[i].Metadata[j])
			}
		}
		if i == r.CaseIndex {
			require.Empty(t, got[i].Close.Events)
			require.Equal(t, map[string]any{"events": []any{}, "repeat_events": float64(0), "BufferedBytes_after": float64(0)}, c.Close)
		}
	}
	for _, tr := range []string{"udp", "tcp"} {
		t.Run(tr, func(t *testing.T) {
			c := r.Captures[tr]
			wire := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+c.Member)
			require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
		})
	}
}
