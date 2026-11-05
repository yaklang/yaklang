package bin_parser

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// Preserve the actual bounded capture contract after removing delivery scoring.
func TestSPNEGOCorpusContractsMatchBoundedProfile(t *testing.T) {
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
