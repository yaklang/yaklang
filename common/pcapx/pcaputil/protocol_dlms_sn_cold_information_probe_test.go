package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// Same fixed-source authored Q7 carrier and adjacent complete-M0 Q7. Admission
// is checked through the existing public API, with no preceding LN exchange.
func TestDLMSSNColdInformationProbeExistingAPI(t *testing.T) {
	for _, tr := range []string{"tcp", "udp"} {
		for _, tc := range []struct{ name, wire, version string }{
			{"cold-segment", "7ea816032132b630e6e60006010700000101090401c4207e", "hdlc-sn-information-segmented"},
			{"complete-M0-neighbor", "7ea0190321326fd8e6e6000601070000010109040102100865207e", "hdlc-sn-write"},
		} {
			t.Run(tc.name+"/"+tr, func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				before := s.Stats()
				require.Equal(t, ProbeResult{Verdict: ProbeAccept, Protocol: "dlms", Version: tc.version, Confidence: 98}, s.Probe(wrapperWire(t, tc.wire)))
				require.Equal(t, before, s.Stats())
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
