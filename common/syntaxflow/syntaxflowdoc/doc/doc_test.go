package doc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/syntaxflowdoc"
)

func TestEmbedDocumentAvailable(t *testing.T) {
	require.True(t, IsDocumentAvailable())
	h := GetDefaultDocumentHelper()
	require.Greater(t, h.Total(), 100)
	require.NotEmpty(t, h.NativeCalls)
	require.NotEmpty(t, h.BuiltinLibs)
	require.NotNil(t, h.GetNativeCall("include"))
	require.NotNil(t, h.GetNativeCall("npd"), "npd must be overlaid from live ssaapi.NativeCallDocuments")
	require.NotNil(t, h.GetBuiltinLib("golang-gin-context"))

	hits := SearchDocument("gin context include", 8, "")
	require.NotEmpty(t, hits)

	hits = SearchDocument("dataflow", 5, syntaxflowdoc.CategoryNativeCall)
	require.NotEmpty(t, hits)
}
