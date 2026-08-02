package loop_infosec_recon

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInfosecReconAllowedActions_BoundsSaaSMode(t *testing.T) {
	t.Parallel()

	require.ElementsMatch(t, []string{
		ToolCrawlJsCollector,
		ToolJsStaticExtractAI,
		"probe_api_candidates",
	}, infosecReconAllowedActions(true, false))

	desktop := infosecReconAllowedActions(false, false)
	require.Contains(t, desktop, "scan_port")
	require.Contains(t, desktop, ToolCrawlJsCollector)
	require.Contains(t, desktop, "api_pool_merge")
}

func TestNormalizeSaaSReconScope_DefaultsToExactSeedHost(t *testing.T) {
	t.Parallel()

	scope, err := normalizeSaaSReconScope("http://127.0.0.1:18080/example", "")
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", scope)

	scope, err = normalizeSaaSReconScope(
		"https://Business.Example:8443/example",
		"business.example",
	)
	require.NoError(t, err)
	require.Equal(t, "business.example", scope)
}

func TestNormalizeSaaSReconScope_RejectsWidening(t *testing.T) {
	t.Parallel()

	_, err := normalizeSaaSReconScope(
		"https://business.example/example",
		"business.example,other.example",
	)
	require.ErrorContains(t, err, "cannot widen")
}

func TestSaaSReconProbeSettings_AreBoundedForMultiStageDiscovery(t *testing.T) {
	t.Parallel()

	settings := boundedSaaSReconProbeSettings(99, 99, false, 99*time.Second)
	require.Equal(t, 24, settings.limit)
	require.Equal(t, 2, settings.concurrency)
	require.True(t, settings.useHead)
	require.Equal(t, 5*time.Second, settings.timeout)
	require.False(t, settings.followRedirects)
}

func TestAddSaaSReconSeedCandidate_AddsOnlyDeclaredURL(t *testing.T) {
	t.Parallel()

	pool := &APIPool{Entries: []APIPoolEntry{}}
	added, errs := addSaaSReconSeedCandidate(
		pool,
		"https://business.example/health",
		"business.example",
	)
	require.Empty(t, errs)
	require.Equal(t, 1, added)
	require.Equal(t, []APIPoolEntry{{
		NormalizedURL: "https://business.example/health",
		Method:        http.MethodGet,
		Source:        "saas_declared_seed",
		Confidence:    1,
		Evidence:      "exact user-declared URL",
	}}, pool.Entries)
}
