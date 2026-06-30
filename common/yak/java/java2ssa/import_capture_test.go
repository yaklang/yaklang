package java2ssa

import (
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestNewJavaImportDeclCapturesLightweightToken(t *testing.T) {
	source := `package demo;
import static java.util.Collections.*;
import java.util.List;
class A {}`
	raw, err := Frontend(source, nil)
	require.NoError(t, err)
	cu := raw.(*javaparser.CompilationUnitContext)
	imports := cu.AllImportDeclaration()
	require.Len(t, imports, 2)

	staticAll, ok := newJavaImportDecl(imports[0])
	require.True(t, ok)
	require.Equal(t, []string{"java", "util", "Collections", "*"}, staticAll.pkgNames)
	require.True(t, staticAll.static)
	require.True(t, staticAll.all)
	require.IsType(t, &ssa.TextRangeToken{}, staticAll.token)
	_, isTree := staticAll.token.(antlr.Tree)
	require.False(t, isTree, "captured import token must not retain the import parser context")
	require.Nil(t, staticAll.token.GetStart().GetInputStream())
	require.Nil(t, staticAll.token.GetStart().GetTokenSource())

	normal, ok := newJavaImportDecl(imports[1])
	require.True(t, ok)
	require.Equal(t, []string{"java", "util", "List"}, normal.pkgNames)
	require.False(t, normal.static)
	require.False(t, normal.all)

	ssa.ReleaseASTRoot(cu)

	rng := ssa.GetRange(memedit.NewMemEditor(source), staticAll.token)
	require.NotNil(t, rng)
	require.Equal(t, "import static java.util.Collections.*;", rng.GetText())
}
