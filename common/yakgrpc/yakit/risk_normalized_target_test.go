package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateRiskNormalizedURLKeepsEndpointAndResolvesHost(t *testing.T) {
	for _, tc := range []struct {
		target string
		port   int
	}{{"127.0.0.1/debug", 80}, {"127.0.0.1:8443/debug?q=1", 8443}, {"127.0.0.1/?q=1#fragment", 80}} {
		target := tc.target
		t.Run(target, func(t *testing.T) {
			r := CreateRisk(target, WithRiskParam_RiskType("xss"), WithRiskParam_Parameter("q"))
			require.Equal(t, "127.0.0.1", r.Host)
			require.Equal(t, "127.0.0.1", r.IP)
			require.Equal(t, target, r.Url)
			require.EqualValues(t, tc.port, r.Port)
			require.Equal(t, ComputeRiskHash(target, "", 0, "xss", "q"), r.Hash)
		})
	}
}
