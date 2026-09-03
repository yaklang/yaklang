package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
)

func collectTypeNames(unit csharpparser.ICompilation_unitContext) []string {
	var names []string
	var walkMember func(csharpparser.INamespace_member_declarationContext)
	var walkType func(csharpparser.IType_declarationContext)
	walkType = func(td csharpparser.IType_declarationContext) {
		if td == nil {
			return
		}
		if c := td.Class_declaration(); c != nil && c.Identifier() != nil {
			names = append(names, c.Identifier().GetText())
		}
		if s := td.Struct_declaration(); s != nil && s.Identifier() != nil {
			names = append(names, s.Identifier().GetText())
		}
		if i := td.Interface_declaration(); i != nil && i.Identifier() != nil {
			names = append(names, i.Identifier().GetText())
		}
		if e := td.Enum_declaration(); e != nil && e.Identifier() != nil {
			names = append(names, e.Identifier().GetText())
		}
		if d := td.Delegate_declaration(); d != nil {
			if header := d.Delegate_header(); header != nil && header.Identifier() != nil {
				names = append(names, header.Identifier().GetText())
			}
		}
	}
	walkMember = func(m csharpparser.INamespace_member_declarationContext) {
		if m == nil {
			return
		}
		if ns := m.Namespace_declaration(); ns != nil {
			if body := ns.Namespace_body(); body != nil {
				for _, inner := range body.AllNamespace_member_declaration() {
					walkMember(inner)
				}
			}
		}
		walkType(m.Type_declaration())
	}
	for _, m := range unit.AllNamespace_member_declaration() {
		walkMember(m)
	}
	return names
}

func TestCSharpFrontendNamedConstructs(t *testing.T) {
	src := mustReadCodeFixture(t, "code/edge_types.cs")
	start := time.Now()
	ast, err := csharp2ssa.Frontend(src)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.LessOrEqual(t, time.Since(start), savedCSharpFixtureMaxParseDuration)

	names := collectTypeNames(ast)
	require.Contains(t, names, "NestedHost", "shipped Frontend AST must contain class NestedHost")
	require.Contains(t, names, "Color", "shipped Frontend AST must contain enum Color")
	require.Contains(t, names, "IRepo", "shipped Frontend AST must contain interface IRepo")
	require.Contains(t, names, "Pair", "shipped Frontend AST must contain struct Pair")
	require.Contains(t, names, "Mapper", "shipped Frontend AST must contain delegate Mapper")
}

func TestCSharpFrontendPreprocessorFalseBranchSkipped(t *testing.T) {
	src := mustReadCodeFixture(t, "code/edge_preproc.cs")
	start := time.Now()
	ast, err := csharp2ssa.Frontend(src)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.LessOrEqual(t, time.Since(start), savedCSharpFixtureMaxParseDuration)

	text := ast.GetText()
	require.Contains(t, text, "keptA", "#if true branch must survive in AST")
	require.Contains(t, text, "liveInner", "#else of #if false must survive in AST")
	require.NotContains(t, text, "skippedB", "false #if branch must be skipped")
	require.NotContains(t, text, "skippedElse", "false #else branch must be skipped")
	require.NotContains(t, text, "deadInner", "false nested #if branch must be skipped")
	require.True(t, strings.Contains(text, "PreprocElif"))
}

func TestCSharpFrontendMethodPresent(t *testing.T) {
	src := `
public class EdgeClass {
    public int EdgeMethod() { return 1; }
}
`
	ast, err := csharp2ssa.Frontend(src)
	require.NoError(t, err)
	require.NotNil(t, ast)
	names := collectTypeNames(ast)
	require.Contains(t, names, "EdgeClass")
	require.Contains(t, ast.GetText(), "EdgeMethod")
}
