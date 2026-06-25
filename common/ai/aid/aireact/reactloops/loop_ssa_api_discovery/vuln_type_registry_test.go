package loop_ssa_api_discovery

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllVulnTypeIDs_MatchesEmbeddedJSON(t *testing.T) {
	var raw []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(vulnTypeRegistryJSON, &raw))
	require.Len(t, raw, 23)

	ids := AllVulnTypeIDs()
	require.Len(t, ids, 23)
	seen := map[string]struct{}{}
	for _, id := range ids {
		seen[id] = struct{}{}
	}
	for _, entry := range raw {
		_, ok := seen[entry.ID]
		require.True(t, ok, "missing id %s from AllVulnTypeIDs", entry.ID)
	}
}

func TestVulnTypeDefByID(t *testing.T) {
	def, ok := VulnTypeDefByID("sqli")
	require.True(t, ok)
	require.Equal(t, "SQL注入漏洞", def.Name)
	require.Equal(t, "all", def.Scope)
}
