package ssatools

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestSyntaxFlowDocToolsQueryEmbeddedIndex verifies the global scenario tools
// resolve against the embedded SyntaxFlowDoc snapshot, matching what the
// write_syntaxflow_rule focus mode sees through its loop actions.
func TestSyntaxFlowDocToolsQueryEmbeddedIndex(t *testing.T) {
	out, err := syntaxFlowDocSearchCallback(aitool.InvokeParams{
		"query":    "golang-gin-context",
		"category": "builtin_lib",
		"limit":    int64(10),
	}, io.Discard, io.Discard)
	require.NoError(t, err)
	require.Contains(t, out, "golang-gin-context")

	details, err := syntaxFlowDocBuiltinLibDetailsCallback(aitool.InvokeParams{
		"name": []any{"golang-gin-context"},
	}, io.Discard, io.Discard)
	require.NoError(t, err)
	require.Contains(t, details, "golang-gin-context")

	tools, err := CreateSSATools()
	require.NoError(t, err)
	var names []string
	for _, tool := range tools {
		names = append(names, tool.GetName())
	}
	joined := strings.Join(names, ",")
	require.Contains(t, joined, SyntaxFlowToolName_DocSearch)
	require.Contains(t, joined, SyntaxFlowToolName_BuiltinLibDetails)
}
