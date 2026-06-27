package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestValidateEndpointDeepMiningCoverage_MissingTypes(t *testing.T) {
	err := validateEndpointDeepMiningCoverage(1, 42, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing vuln probe")
}

func TestValidateEndpointDeepMiningCoverage_SkippedWithoutReason(t *testing.T) {
	probes := []store.EndpointVulnProbe{
		{VulnType: "sqli", Status: "skipped"},
	}
	for _, id := range AllVulnTypeIDs() {
		if id == "sqli" {
			continue
		}
		probes = append(probes, store.EndpointVulnProbe{VulnType: id, Status: "safe"})
	}
	err := validateEndpointDeepMiningCoverage(1, 42, probes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "skip_reason")
}

func TestValidateEndpointDeepMiningCoverage_FullCoverage(t *testing.T) {
	var probes []store.EndpointVulnProbe
	for _, id := range AllVulnTypeIDs() {
		probes = append(probes, store.EndpointVulnProbe{
			VulnType: id, Status: "skipped", SkipReason: "scope=no_get",
		})
	}
	require.NoError(t, validateEndpointDeepMiningCoverage(1, 42, probes))
}
