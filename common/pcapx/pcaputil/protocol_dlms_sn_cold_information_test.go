package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snColdInformationLoad(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-cold-information-source", "DLMS_SN_COLD_INFORMATION_ROOT", name)
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

func TestDLMSSNColdInformationNative(t *testing.T) {
	snInformationCountNativeWithIDSeed(t, snColdInformationLoad(t, "cold-controls.json", "3e815c0a42658a8436e8ced80021b5a6aef16a84195daa98e14dbf8b1b987f02", 1), 41000, 4059, 10000)
}
func TestDLMSSNColdInformationMatrix(t *testing.T) {
	cases := snColdInformationLoad(t, "cold-captured-controls.json", "5a58d523fa9964b983713842be6dc307f01e5ffc1d705aabbb062d5c8e0e6195", 1)
	hdlcSNReadMatrixChecked(t, cases, "dlms-sn-cold-information-source/captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
		require.Equal(t, meta["public_error"], e.Error)
		actionPoison(e)
		hdlcSNReadQueries(t, e, st)
	})
}

// These vectors specify DLMS admission only. Another detector's fallback is
// deliberately not promoted to a DLMS semantic oracle.
func TestDLMSSNColdInformationAdmissionBoundaries(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-cold-information-source", "DLMS_SN_COLD_INFORMATION_ROOT", "cold-probe-controls.json")
	require.Equal(t, "438d32543fbb334798f67ccf72cafae5b8eff326a94bfd57633ba350c3a90e44", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var g struct {
		Vectors []struct {
			Name     string      `json:"name"`
			Wire     string      `json:"input_hex"`
			SHA      string      `json:"input_sha256"`
			Expected ProbeResult `json:"expected_probe_result"`
		} `json:"vectors"`
	}
	require.NoError(t, json.Unmarshal(raw, &g))
	require.Len(t, g.Vectors, 4)
	for _, tr := range []string{"tcp", "udp"} {
		for _, v := range g.Vectors {
			t.Run(v.Name+"/"+tr, func(t *testing.T) {
				wire := wrapperWire(t, v.Wire)
				require.Equal(t, v.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
				require.Equal(t, ProbeResult{Verdict: ProbeReject}, v.Expected)
				require.Equal(t, v.Expected, probeDLMS(wire, DefaultParserBudget().MaxFrameBytes))
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				before := s.Stats()
				r := s.Probe(wire)
				require.Equal(t, before, s.Stats())
				require.False(t, r.Protocol == "dlms" && r.Verdict == ProbeAccept)
				require.Zero(t, s.Stats().BufferedBytes)
				require.Empty(t, s.Close("rejected-probe"))
				require.Empty(t, s.Close("again"))
			})
		}
	}
}

func TestDLMSSNColdInformationResource(t *testing.T) {
	snInformationCountNativeWithIDSeed(t, snColdInformationLoad(t, "cold-resource-controls.json", "15df0fb46d26ea7e7ab33b2b40fc846d2a1b2ba76e267dc79147b122a472b993", 2), 41000, 4059, 10000)
}
func TestDLMSSNColdInformationCommandAdmission(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-cold-information-source", "DLMS_SN_COLD_INFORMATION_ROOT", "cold-probe-command-controls.json")
	require.Equal(t, "eecce968590f8623478c5e33c9393d20df707ada72e4dff4276c3602ea818442", fmt.Sprintf("%x", sha256.Sum256(raw)))
	type vector struct {
		Command   byte        `json:"command"`
		Segmented bool        `json:"segmented"`
		Wire      string      `json:"input_hex"`
		SHA       string      `json:"input_sha256"`
		Expected  ProbeResult `json:"expected_probe_result"`
		Reject    ProbeResult `json:"expected_DLMS_probe_result"`
	}
	var g struct {
		Positive []vector `json:"positive_vectors"`
		Negative []vector `json:"wrong_LLC_vectors"`
	}
	require.NoError(t, json.Unmarshal(raw, &g))
	require.Len(t, g.Positive, 6)
	require.Len(t, g.Negative, 6)
	for _, tr := range []string{"tcp", "udp"} {
		for _, reject := range []bool{false, true} {
			vectors := g.Positive
			if reject {
				vectors = g.Negative
			}
			for _, v := range vectors {
				t.Run(fmt.Sprintf("command%d/M%v/reject%v/%s", v.Command, v.Segmented, reject, tr), func(t *testing.T) {
					wire := wrapperWire(t, v.Wire)
					require.Equal(t, v.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(41000, 4059))
					require.NoError(t, err)
					defer s.Close("cleanup")
					before := s.Stats()
					r := s.Probe(wire)
					require.Equal(t, before, s.Stats())
					if reject {
						require.Equal(t, ProbeResult{Verdict: ProbeReject}, v.Reject)
						require.Equal(t, v.Reject, probeDLMS(wire, DefaultParserBudget().MaxFrameBytes))
						require.False(t, r.Protocol == "dlms" && r.Verdict == ProbeAccept)
					} else {
						require.Equal(t, v.Expected, r)
					}
					require.Empty(t, s.Close("probe-only"))
					require.Empty(t, s.Close("again"))
				})
			}
		}
	}
}
