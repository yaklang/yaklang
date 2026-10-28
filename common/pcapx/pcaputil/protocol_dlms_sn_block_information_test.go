package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snBlockInformationLoad(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-serviceblock-information-source", "DLMS_SN_BLOCK_INFORMATION_ROOT", name)
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
		for j := range c.Close.Events {
			e := &c.Close.Events[j]
			if e.Profile == "dlms-hdlc-sn-information-segmented" {
				require.Equal(t, "test-close: "+e.SummaryUDP, e.SummaryTCP)
				e.SummaryTCP = "<reason>: " + e.SummaryUDP
			}
		}
		out[i] = hdlcSNReadCase{c, m.Cases[i].Steps, m.Cases[i].UDP, m.Cases[i].TCP}
	}
	return out
}
func TestDLMSSNBlockInformationNative(t *testing.T) {
	snInformationCountNativeWithIDSeed(t, snBlockInformationLoad(t, "full-controls-v2.json", "7d6234320cba67c3eaee56bdfa81890da6b87a6cd6c34d582c8fe11dd07d6221", 6), 41000, 4059, 10000)
}

func TestDLMSSNBlockInformationFullExistingAPI(t *testing.T) {
	snInformationCountNative(t, snBlockInformationLoad(t, "baseline-controls.json", "e90c3364be1ebd3c3de6c7600ce75adfbc218b360da40f2516d1c9fec6ddf251", 3), 41000, 4059)
}
func TestDLMSSNBlockInformationMatrix(t *testing.T) {
	cases := snBlockInformationLoad(t, "captured-controls-v2.json", "d4fd261b5c624565f36bc51c571bc38720bb51a3e165072c0f60510a94fe0c3c", 6)
	neighbor := snBlockInformationLoad(t, "captured-baseline-controls.json", "2318c8746efe9b611dd8f062787be21c60b6a7d7373b31bfad35a3e23cf6ddb5", 3)
	cases = append(cases, neighbor[2])
	hdlcSNReadMatrixChecked(t, cases, "dlms-sn-serviceblock-information-source/captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
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
