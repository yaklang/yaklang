package bin_parser

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSPNEGOScoresMatchCurrentRuleScope(t *testing.T) {
	const rule = "application-layer/spnego.yaml"
	// An explanation of unparsed structured fields must not become an opaque
	// exemption that raises the score above the rule's actual schema ceiling.
	require.Equal(t, 15, schemaCeiling(rule, ""))
	require.Equal(t, 15, schemaCeiling(rule, spnegoUnparsedFields))

	childNames := p1MustChildNames()
	for _, group := range []struct {
		cards   []ProtocolScorecard
		resolve func(string) (ProtocolScorecard, bool)
		doc     string
	}{
		{P0Scorecards, ResolveP0Scorecard, "P0_SCORES.md"},
		{P1Scorecards, ResolveP1Scorecard, "P1_SCORES.md"},
	} {
		for _, entry := range group.cards {
			sc, ok := group.resolve(entry.Name)
			require.True(t, ok)
			if sc.Rule != rule {
				continue
			}
			t.Run(entry.Name, func(t *testing.T) {
				require.Equal(t, []int{15, 25, 16, 14, 10}, []int{sc.Schema, sc.Traffic, sc.Tests, sc.Branches, sc.Stack})
				require.Equal(t, spnegoScoreEvidence, sc.Evidence)
				require.Equal(t, spnegoUnparsedFields, sc.OpaqueRaw)
				require.True(t, hasEthernetMustChild(sc, childNames), "Traffic 25 requires full-stack named-field assertions")
				for _, evidence := range []string{"spnego/ntlm", "/spnego/krb5", "MechOID", "TCP/445", "TestSPNEGONestedLengthValidation", "TestSPNEGOAdditionalMechanismAndOpaqueOptionalFields"} {
					require.Contains(t, sc.Evidence, evidence)
				}
				for _, unparsed := range []string{"Optional Fields", "reqFlags/mechToken/mechListMIC", "Octets", "NegTokenResp", "long-form BER"} {
					require.Contains(t, sc.OpaqueRaw, unparsed)
				}
				sc = deriveP1Gates(sc, failCount(sc.Rule), true, true)
				require.True(t, sc.GatesOK())
				require.Equal(t, 80, sc.Total())
				require.Equal(t, "B", sc.Grade())

				doc, err := os.ReadFile(group.doc)
				require.NoError(t, err)
				row := fmt.Sprintf("| %s | B | 80 | 15 | 25 | 16 | 14 | 10 | L2 | `%s` |", entry.Name, rule)
				require.Contains(t, string(doc), row, "score documentation must match the actual card")
				require.Contains(t, string(doc), "Optional Fields")
				require.Contains(t, string(doc), "tokenless 401 challenge 的 outer-only")
			})
		}
	}

	p0, ok := ResolveP0Scorecard("SPNEGO")
	require.True(t, ok)
	p1, ok := ResolveP1Scorecard("GSS-API")
	require.True(t, ok)
	require.Equal(t, p0.Rule, p1.Rule)
	require.True(t, strings.Contains(p1InventoryMarkdown(), "GSS-API | B | 80 | 15 | 25 | 16 | 14 | 10"))
	// The scorecard still counts only the legacy SPNEGO rule. The independent
	// corpus profile decodes tokens, rejects the incomplete original, and does
	// not mistake a tokenless HTTP 401 challenge for decoded token fields.
	contract := protocolCorpusParseContracts["GSS-API"]
	require.False(t, contract.OuterOnly)
	require.Equal(t, "application-layer/gssapi.yaml", contract.RuleFile)
	require.Equal(t, "GSSAPIHTTP", contract.EntryNode)
	spec, ok := protocolCorpusRejectionSpecs["pr5023-gen-gssapi-http"]
	require.True(t, ok)
	require.Equal(t, contract, spec.Contract)
	require.Equal(t, contract, spec.ControlContract)
	require.Equal(t, "gen-gssapi-valid", spec.ControlCaptureID)
	require.Equal(t, protocolCorpusFailureInvalidValue, spec.ExpectedFailureClass)
	require.Equal(t, "missing SPNEGO negotiation token after mechanism OID", spec.ExpectedErrorContains)
}
