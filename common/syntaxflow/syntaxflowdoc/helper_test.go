package syntaxflowdoc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildDocumentHelper_StaticAndLibs(t *testing.T) {
	h := BuildDocumentHelper(BuildOptions{
		NativeCalls: []NativeCallInfo{
			{Name: "include", Description: "include a library"},
			{Name: "dataflow", Description: "dataflow filter"},
		},
	})
	require.NotNil(t, h)
	require.Greater(t, h.Total(), 50)
	require.Len(t, h.NativeCalls, 2)
	require.NotEmpty(t, h.Operators)
	require.NotEmpty(t, h.Opcodes)
	require.NotEmpty(t, h.DescKeys)
	require.NotEmpty(t, h.BuiltinLibs)

	require.NotNil(t, h.GetNativeCall("include"))
	require.NotNil(t, h.GetBuiltinLib("golang-gin-context"))

	hits := SearchDocument(h, "include gin", 10, "")
	require.NotEmpty(t, hits)

	hits = SearchDocument(h, "dataflow", 10, CategoryNativeCall)
	require.NotEmpty(t, hits)
	require.Equal(t, "dataflow", hits[0].Name)
}

func TestBuildDocumentHelper_SkipLibs(t *testing.T) {
	h := BuildDocumentHelper(BuildOptions{SkipBuiltinLibs: true})
	require.Empty(t, h.NativeCalls)
	require.Empty(t, h.BuiltinLibs)
	require.NotEmpty(t, h.Operators)
}
