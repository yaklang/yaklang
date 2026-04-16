package aireact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestCapabilitySearch_SplitCatalogIntoChunksRespectsLimit(t *testing.T) {
	catalog := strings.Join([]string{
		"[tool:a]: A - first",
		"[tool:b]: B - second",
		"[forge:c]: C - third",
	}, "\n")

	chunks := reactloops.SplitCapabilityCatalogIntoChunks(catalog, 30)

	require.Greater(t, len(chunks), 1)
	for _, chunk := range chunks {
		require.LessOrEqual(t, len(chunk), 40)
	}
	require.Contains(t, strings.Join(chunks, ""), "[tool:a]")
	require.Contains(t, strings.Join(chunks, ""), "[forge:c]")
}

func TestCapabilitySearch_BuildEnrichmentMarkdownFiltersRecommended(t *testing.T) {
	details := []reactloops.CapabilityDetail{
		{CapabilityName: "synscan", CapabilityType: "tool", Description: "SYN scanner"},
		{CapabilityName: "report", CapabilityType: "forge", Description: "Report generator"},
		{CapabilityName: "unused", CapabilityType: "skill", Description: "Unused skill"},
	}

	md := reactloops.BuildCapabilityEnrichmentMarkdown(details, map[string]bool{
		"synscan": true,
		"report":  true,
	})

	require.Contains(t, md, "Tools")
	require.Contains(t, md, "Forges")
	require.Contains(t, md, "synscan")
	require.Contains(t, md, "report")
	require.NotContains(t, md, "unused")
}

func TestCapabilitySearch_DetailsJSONRoundTrip(t *testing.T) {
	details := []reactloops.CapabilityDetail{
		{CapabilityName: "tool-a", CapabilityType: "tool", Description: "desc-a"},
		{CapabilityName: "forge-b", CapabilityType: "forge", Description: "desc-b"},
	}

	encoded := reactloops.MarshalCapabilityDetails(details)
	decoded := reactloops.ParseCapabilityDetails(encoded)

	require.Equal(t, details, decoded)
}
