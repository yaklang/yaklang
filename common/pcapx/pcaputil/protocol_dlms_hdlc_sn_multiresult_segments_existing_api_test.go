package pcaputil

// CC0 source-only public session regression material; child has not executed Go.
import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snMultiSegmentsIndependentBaseline(t *testing.T) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-multiresult-segments-source", "DLMS_HDLC_SN_MULTIRESULT_SEGMENTS_ROOT", "baseline-controls.json")
	require.Equal(t, "1ef63d6d2338dbf7a9753b031839fde2734e756282b33489cb223c23e9ef3455", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var g struct{ Cases []hdlcOpCase }
	var m struct {
		Cases []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &g))
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Len(t, g.Cases, 3)
	require.Len(t, m.Cases, 3)
	out := make([]hdlcSNReadCase, 3)
	for i, c := range g.Cases {
		require.Empty(t, c.Close.Events)
		for j := range m.Cases[i].Steps {
			if msg, ok := m.Cases[i].Steps[j]["error_message"]; ok {
				m.Cases[i].Steps[j]["query_error_message"] = msg
			}
		}
		out[i] = hdlcSNReadCase{c, m.Cases[i].Steps, m.Cases[i].UDP, m.Cases[i].TCP}
	}
	return out
}
func TestDLMSHDLCSNMultiResultSegmentsExistingAPI(t *testing.T) {
	// Existing strict helper runs only NewProtocolSession/Feed/query/Close and
	// exact literal fields/types/Session/IDs/offset/Raw/flow/poison assertions.
	snInformationCountNative(t, snMultiSegmentsIndependentBaseline(t), 41000, 4059)
}
