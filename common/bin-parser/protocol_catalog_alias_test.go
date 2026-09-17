package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The catalog describes available library entries, while a corpus contract
// describes what one fixture can prove. Adding a limited fixture for an
// existing alias must not replace its canonical library registration.
func TestProtocolCatalogP0AliasesMatchCanonicalEntries(t *testing.T) {
	catalog := make(map[string]ProtocolInfo, len(ProtocolCatalog))
	for _, item := range ProtocolCatalog {
		catalog[item.Name] = item
	}
	for _, score := range P0Scorecards {
		if score.AliasOf == "" {
			continue
		}
		item, registered := catalog[score.Name]
		if !registered {
			continue // Some historical scorecard aliases have no catalog row.
		}
		t.Run(score.Name, func(t *testing.T) {
			canonical, exists := catalog[score.AliasOf]
			require.True(t, exists, "catalog alias %s has no canonical registration %s", item.Name, score.AliasOf)
			require.NotEqual(t, item.Name, canonical.Name)
			require.Equal(t, canonical.RuleFile, item.RuleFile)
			require.Equal(t, canonical.EntryNode, item.EntryNode)
			require.Equal(t, canonical.Layer, item.Layer)
			require.Equal(t, canonical.Status, item.Status, "an alias must not independently upgrade or downgrade its canonical entry")
		})
	}
	for _, name := range []string{"CIFS", "NTLM v1/v2"} {
		_, registered := catalog[name]
		require.True(t, registered, "the observed alias must retain a catalog entry")
	}
	// The original CIFS request remains a negative, while the independent
	// companion exercises the complete bounded NEGOTIATE request profile.
	// Neither changes the established generic SMB alias above.
	cifs := protocolCorpusParseContracts["CIFS"]
	require.Equal(t, "application-layer/cifs.yaml", cifs.RuleFile)
	require.Equal(t, "CIFSDirectTCP", cifs.EntryNode)
	require.Zero(t, cifs.InputLength)
	require.False(t, cifs.OuterOnly)
	profile, registered := catalog["CIFS Negotiate Request"]
	require.True(t, registered)
	require.Equal(t, cifs.RuleFile, profile.RuleFile)
	require.Equal(t, cifs.EntryNode, profile.EntryNode)
	original, negative := protocolCorpusRejectionSpecs["pr5023-gen-cifs"]
	require.True(t, negative)
	require.Equal(t, "gen-cifs-valid", original.ControlCaptureID)
}
