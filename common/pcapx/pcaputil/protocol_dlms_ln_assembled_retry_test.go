package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func lnAssembledRetryLoad(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-ln-assembled-retry-source", "DLMS_LN_ASSEMBLED_RETRY_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
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
	require.Len(t, g.Cases, count)
	require.Len(t, m.Cases, count)
	out := make([]hdlcSNReadCase, count)
	for i, c := range g.Cases {
		for j := range m.Cases[i].Steps {
			// Original positive gold specifies semantic fields and the retained flow;
			// derive only fixed carrier/public-success metadata, never parser answers.
			st := c.Steps[j]
			meta := m.Cases[i].Steps[j]
			require.Contains(t, []int{0, 1}, st.Direction)
			ends := []string{"192.0.2.1:41000", "192.0.2.2:4059"}
			expected := map[string]any{"source": ends[st.Direction], "destination": ends[1-st.Direction], "capture_udp_direction": float64(st.Direction), "native_udp_flow_nonzero": true, "captured_udp_flow_nonzero": true, "PacketRef_udp": float64(j + 1), "PacketRef_tcp": float64(j + 4)}
			if st.Error == "" {
				expected["public_error"] = ""
			}
			for key, value := range expected {
				if old, ok := meta[key]; ok {
					require.Equal(t, value, old, key)
				} else {
					meta[key] = value
				}
			}
			if message, ok := m.Cases[i].Steps[j]["error_message"]; ok {
				m.Cases[i].Steps[j]["query_error_message"] = message
			}
		}
		out[i] = hdlcSNReadCase{c, m.Cases[i].Steps, m.Cases[i].UDP, m.Cases[i].TCP}
	}
	return out
}
func TestDLMSLNAssembledOpaqueRetryNative(t *testing.T) {
	snInformationCountNative(t, lnAssembledRetryLoad(t, "captured-controls.json", "f84eab12832c9ff7fb502dc598aaf6f03565372e689773ea1543552753cfddb8", 3), 41000, 4059)
}
func TestDLMSLNAssembledOpaqueRetryMatrix(t *testing.T) {
	hdlcSNReadMatrixChecked(t, lnAssembledRetryLoad(t, "captured-controls.json", "f84eab12832c9ff7fb502dc598aaf6f03565372e689773ea1543552753cfddb8", 3), "dlms-ln-assembled-retry-source/captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
		require.Equal(t, meta["public_error"], e.Error)
		if st.Error != "" {
			snMultiOwnedDiagnosis(t, e, st.Error, meta["error_message"].(string))
		}
		actionPoison(e)
		hdlcSNReadQueries(t, e, st)
		if st.Error != "" {
			snMultiOwnedDiagnosis(t, e, st.Error, meta["error_message"].(string))
		}
	})
}
