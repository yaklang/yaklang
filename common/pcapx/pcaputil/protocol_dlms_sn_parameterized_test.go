package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func snParameterizedBaseline(t *testing.T) ([]snWriteControl, []hdlcSNReadCase) {
	t.Helper()
	return snParameterizedCases(t, "baseline-controls.json", "30095e2e2b6596d301716ab2501fd7f53332542c8971d674f98fcf2830922d64", 3, 3)
}

func snParameterizedCases(t *testing.T, name, pin string, wrapperCount, hdlcCount int) ([]snWriteControl, []hdlcSNReadCase) {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-parameterized-access", "DLMS_SN_PARAMETERIZED_TEST_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct {
		Wrapper []snReadControl `json:"wrapper_cases"`
		HDLC    []hdlcOpCase    `json:"hdlc_cases"`
	}
	var meta struct {
		Wrapper []struct {
			UDP string `json:"sha256"`
			TCP string `json:"tcp_sha256"`
		} `json:"wrapper_cases"`
		HDLC []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		} `json:"hdlc_cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Len(t, d.Wrapper, wrapperCount)
	require.Len(t, d.HDLC, hdlcCount)
	require.Len(t, meta.Wrapper, wrapperCount)
	require.Len(t, meta.HDLC, hdlcCount)
	w := make([]snWriteControl, len(d.Wrapper))
	for i, c := range d.Wrapper {
		w[i] = snWriteControl{c, meta.Wrapper[i].UDP, meta.Wrapper[i].TCP, "dlms-sn-parameterized-wrapper"}
	}
	h := make([]hdlcSNReadCase, len(d.HDLC))
	for i, c := range d.HDLC {
		h[i] = hdlcSNReadCase{c, meta.HDLC[i].Steps, meta.HDLC[i].UDP, meta.HDLC[i].TCP}
	}
	return w, h
}

func TestDLMSSNParameterizedExistingAPIWrapper(t *testing.T) {
	w, _ := snParameterizedBaseline(t)
	snWriteNative(t, w)
}

func TestDLMSSNParameterizedExistingAPIHDLC(t *testing.T) {
	_, h := snParameterizedBaseline(t)
	hdlcSNReadNative(t, h, 41000, 4059)
}

func snParameterizedControls(t *testing.T) ([]snWriteControl, []hdlcSNReadCase) {
	t.Helper()
	return snParameterizedCases(t, "controls.json", "74dedf7b7c3ff91c92bb988d6e5226ea9f3f4e0c774e064191617cc52a09691b", 12, 14)
}

func TestDLMSSNParameterizedWrapperNative(t *testing.T) {
	w, _ := snParameterizedControls(t)
	snWriteNative(t, w)
}

func TestDLMSSNParameterizedWrapperMatrix(t *testing.T) {
	w, _ := snParameterizedControls(t)
	snWriteMatrix(t, w)
}

func TestDLMSSNParameterizedHDLCNative(t *testing.T) {
	_, h := snParameterizedControls(t)
	hdlcSNReadNative(t, h, 41000, 4059)
}

func TestDLMSSNParameterizedHDLCMatrix(t *testing.T) {
	_, h := snParameterizedControls(t)
	hdlcSNReadMatrix(t, h, "dlms-sn-parameterized-hdlc")
}

func snParameterizedHistoryItem(t *testing.T) hdlcSNWriteHistory {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-parameterized-access", "DLMS_SN_PARAMETERIZED_TEST_ROOT", "historical-parameterized-upgrade.json")
	require.Equal(t, "0dea92eeaeb4c09402aa7891cd200a382cb0af008da43fe42d38cdc84c433da3", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Records []hdlcSNWriteHistory }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Records, 1)
	r := d.Records[0]
	require.Equal(t, "validation-dlms-hdlc-sn-get-close.zip", r.OriginalArchive)
	require.Equal(t, "5924169d9091f4a7671148b4c198f208d62bdb9ced0b739659c33ebdee23dcbb", r.ArchiveSHA)
	require.Equal(t, "dlms-hdlc-sn-read/controls-safe-owner.json", r.Member)
	require.Equal(t, "3bab1436ded0f609435ca0a0c6337f15274a6a04ee4f0c71949b5eb61a2dc704", r.MemberSHA)
	require.Equal(t, 10, r.CaseIndex)
	require.Equal(t, 10, r.StepIndex)
	require.Equal(t, "completeframe-malformed-and-unsupported-still-independent-SN", r.Name)
	require.Equal(t, "742452650ce1d6a7a97cdca7db0bb1e497121e1fd499b7801b1b7c544aa0e539", r.WireSHA)
	snParameterizedHistoryChangedStep(t, r)
	require.Len(t, r.Captures, 2)
	for _, tr := range []string{"udp", "tcp"} {
		require.Equal(t, "dlms-hdlc-sn-read/"+r.Name+"."+tr+".pcap", r.Captures[tr].Member)
	}
	return r
}

func snParameterizedUpgradeHistoricalRead(t *testing.T, raw []byte, out []hdlcSNReadCase) []hdlcSNReadCase {
	t.Helper()
	r := snParameterizedHistoryItem(t)
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
	b, err := json.Marshal(r.Step)
	require.NoError(t, err)
	var next hdlcOpStep
	require.NoError(t, json.Unmarshal(b, &next))
	c.Steps[r.StepIndex] = next
	c.Metadata[r.StepIndex] = r.Step
	return out
}

func snParameterizedHistoryChangedStep(t *testing.T, r hdlcSNWriteHistory) {
	t.Helper()
	require.Equal(t, []string{"Decode", "GetFields", "Session", "completeness", "error", "fields", "query_error", "session", "status"}, r.Allowed)
	require.Len(t, r.Step, len(r.Original))
	allowed := map[string]bool{}
	for _, k := range r.Allowed {
		allowed[k] = true
	}
	for k, v := range r.Original {
		_, present := r.Step[k]
		require.True(t, present, k)
		if allowed[k] {
			require.NotEqual(t, v, r.Step[k], k)
		} else {
			require.Equal(t, v, r.Step[k], k)
		}
	}
	require.Nil(t, r.Step["error"])
	require.Nil(t, r.Step["query_error"])
	require.Equal(t, "decoded", r.Step["status"])
	require.Equal(t, "message", r.Step["completeness"])
}

func snParameterizedWrapperHistoryItem(t *testing.T) hdlcSNWriteHistory {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-parameterized-access", "DLMS_SN_PARAMETERIZED_TEST_ROOT", "historical-wrapper-parameterized-upgrade.json")
	require.Equal(t, "6a0dd2a6cdc3fd9e260bcbb726c41513c973883afdde2563f4ac53745e10b244", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Records []hdlcSNWriteHistory }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Records, 1)
	r := d.Records[0]
	require.Equal(t, "validation-dlms-wrapper-sn-read.zip", r.OriginalArchive)
	require.Equal(t, "bb588f49ed993af49d8d97c13370ee051c511fc5f81b09b127dbc9fdd6848873", r.ArchiveSHA)
	require.Equal(t, "dlms-wrapper-sn-read/controls.json", r.Member)
	require.Equal(t, "7351c3e218e7add499689d78f4f7cc4103f74fdfe0e39cf8b1b46fdd7aeb10b1", r.MemberSHA)
	require.Equal(t, 9, r.CaseIndex)
	require.Equal(t, 2, r.StepIndex)
	require.Equal(t, "unsupported-forms-enum-empty-indefinite-and-hard17-Resource", r.Name)
	require.Equal(t, "ded5653bc2172556e4777d989eb4a28d0df1c6125cb6e30139962ffb31e6c6b1", r.WireSHA)
	snParameterizedHistoryChangedStep(t, r)
	require.Len(t, r.Captures, 2)
	for _, tr := range []string{"udp", "tcp"} {
		require.Equal(t, "dlms-wrapper-sn-read/"+r.Name+"."+tr+".pcap", r.Captures[tr].Member)
	}
	return r
}

func snParameterizedUpgradeHistoricalWrapper(t *testing.T, raw []byte, out []snReadControl) []snReadControl {
	t.Helper()
	r := snParameterizedWrapperHistoryItem(t)
	require.Equal(t, r.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Len(t, out, 12)
	c := &out[r.CaseIndex]
	require.Equal(t, r.Name, c.Name)
	require.Len(t, c.Steps, 14)
	require.Equal(t, r.Original, c.Steps[r.StepIndex])
	require.Equal(t, r.WireSHA, c.Steps[r.StepIndex]["input_sha256"])
	c.Steps[r.StepIndex] = r.Step
	return out
}

func TestDLMSSNParameterizedHistoricalBinding(t *testing.T) {
	r := snParameterizedWrapperHistoryItem(t)
	raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+r.Member)
	require.Equal(t, r.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var old struct{ Cases []snReadControl }
	require.NoError(t, json.Unmarshal(raw, &old))
	got := snReadControls(t)
	require.Len(t, got, len(old.Cases))
	for i, c := range old.Cases {
		if i == r.CaseIndex {
			require.Equal(t, r.Original, c.Steps[r.StepIndex])
			c.Steps[r.StepIndex] = r.Step
			next := snBlocksHistoricalItem(t)
			require.Equal(t, next.Original, c.Steps[next.StepIndex])
			c.Steps[next.StepIndex] = next.Step
		}
		require.Equal(t, c, got[i], "only the independently pinned parameter/selected-block metadata rows may change; budgets, Close and every other field stay exact")
	}
	for _, tr := range []string{"udp", "tcp"} {
		t.Run(tr, func(t *testing.T) {
			capture := r.Captures[tr]
			raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+capture.Member)
			require.Equal(t, capture.SHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
		})
	}
}
