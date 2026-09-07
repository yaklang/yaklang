package ssaapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceQueryBatchesHighVolumeRegexHits(t *testing.T) {
	files := map[string]string{
		"many.txt": strings.Repeat("key\n", 513),
	}
	target := NewSourceQueryTarget(t.Name(), files)
	result, err := target.SyntaxFlowWithError(`
desc(
	mode: "source"
	language: "general"
	title: "batch source hits"
)
${many.txt}.pattern_regex(/key/) as $hits
alert $hits for {
	level: "info",
	title: "batch source hits",
}
`)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 513, len(result.GetAlertValue("hits")))
}
