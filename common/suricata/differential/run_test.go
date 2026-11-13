package differential

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestYaklangDNSQueryCountOnPinnedCapture(t *testing.T) {
	const fixture = "../../bin-parser/testdata/protocol-corpus/captures/ndpi/ndpi-dns.pcap"
	pcapPath, err := trafficfixture.Materialize(fixture, t.TempDir())
	require.NoError(t, err)

	rulesPath := filepath.Join(t.TempDir(), "dns.rules")
	require.NoError(t, os.WriteFile(rulesPath, []byte(`alert dns any any -> any any (dns.query.name; content:"www.github.com"; sid:9902001; rev:1;)`), 0o600))

	result := RunYaklang(rulesPath, pcapPath)
	require.Empty(t, result.Error)
	require.Equal(t, []Alert{{GID: 1, SID: 9902001, Rev: 1, Count: 2}}, result.Alerts)
}

func TestCompareResults(t *testing.T) {
	results := []Result{
		{Engine: EngineYaklang, Alerts: []Alert{{GID: 1, SID: 10, Rev: 1, Count: 2}, {GID: 1, SID: 11, Rev: 1, Count: 1}}},
		{Engine: EngineSuricata, Alerts: []Alert{{GID: 1, SID: 10, Rev: 1, Count: 1}, {GID: 1, SID: 12, Rev: 1, Count: 1}}},
	}
	comparison := compareResults(results)
	require.False(t, comparison.Equal)
	require.Equal(t, []AlertIdentity{{GID: 1, SID: 11, Rev: 1}}, comparison.LocalOnly)
	require.Equal(t, []AlertIdentity{{GID: 1, SID: 12, Rev: 1}}, comparison.ReferenceOnly)
	require.Equal(t, []CountMismatch{{Engine: EngineSuricata, Identity: AlertIdentity{GID: 1, SID: 10, Rev: 1}, Local: 2, Reference: 1}}, comparison.CountMismatch)
}

func TestCompareResultsReportsRunnerFailure(t *testing.T) {
	comparison := compareResults([]Result{
		{Engine: EngineYaklang},
		{Engine: EngineSnort, Error: "failed"},
	})
	require.False(t, comparison.Equal)
	require.Equal(t, "failed", comparison.Failures[EngineSnort])
}

func TestCompareResultsRejectsDifferentRuleLoadCounts(t *testing.T) {
	comparison := compareResults([]Result{
		{Engine: EngineYaklang, RuleStats: &RuleStats{Loaded: 2}},
		{Engine: EngineSuricata, RuleStats: &RuleStats{Loaded: 1}},
	})
	require.False(t, comparison.Equal)
	require.Equal(t, "rule load count differs: local=2 reference=1", comparison.Failures[EngineSuricata])
}
