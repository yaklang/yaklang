package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfpattern"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

// TestSimpleValueToSSAValue_FullFileAnchoring verifies that source-mode
// pattern hits are anchored inside the FULL file editor, so risks carry the
// real file path, line numbers and context — same as SSA-mode risks.
func TestSimpleValueToSSAValue_FullFileAnchoring(t *testing.T) {
	content := "package main\n\nconst key = \"AKIAIOSFODNN7EXAMPLE\"\n"
	files := map[string]string{"src/main.go": content}

	vals, err := sfpattern.MatchRegexp(files, "src/main.go", []string{`AKIA[0-9A-Z]{16}`})
	require.NoError(t, err)
	require.Len(t, vals, 1)

	ssaVals := FromSFVMValues(vals)
	require.Len(t, ssaVals, 1)
	v := ssaVals[0]

	r := v.GetRange()
	require.NotNil(t, r, "hit value should carry a range")
	ed := r.GetEditor()
	require.NotNil(t, ed)
	// the editor must hold the WHOLE file, not just the matched snippet
	require.Equal(t, content, ed.GetSourceCode())
	// the match is on line 3 of the file
	require.Equal(t, 3, r.GetStart().GetLine())
	require.Equal(t, 3, r.GetEnd().GetLine())
	// the value text is exactly the matched fragment (const values render quoted)
	require.Contains(t, v.String(), "AKIAIOSFODNN7EXAMPLE")
	// range text equals the fragment inside the full file
	text, err := ed.GetTextFromRangeWithError(r)
	require.NoError(t, err)
	require.Equal(t, "AKIAIOSFODNN7EXAMPLE", text)
}

// TestSimpleValueToSSAValue_FallbackSnippetEditor keeps the legacy fallback:
// without a full-file editor the range covers the snippet only.
func TestSimpleValueToSSAValue_FallbackSnippetEditor(t *testing.T) {
	sv := sfvm.NewSimpleValue("AKIAIOSFODNN7EXAMPLE", "src/main.go", 0, 20)
	v := simpleValueToSSAValue(sv)
	r := v.GetRange()
	require.NotNil(t, r)
	require.NotNil(t, r.GetEditor())
	require.Equal(t, "AKIAIOSFODNN7EXAMPLE", r.GetEditor().GetSourceCode())
	require.Equal(t, 1, r.GetStart().GetLine())
}

// TestPatternHitEditorProgramName verifies PatternRoot propagates the program
// name into hit editors so ir-source hashes align with compiled programs.
func TestPatternHitEditorProgramName(t *testing.T) {
	files := map[string]string{"src/a.env": "TOKEN=AKIAIOSFODNN7EXAMPLE\n"}
	root := sfpattern.NewRoot(files)
	root.SetProgramName("demo-proj")

	vals, err := root.FileFilter("a.env", "regexp", nil, []string{`AKIA[0-9A-Z]{16}`})
	require.NoError(t, err)
	require.Len(t, vals, 1)

	sv, ok := vals[0].(*sfvm.SimpleValue)
	require.True(t, ok)
	ed := sv.FileEditor()
	require.NotNil(t, ed)
	require.Equal(t, "demo-proj", ed.GetProgramName())
	// URL follows the /programName/path convention
	require.Equal(t, "/demo-proj/src/a.env", ed.GetUrl())
	// ir-source hash is non-empty (program + path + content)
	require.NotEmpty(t, ed.GetIrSourceHash())
}
