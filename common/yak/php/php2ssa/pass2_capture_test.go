package php2ssa

import (
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/stretchr/testify/require"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestCollectPHPFilePass2Capture_skipsFunctionAndClassBodies(t *testing.T) {
	src := `<?php
namespace Test;

use Some\Thing;

const TOP = 1;

function helper() {
    return 1;
}

class Example {
    public function run() {
        echo 1;
    }
}
`
	ast, err := Frontend(src, nil)
	require.NoError(t, err)

	capture := collectPHPFilePass2Capture(ast.(phpparser.IHtmlDocumentContext))
	require.NotNil(t, capture)
	require.False(t, capture.empty())

	allCaptured := append([]antlr.Tree{}, capture.uses...)
	allCaptured = append(allCaptured, capture.globals...)
	allCaptured = append(allCaptured, capture.statements...)
	allCaptured = append(allCaptured, capture.enums...)
	for _, ns := range capture.namespaces {
		allCaptured = append(allCaptured, ns.uses...)
		allCaptured = append(allCaptured, ns.globals...)
		allCaptured = append(allCaptured, ns.statements...)
		allCaptured = append(allCaptured, ns.enums...)
	}
	for _, tree := range allCaptured {
		require.NotNil(t, tree)
		assertNoFunctionOrClassAST(t, tree)
	}
	require.NotEmpty(t, capture.namespaces)
	require.NotEmpty(t, capture.namespaces[0].uses)
	// Named namespaces must not retain class/function subtrees for pass2.
	if capture.namespaces[0].name != "" {
		require.Empty(t, capture.namespaces[0].classes)
		require.Empty(t, capture.namespaces[0].functions)
	}
}

func assertNoFunctionOrClassAST(t *testing.T, tree antlr.Tree) {
	t.Helper()
	switch tree.(type) {
	case phpparser.IFunctionDeclarationContext, phpparser.IClassDeclarationContext:
		t.Fatalf("pass2 capture must not retain function/class declarations: %T", tree)
	}
	rule, ok := tree.(antlr.ParserRuleContext)
	if !ok {
		return
	}
	for _, child := range rule.GetChildren() {
		if c, ok := child.(antlr.Tree); ok {
			assertNoFunctionOrClassAST(t, c)
		}
	}
}

func TestCollectPHPFilePass2Capture_functionOnlyFileReturnsNil(t *testing.T) {
	src := `<?php
function only_helper() {
    return 1;
}
`
	ast, err := Frontend(src, nil)
	require.NoError(t, err)

	capture := collectPHPFilePass2Capture(ast.(phpparser.IHtmlDocumentContext))
	require.True(t, capture == nil || capture.empty())
}

func TestVisitPHPFilePass2Capture_doesNotPanicOnTopLevelStatement(t *testing.T) {
	src := `<?php
$top = 1;
`
	ast, err := Frontend(src, nil)
	require.NoError(t, err)

	capture := collectPHPFilePass2Capture(ast.(phpparser.IHtmlDocumentContext))
	require.NotNil(t, capture)
	require.Len(t, capture.statements, 1)

	prog := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "test", 0)
	builder := prog.GetAndCreateFunctionBuilder("main", "main")
	editor := prog.CreateEditor([]byte(src), "test.php")
	builder.SetEditor(editor)

	require.NotPanics(t, func() {
		visitPHPFilePass2Capture(builder, builder, capture)
	})
}
