package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func snBlockControls(t *testing.T, native bool) []snWriteControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-read-service-block", "DLMS_SN_READ_BLOCK_TEST_ROOT", "controls.json")
	require.Equal(t, "8b81bcf95895c6cce04c8ddbaab958bc3df734507a2e79abde2f806f8f8c2eeb", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []snReadControl }
	var h struct {
		Cases []struct {
			UDP        string          `json:"sha256"`
			TCP        string          `json:"tcp_sha256"`
			UDPClosing snReadCloseSpec `json:"native_udp_Close"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.NoError(t, json.Unmarshal(raw, &h))
	require.Len(t, d.Cases, 26)
	require.Len(t, h.Cases, 26)
	d.Cases = snUploadUpgradeHistoricalDownload(t, raw, d.Cases)
	out := make([]snWriteControl, len(d.Cases))
	for i, c := range d.Cases {
		if native {
			c.Close = h.Cases[i].UDPClosing
		}
		out[i] = snWriteControl{c, h.Cases[i].UDP, h.Cases[i].TCP, "dlms-sn-read-block-captures"}
	}
	return out
}

func TestDLMSWrapperSNBlocksNative(t *testing.T) { snWriteNative(t, snBlockControls(t, true)) }
func TestDLMSWrapperSNBlocksMatrix(t *testing.T) { snWriteMatrix(t, snBlockControls(t, false)) }

func snBlocksHistoricalItem(t *testing.T) hdlcSNWriteHistory {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-read-service-block", "DLMS_SN_READ_BLOCK_TEST_ROOT", "historical-selected-block-diagnostic.json")
	require.Equal(t, "0ff9f25d0e87efb35aa66ea4ff1157668cf4f3c3f3bea1ece6b5c5ef525928ec", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Records []hdlcSNWriteHistory }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Records, 1)
	r := d.Records[0]
	require.Equal(t, "validation-dlms-wrapper-sn-read.zip", r.OriginalArchive)
	require.Equal(t, "bb588f49ed993af49d8d97c13370ee051c511fc5f81b09b127dbc9fdd6848873", r.ArchiveSHA)
	require.Equal(t, "dlms-wrapper-sn-read/controls.json", r.Member)
	require.Equal(t, "7351c3e218e7add499689d78f4f7cc4103f74fdfe0e39cf8b1b46fdd7aeb10b1", r.MemberSHA)
	require.Equal(t, 9, r.CaseIndex)
	require.Equal(t, 4, r.StepIndex)
	require.Equal(t, "unsupported-forms-enum-empty-indefinite-and-hard17-Resource", r.Name)
	require.Equal(t, "fcdba84c45e5f4a83b35a73ef329cccedeefb0e3342456a45e1e7fdf4e7e9659", r.WireSHA)
	require.Equal(t, []string{"completeness", "error", "profile", "query_error", "status"}, r.Allowed)
	allowed := map[string]any{"profile": "dlms-wrapper-v1-sn-read-block", "status": "malformed", "completeness": "malformed", "query_error": "MalformedMessage", "error": map[string]any{"kind": "MalformedMessage"}}
	require.Len(t, r.Step, len(r.Original))
	for k, v := range r.Original {
		_, present := r.Step[k]
		require.True(t, present, k)
		if next, ok := allowed[k]; ok {
			require.NotEqual(t, v, r.Step[k], k)
			require.Equal(t, next, r.Step[k], k)
		} else {
			require.Equal(t, v, r.Step[k], k)
		}
	}
	require.Equal(t, "00010001001000090c0102000000000100", r.Step["input_hex"])
	require.Len(t, r.Captures, 2)
	return r
}

func snBlocksUpgradeHistorical(t *testing.T, raw []byte, out []snReadControl) []snReadControl {
	t.Helper()
	r := snBlocksHistoricalItem(t)
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

func TestDLMSWrapperSNBlocksHistoricalBinding(t *testing.T) {
	r := snBlocksHistoricalItem(t)
	raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+r.Member)
	require.Equal(t, r.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	for _, tr := range []string{"udp", "tcp"} {
		t.Run(tr, func(t *testing.T) {
			c := r.Captures[tr]
			require.Equal(t, "dlms-wrapper-sn-read/"+r.Name+"."+tr+".pcap", c.Member)
			raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+c.Member)
			require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
		})
	}
}
