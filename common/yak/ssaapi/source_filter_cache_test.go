package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceQueryTargetSharesFileFilterHits(t *testing.T) {
	target := NewSourceQueryTarget("t", map[string]string{
		"A.java": "call tx.origin and CipherMode.ECB",
	})
	first := `
desc(mode: "source", language: general, title: t)
${*}.pattern_regex(/tx\.origin/) as $hit
alert $hit
`
	res, err := target.SyntaxFlowWithError(first)
	require.NoError(t, err)
	require.NotEmpty(t, res.GetAlertVariables())
	_, err = target.SyntaxFlowWithError(first)
	require.NoError(t, err)
	require.Equal(t, 1, target.SourceFilterComputes())

	_, err = target.SyntaxFlowWithError(`
desc(mode: "source", language: general, title: t2)
${*}.pattern_regex(/CipherMode\.ECB/) as $hit
alert $hit
`)
	require.NoError(t, err)
	require.Equal(t, 2, target.SourceFilterComputes())

	_, err = target.SyntaxFlowWithError(first)
	require.NoError(t, err)
	require.Equal(t, 2, target.SourceFilterComputes())
}
