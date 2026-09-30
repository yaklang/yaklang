package php2ssa

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Compare complete typed trees, including operator nesting and all terminals.
// Successful parsing alone would miss a call mistaken for an arrow function or
// an expression swallowed by an alternative-syntax body delimiter.
func predictionTreeShape(tree antlr.Tree) string {
	var out strings.Builder
	var walk func(antlr.Tree)
	walk = func(node antlr.Tree) {
		fmt.Fprintf(&out, "(%T", node)
		if leaf, ok := node.(antlr.TerminalNode); ok {
			fmt.Fprintf(&out, ":%d:%q", leaf.GetSymbol().GetTokenType(), leaf.GetText())
		}
		for _, child := range node.GetChildren() {
			walk(child)
		}
		out.WriteByte(')')
	}
	walk(tree)
	return out.String()
}

func parsePredictionTree(src string, mode int) (ast phpparser.IHtmlDocumentContext, parser *phpparser.PHPParser, err error) {
	if rewritten, ok := rewriteSingleSemicolonNamespaceUseBlock(src); ok {
		src = rewritten
	}
	lexer := phpparser.NewPHPLexer(antlr.NewInputStream(src))
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	stream.SetTokenSource(newHTMLCoalescingTokenSource(lexer))
	parser = phpparser.NewPHPParser(stream)
	ssa.ParserSetAntlrCache(parser, lexer, CreateBuilder().GetAntlrCache())
	listener := antlr4util.NewErrorListener()
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(listener)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(listener)
	parser.GetInterpreter().SetPredictionMode(mode)
	if mode == antlr.PredictionModeSLL {
		parser.SetErrorHandler(antlr4util.NewBailErrorStrategy())
	}
	defer antlr4util.DetachLexerTokenSource(lexer)
	defer func() {
		if recovered := recover(); recovered != nil {
			switch recovered.(type) {
			case *antlr.ParseCancellationException:
				err = fmt.Errorf("SLL cancelled")
			default:
				if message, ok := recovered.(string); ok && message == "implement me" && mode == antlr.PredictionModeSLL {
					err = fmt.Errorf("SLL cancelled")
				} else {
					panic(recovered)
				}
			}
		}
	}()
	ast = parser.HtmlDocument()
	return ast, parser, listener.Error()
}

func adversarialPredictionSources(n int) []struct{ name, source string } {
	chain := "$collection" + strings.Repeat("->sites()->map->handle()", n)
	arrow := "$x"
	for i := 0; i < n; i++ {
		arrow = "fn($x) => match($x) { 1 => " + arrow + ", default => $x ?? 0 }"
	}
	arguments := make([]string, n)
	for i := range arguments {
		arguments[i] = "fn($x) => $x + " + strconv.Itoa(i)
	}
	return []struct{ name, source string }{
		{"arrow in member chain", "<?php return " + chain + "->filter(fn($handle) => User::current()->can('view', Site::get($handle)));"},
		{"nested arrows and match", "<?php $f = " + arrow + "; println('tail');"},
		{"call result assignment", "<?php factory(" + strings.Join(arguments, ",") + ")->values[0] += 1 + 2 * 3; println('tail');"},
		{"nested alternative syntax", "<?php " + strings.Repeat("if ($x): ?>html<?php ", n) + "echo 'body';" + strings.Repeat("else: echo 'fallback'; endif;", n) + "echo 'tail';"},
	}
}

func TestFrontendPredictionAdversarialTrees(t *testing.T) {
	var cases []struct{ name, source string }
	for _, size := range []int{1, 4, 8} {
		for _, tc := range adversarialPredictionSources(size) {
			tc.name += "/" + strconv.Itoa(size)
			cases = append(cases, tc)
		}
	}
	cases = append(cases, []struct{ name, source string }{
		{"keyword members and qualified names", `<?php $obj->fn($x); $obj->match($x); Type::fn($x); Type::match($x); Ns\fn($x); \Ns\match($x); fn\operator($x); \match($x);`},
		{"namespace relative call", `<?php namespace Demo { namespace\helper($x); }`},
		{"grouped imports", `<?php namespace Demo; use A\B\{C, D as E}; use function F\G\{one, two}; function f() { return new E; }`},
		{"closure with capture", `<?php consume(static function (&$x) use (&$captured): int { return $x + $captured; });`},
		{"static call result reference", `<?php Factory::make()->values['key'] = &$value; println('tail');`},
		{"repeated callable result", `<?php factory()()->value = 7; println('tail');`},
		{"nested assignment operands", `<?php factory(($x[0] ?? 0), fn($v) => $v)->value = $a = 1 + 2 * 3;`},
	}...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sllTree, sllParser, err := parsePredictionTree(tc.source, antlr.PredictionModeSLL)
			require.NoError(t, err, "this regression must finish without restarting the file in LL")
			defer antlr4util.DetachParserATNSimulatorCaches(sllParser)
			require.Equal(t, antlr.PredictionModeSLL, sllParser.GetInterpreter().GetPredictionMode(), "local LL must restore its caller's mode")
			llTree, llParser, err := parsePredictionTree(tc.source, antlr.PredictionModeLL)
			require.NoError(t, err)
			defer antlr4util.DetachParserATNSimulatorCaches(llParser)
			require.Equal(t, antlr.PredictionModeLL, llParser.GetInterpreter().GetPredictionMode())
			require.Equal(t, predictionTreeShape(llTree), predictionTreeShape(sllTree))
		})
	}
}

func TestFrontendPredictionRetainsLLRecovery(t *testing.T) {
	// Prefixes beyond the lookahead bound and dynamic field names deliberately
	// keep the original SLL-to-LL path. They must still produce the reference
	// tree rather than being rejected or truncated by a fast-path heuristic.
	for _, source := range []string{
		"<?php factory(" + strings.TrimSuffix(strings.Repeat("0,", 128), ",") + ")->value = 7; println('tail');",
		`<?php factory()->{$field} = 7; println('tail');`,
	} {
		ast, err := Frontend(source, CreateBuilder().GetAntlrCache())
		require.NoError(t, err)
		llTree, parser, err := parsePredictionTree(source, antlr.PredictionModeLL)
		require.NoError(t, err)
		require.Equal(t, predictionTreeShape(llTree), predictionTreeShape(ast))
		antlr4util.DetachParserATNSimulatorCaches(parser)
	}
}

func TestFrontendPredictionMalformedBoundaries(t *testing.T) {
	for _, source := range []string{
		`<?php foo(fn($x) => );`,
		`<?php $x = match($v) { 1 => 2, default => };`,
		`<?php factory()->value = ;`,
		`<?php factory(($x])->value = 1;`,
		`<?php if ($x): echo 'body'; endforeach;`,
		`<?php use A\B\;`,
	} {
		_, err := Frontend(source, CreateBuilder().GetAntlrCache())
		require.Error(t, err, "malformed source must not become a successful AST: %s", source)
	}
	_, parser, err := parsePredictionTree(`<?php factory()->value = ;`, antlr.PredictionModeSLL)
	require.Error(t, err)
	defer antlr4util.DetachParserATNSimulatorCaches(parser)
	require.Equal(t, antlr.PredictionModeSLL, parser.GetInterpreter().GetPredictionMode(), "restore SLL even on cancellation")
}

func BenchmarkFrontendPrediction(b *testing.B) {
	for _, n := range []int{1, 4, 16} {
		for _, tc := range adversarialPredictionSources(n) {
			b.Run(tc.name+"/"+strconv.Itoa(n), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					_, err := Frontend(tc.source, CreateBuilder().GetAntlrCache())
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
	for _, path := range []string{
		"cms/src__Http__Controllers__CP__Collections__EntriesController.php",
		"filament/tests__src__Panels__Commands__MakeRelationManagerCommandTest.php",
		"pfsense/status_dhcp_leases.php",
	} {
		source, err := os.ReadFile(filepath.Join("..", "tests", "syntax", path))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := Frontend(string(source), CreateBuilder().GetAntlrCache())
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
