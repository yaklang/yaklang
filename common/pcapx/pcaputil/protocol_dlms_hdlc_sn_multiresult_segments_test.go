package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snMultiSegmentsLoad(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-multiresult-segments-source", "DLMS_HDLC_SN_MULTIRESULT_SEGMENTS_ROOT", name)
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
		// The independent gold stores an actual test-close string; the old
		// checker accepts a reason placeholder and expands it before comparing.
		for j := range c.Close.Events {
			e := &c.Close.Events[j]
			if e.Profile == "dlms-hdlc-sn-information-segmented" {
				require.Equal(t, "test-close: "+e.SummaryUDP, e.SummaryTCP)
				e.SummaryTCP = "<reason>: " + e.SummaryUDP
			}
		}
		for j := range m.Cases[i].Steps {
			if message, ok := m.Cases[i].Steps[j]["error_message"]; ok {
				m.Cases[i].Steps[j]["query_error_message"] = message
			}
		}
		out[i] = hdlcSNReadCase{c, m.Cases[i].Steps, m.Cases[i].UDP, m.Cases[i].TCP}
	}
	return out
}
func TestDLMSHDLCSNMultiResultSegmentsNative(t *testing.T) {
	snInformationCountNative(t, snMultiSegmentsLoad(t, "controls.json", "fc75d5cec9be7ac4bb61d77f7fbcf57271d157e3feae30fb10fbd27cf0049de3", 8), 41000, 4059)
}

func TestDLMSHDLCSNMultiResultSegmentsFirstChoice(t *testing.T) {
	snInformationCountNative(t, snMultiSegmentsLoad(t, "first-choice-controls.json", "f17a7790d3a62ecfa056fc8069b100bf1c672cc7d59a880541c4a75769bf41d3", 1), 41000, 4059)
}
func TestDLMSHDLCSNMultiResultSegmentsTailIsolation(t *testing.T) {
	snInformationCountNative(t, snMultiSegmentsLoad(t, "lookalike-controls.json", "9bd9929242ebf64111a66428140c9ccd73f3290b657cb5f67f6b9ff296986d66", 2), 41000, 4059)
}

func TestDLMSHDLCSNMultiResultSegmentsMatrix(t *testing.T) {
	cs := snMultiSegmentsLoad(t, "controls.json", "fc75d5cec9be7ac4bb61d77f7fbcf57271d157e3feae30fb10fbd27cf0049de3", 8)
	cs = append(cs, snMultiSegmentsLoad(t, "first-choice-controls.json", "f17a7790d3a62ecfa056fc8069b100bf1c672cc7d59a880541c4a75769bf41d3", 1)...)
	cs = append(cs, snMultiSegmentsLoad(t, "lookalike-controls.json", "9bd9929242ebf64111a66428140c9ccd73f3290b657cb5f67f6b9ff296986d66", 2)...)
	hdlcSNReadMatrixChecked(t, cs, "dlms-hdlc-sn-multiresult-segments-captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
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
