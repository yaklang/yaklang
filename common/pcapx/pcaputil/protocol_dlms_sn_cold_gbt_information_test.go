package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snColdGBTInformationLoad(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-cold-gbt-information-source", "DLMS_SN_COLD_GBT_INFORMATION_ROOT", name)
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
		out[i] = hdlcSNReadCase{c, m.Cases[i].Steps, m.Cases[i].UDP, m.Cases[i].TCP}
	}
	return out
}
func TestDLMSSNColdGBTInformationExistingAPI(t *testing.T) {
	snInformationCountNativeWithIDSeed(t, snColdGBTInformationLoad(t, "cold-gbt-baseline-controls.json", "824a49ebcbd363fffd567ad1531ff19f9f2f70f054b9395a35a4bc907528adcb", 2), 41000, 4059, 10000)
}
func TestDLMSSNColdGBTInformationNative(t *testing.T) {
	snInformationCountNativeWithIDSeed(t, snColdGBTInformationLoad(t, "cold-gbt-controls.json", "ef6d3752d51d5afd0ef93e556d901020ec97e69ec99bf272e059fcd86e452105", 7), 41000, 4059, 10000)
}

func TestDLMSSNColdGBTInformationRequestNative(t *testing.T) {
	snInformationCountNativeWithIDSeed(t, snColdGBTInformationLoad(t, "cold-gbt-request-controls.json", "5e9b4af83c873c3c963cd7c21e9dbe3a3bcd64e49c3ac801022d386fd6ae0183", 1), 41000, 4059, 10000)
}

// Only DLMS admission is specified for negative vectors; another detector's
// fallback must not become a fabricated DLMS semantic answer.
func TestDLMSSNColdGBTInformationAdmission(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-cold-gbt-information-source", "DLMS_SN_COLD_GBT_INFORMATION_ROOT", "cold-gbt-probe-controls.json")
	require.Equal(t, "7a58f90d0fab2165f78233215b33e9d1e6a02934fe40dc44d61b2816656d915c", fmt.Sprintf("%x", sha256.Sum256(raw)))
	type vector struct {
		Name     string      `json:"name"`
		Wire     string      `json:"input_hex"`
		SHA      string      `json:"input_sha256"`
		Expected ProbeResult `json:"expected_probe_result"`
		Reject   ProbeResult `json:"expected_DLMS_probe_result"`
	}
	var g struct {
		Positive vector   `json:"positive"`
		Negative []vector `json:"negative_vectors"`
	}
	require.NoError(t, json.Unmarshal(raw, &g))
	require.Len(t, g.Negative, 4)
	g.Positive.Name = "cold-valid-E0-first-carrier"
	vectors := append([]vector{g.Positive}, g.Negative...)
	for _, tr := range []string{"tcp", "udp"} {
		for i, v := range vectors {
			t.Run(v.Name+"/"+tr, func(t *testing.T) {
				w := wrapperWire(t, v.Wire)
				require.Equal(t, v.SHA, fmt.Sprintf("%x", sha256.Sum256(w)))
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				before := s.Stats()
				r := s.Probe(w)
				require.Equal(t, before, s.Stats())
				if i == 0 {
					require.Equal(t, v.Expected, probeDLMS(w, DefaultParserBudget().MaxFrameBytes))
					require.Equal(t, v.Expected, r)
				} else {
					require.Equal(t, ProbeResult{Verdict: ProbeReject}, v.Reject)
					require.Equal(t, v.Reject, probeDLMS(w, DefaultParserBudget().MaxFrameBytes))
					require.False(t, r.Protocol == "dlms" && r.Verdict == ProbeAccept)
				}
				require.Zero(t, s.Stats().BufferedBytes)
				require.Empty(t, s.Close("probe-only"))
				require.Empty(t, s.Close("again"))
			})
		}
	}
}

func TestDLMSSNColdGBTInformationMatrix(t *testing.T) {
	cases := snColdGBTInformationLoad(t, "cold-gbt-captured-controls.json", "c7f36c883b6ae8cf4534791c7b6c06be23c3e8828d79c1e61d2a2ad93006c660", 1)
	cases = append(cases, snColdGBTInformationLoad(t, "cold-gbt-request-captured-controls.json", "9cfa3cba49f7dc74bffde0234cb3ee8643da5b1c50dac1de3e359ee194780dc2", 1)...)
	hdlcSNReadMatrixChecked(t, cases, "dlms-sn-cold-gbt-information-source/captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
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
