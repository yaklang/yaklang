package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestFrontendAlternativeSyntaxBodyBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, source      string
		wantIf, wantLists int
	}{
		{"nested mixed HTML", `<?php if ($outer): ?>outer<?= $a ?><?php
if ($inner): ?>inner<?= $b ?><?php elseif ($other): ?>other<?php
else: ?>fallback<?php endif; echo "after-inner"; else: ?>outer-else<?php
endif; echo "after-outer";`, 2, 5},
		{"loops and switch", `<?php
while ($w): echo "while"; endwhile;
for ($i=0; $i<2; $i++): echo "for"; endfor;
foreach ($rows as $row): echo $row; endforeach;
switch ($v): case 1: echo "one"; break; default: echo "default"; endswitch;
declare(ticks=1): echo "declare"; enddeclare;
echo "after-bodies";`, 0, 6},
		{"keyword members", `<?php if ($condition):
$obj->endif(); Type::endforeach(); $obj->else();
else: $obj->endwhile(); endif; echo "after-members";`, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ast, err := php2ssa.Frontend(tc.source, nil)
			require.NoError(t, err)
			require.NotNil(t, ast)
			ifs, lists := 0, 0
			var walk func(antlr.Tree)
			walk = func(node antlr.Tree) {
				if _, ok := node.(*phpparser.IfStatementContext); ok {
					ifs++
				}
				if body, ok := node.(*phpparser.InnerStatementListContext); ok {
					lists++
					require.NotEmpty(t, body.AllInnerStatement(), "the delimiter must not hide the body")
				}
				for _, child := range node.GetChildren() {
					walk(child)
				}
			}
			walk(ast)
			require.Equal(t, tc.wantIf, ifs)
			require.Equal(t, tc.wantLists, lists)
		})
	}
}

func TestFrontendAlternativeSyntaxRejectsMissingOrMismatchedEnd(t *testing.T) {
	for _, source := range []string{
		`<?php if ($x): echo "body";`,
		`<?php if ($x): echo "body"; endforeach;`,
		`<?php foreach ($xs as $x): echo $x; endif;`,
		`<?php if ($x): if ($y): echo "inner"; endif;`,
	} {
		_, err := php2ssa.Frontend(source, nil)
		require.Error(t, err, "malformed control flow must not become a successful AST: %s", source)
	}
}

func TestAlternativeSyntaxPreservesSSAControlFlow(t *testing.T) {
	// The two syntaxes must produce the same phi and retain the statement after
	// endif. This checks executable SSA values, beyond successful AST parsing.
	var results []string
	for _, source := range []string{
		`<?php $a=0; if ($x): $a=1; elseif ($y): $a=2; else: $a=3; endif; println($a); println("tail");`,
		`<?php $a=0; if ($x) { $a=1; } elseif ($y) { $a=2; } else { $a=3; } println($a); println("tail");`,
	} {
		prog, err := ssaapi.Parse(source, ssaapi.WithLanguage(ssaconfig.PHP))
		require.NoError(t, err)
		result, err := prog.SyntaxFlowWithError(`println(* as $values)`, ssaapi.QueryWithMemory())
		require.NoError(t, err)
		values := result.GetValues("values").String()
		require.Contains(t, values, "phi($a)[1,2,3]")
		require.Contains(t, values, `"tail"`)
		results = append(results, values)
	}
	require.Equal(t, results[1], results[0])
}
