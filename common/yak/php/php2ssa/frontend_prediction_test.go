package php2ssa

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Compare complete typed trees, including operator nesting, tokens and ranges.
// Successful parsing alone would miss a call mistaken for an arrow function or
// an expression swallowed by an alternative-syntax body delimiter.
func predictionTreeShape(tree antlr.Tree) string {
	var out strings.Builder
	typeNames := make(map[reflect.Type]string)
	var scratch [128]byte
	writeInt := func(value int) {
		out.Write(strconv.AppendInt(scratch[:0], int64(value), 10))
	}
	var walk func(antlr.Tree)
	walk = func(node antlr.Tree) {
		typ := reflect.TypeOf(node)
		name, ok := typeNames[typ]
		if !ok {
			name = typ.String() // identical to fmt's %T
			typeNames[typ] = name
		}
		out.WriteByte('(')
		out.WriteString(name)
		if leaf, ok := node.(antlr.TerminalNode); ok {
			token := leaf.GetSymbol()
			out.WriteByte(':')
			writeInt(token.GetTokenType())
			out.WriteByte(':')
			out.Write(strconv.AppendQuote(scratch[:0], leaf.GetText()))
			for _, value := range [...]int{token.GetLine(), token.GetColumn(), token.GetStart(), token.GetStop()} {
				out.WriteByte(':')
				writeInt(value)
			}
		}
		if ctx, ok := node.(antlr.ParserRuleContext); ok && ctx.GetStart() != nil && ctx.GetStop() != nil {
			out.WriteByte(':')
			writeInt(ctx.GetStart().GetTokenIndex())
			out.WriteByte(':')
			writeInt(ctx.GetStop().GetTokenIndex())
		}
		for _, child := range node.GetChildren() {
			walk(child)
		}
		out.WriteByte(')')
	}
	walk(tree)
	return out.String()
}

func parsePredictionTree(src string, mode int, fastPrediction ...bool) (ast phpparser.IHtmlDocumentContext, parser *phpparser.PHPParser, err error) {
	return parsePredictionTreeWithCache(src, mode, CreateBuilder().GetAntlrCache(), fastPrediction...)
}

func parsePredictionTreeWithCache(src string, mode int, cache *ssa.AntlrCache, fastPrediction ...bool) (ast phpparser.IHtmlDocumentContext, parser *phpparser.PHPParser, err error) {
	if rewritten, ok := rewriteSingleSemicolonNamespaceUseBlock(src); ok {
		src = rewritten
	}
	lexer := phpparser.NewPHPLexer(antlr.NewInputStream(src))
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	stream.SetTokenSource(newHTMLCoalescingTokenSource(lexer))
	parser = phpparser.NewPHPParser(stream)
	if len(fastPrediction) > 0 {
		parser.SetFastPrediction(fastPrediction[0])
	}
	ssa.ParserSetAntlrCache(parser, lexer, cache)
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
			llTree, llParser, err := parsePredictionTree(tc.source, antlr.PredictionModeLL, false)
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
		llTree, parser, err := parsePredictionTree(source, antlr.PredictionModeLL, false)
		require.NoError(t, err)
		require.Equal(t, predictionTreeShape(llTree), predictionTreeShape(ast))
		antlr4util.DetachParserATNSimulatorCaches(parser)
	}
}

// These small sources reproduce expensive decisions from DHCP, CMS and Grav:
// a body delimiter followed by complex statements, variable/member chains,
// and optional call/index suffixes. Compare with the unmodified ATN predictor,
// rather than two prediction modes that both use the same fast classifier.
func TestFrontendFastPredictionMinimalReproductions(t *testing.T) {
	cases := []struct{ name, source string }{
		{"variable arithmetic and literals", `<?php $a = 1 + 2 * 3; $b = $a ?? 4; println($b, 0x10, 0b11, 1.5, 'tail');`},
		{"parentheses casts and callable results", `<?php $a = ($x + 1) * ($y ?? 0); $b = (int)$x; $c = (float)($y + 1); $d = ($object->get())($arg); println($a, $b, $c);`},
		{"builtin calls and array argument", `<?php $a = isset($values[0]) && !empty($values[1]); define('key', factory(['one' => 1, 'two' => 2])); eval($code); exit($a);`},
		{"assignment reference and increments", `<?php $a[0] = &$b; $a[0] += 1; ++$a[0]; $a[0]++; $object->items()[0]->value = 7;`},
		{"member call suffixes and trailing comma", `<?php $x = $this->query($a)->filters()->map->title(); consume(['one' => $object->get(), 'two' => $object->items()->all(),]);`},
		{"dynamic and static receivers", `<?php $object->{$field}[0] = 1; $object->getClass()::method($x); Type::method($x); factory()()->item = 7;`},
		{"dynamic constructor types", `<?php $a = new $this->className(); $b = new $this->typemap[$type](); $c = new $$class($arg); println('tail');`},
		{"call result indexes reserve their suffix", `<?php app('scopes')[PostType::handle()] = PostType::class; factory()[0][1] += 2; println('tail');`},
		{"keyword receivers and members", `<?php if\operator($x); endif\helper($x); Type::else($x); $object->endif($x); $object->match($x);`},
		{"throw and unset priority", `<?php throw Error::create(); unset(static::$cookie->{$name}); unset($array[0]);`},
		{"colon switch and standard dangling else", `<?php switch ($x): case 1: $a = 2; break; default: $a = 3; endswitch; if ($x) if ($y) $a = 1; else $a = 2;`},
		{"closure and new expression", `<?php consume(static function ($x) use (&$captured) { return (new Item($x))->get($captured); }, fn($v) => $v + 1);`},
	}
	closure := `function ($value) { ` + strings.Repeat(`$value += 1; `, 32) + `return $value; }`
	cases = append(cases, []struct{ name, source string }{
		{"large closure member calls", `<?php $this->action(` + closure + `); Type::make(` + closure + `)->items()[0]->get(); println('tail');`},
		{"large closure dynamic static access", `<?php $type = factory(` + closure + `)->get()::name($x); println('tail');`},
		{"large closure assignment suffix", `<?php Type::make(` + closure + `)->items()[0]->value += 1; println('tail');`},
		{"constructor arguments containing member operators", `<?php if (!validate($option = new Option($demand[1]) || $option->id != $demand[0])) { println('tail'); }`},
		{"spread and reference array items", `<?php $items = [...(ready() ? [Type::make($x)->get()] : []), ...$values, 'ref' => &$reference, $x + 1 => &$other,]; println($items);`},
		{"interpolation and hidden comment delimiters", `<?php $this /* }]) */ ->action(function ($value) { /* ([{ */ return "{$value->name} {$value['key']}"; })->items()[0]; println('tail');`},
	}...)
	for _, size := range []int{1, 4, 16} {
		body := `foreach (config_get_path('items', []) as $entry) { if ($entry['ip']) { echo $entry['ip']; } else { $count += 1; } }`
		cases = append(cases, struct{ name, source string }{
			"delimiter and following loops/" + strconv.Itoa(size),
			`<?php if ($ready): ?>body<?php $count = 1; endif; ` + strings.Repeat(body, size) + `println('tail');`,
		})
		cases = append(cases, struct{ name, source string }{
			"member chain/" + strconv.Itoa(size),
			`<?php $value = $object` + strings.Repeat(`->items($offset)[0]->get()`, size) + `; println($value, 'tail');`,
		})
		var methods strings.Builder
		for i := 0; i < size; i++ {
			fmt.Fprintf(&methods, "public function method%d($value) { return $this->items($value)[0]->get(); }\n", i)
		}
		cases = append(cases, struct{ name, source string }{
			"declaration after statements/" + strconv.Itoa(size),
			`<?php require_once 'local.php'; class Payload { ` + methods.String() + ` } println('tail');`,
		})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ast, err := Frontend(tc.source, CreateBuilder().GetAntlrCache())
			require.NoError(t, err)
			reference, parser, err := parsePredictionTree(tc.source, antlr.PredictionModeLL, false)
			require.NoError(t, err)
			defer antlr4util.DetachParserATNSimulatorCaches(parser)
			require.Equal(t, predictionTreeShape(reference), predictionTreeShape(ast))
		})
	}
}

// Batch the operator matrix in one source so the reference parser exercises
// every pair without repeatedly initializing its ATN and DFA for each sample.
func TestFrontendFastPredictionOperatorBindingTrees(t *testing.T) {
	operators := []string{
		"**", "instanceof", "*", "/", "%", "+", "-", ".", "<<", ">>",
		"<", "<=", ">", ">=", "===", "!==", "==", "!=", "&", "^", "|",
		"&&", "||", "??", "<=>", "and", "xor", "or",
	}
	var source strings.Builder
	source.WriteString("<?php\n")
	for _, left := range operators {
		for _, right := range operators {
			fmt.Fprintf(&source, "$value = $a %s $b %s $c;\n", left, right)
		}
	}
	source.WriteString(`
$a = -$x ** 2 + 3;
$b = $a ? ($b ? $c : $d) : $e ?? $f;
$c = $a ? $b : $c ? $d : $e;
$d = ($object->method)($argument)->items()[0] + 1;
$e = (new Item(fn($x) => $x))->get()?->items()[0] ?? 0;
$f = ($object)->{'name'}($argument)[0];
$g = $object->{};
println('tail');`)
	ast, parser, err := parsePredictionTree(source.String(), antlr.PredictionModeSLL)
	require.NoError(t, err, "known operator bindings must finish without restarting in LL")
	defer antlr4util.DetachParserATNSimulatorCaches(parser)
	reference, original, err := parsePredictionTree(source.String(), antlr.PredictionModeLL, false)
	require.NoError(t, err)
	defer antlr4util.DetachParserATNSimulatorCaches(original)
	require.Equal(t, predictionTreeShape(reference), predictionTreeShape(ast))
}

func TestFrontendFastPredictionArrayAndForeachBoundaries(t *testing.T) {
	cases := []struct{ name, source string }{
		{"positional and keyed keyword arrays", `<?php $a = array(1, 2); $b = array('key' => 1); $c = array(array('key' => 1)); $d = array(foo: 1);`},
		{"lambda arrows are not array keys", `<?php $a = array(fn($x) => $x); $b = array($key => fn($x) => $x); $c = array((fn($x) => $x));`},
		{"spread and reference arguments", `<?php $a = array(&$value, ...$values); $b = array(($ready ? 1 : 2), ...$values); $c = [fn($x) => &$value, ...fn($x) => $x];`},
		{"keyed reference array elements", `<?php $a = [($key) => &$value, [1] => &$other, $key + 1 => &$third, &$value];`},
		{"conditional and closure array elements", `<?php $a = [($ready ? [Type::make(fn($x) => $x)->get()] : [])]; $b = [Type::make(function ($x) { return $x + 1; })->get()];`},
		{"array indexing and destructuring", `<?php $a = [1, 2][0]; $b = ['key' => 1]{0}; [$first, $second] = $values;`},
		{"list calls and assignment holes", `<?php list($a, , $b) = $values; $x = list($a, $b); $y = list($key => 1);`},
		{"constant arrays and expression suffixes", `<?php class A { const X = [1, 2]; const Y = array('key' => 1); public $z = [...$values]; } const B = [1, 2][0]; const C = array(1) + 2;`},
		{"constant named arguments and lambda return types", `<?php class A { const X = array(foo: 1); public $y = array(fn($x): int => $x); }`},
		// Keep the language accepted by the original grammar, even where its
		// placeholder/spread forms differ from executable PHP.
		{"constant placeholder and repeated leading spread", `<?php class A { const X = array(...); public $y = [...]; public $z = [... ...$values]; }`},
		{"chain foreach source", `<?php foreach ($values as $value) { println($value); } foreach ($values->items() as &$value): println($value); endforeach;`},
		{"call foreach source and keyed references", `<?php foreach (config_get('list', []) as $key => &$value) { println($value); }`},
		{"array and grouped foreach sources", `<?php foreach ([1, 2] as $key => $value) { println($value); } foreach (($values ?? []) as $value) { println($value); }`},
		{"foreach destructuring and list holes", `<?php foreach ($values as [$a, $b]) { println($a, $b); } foreach ($values as list($a, , $b)) { println($a, $b); }`},
		{"keyed foreach list and indexed binding", `<?php foreach ($values as $key => list($a, $b)) { println($a, $b); } foreach ($values as $value[0]) { println($value); }`},
		{"variable callbacks and repeated callable results", `<?php $value = $callback(fn($x) => $x + 1)()->items()[0]; $other = $callback($argument)?->value;`},
		{"variable callback static and assignment suffixes", `<?php $type = $callback($argument)->get()::name($value); $callback($argument)[0] = &$reference;`},
		{"variable callback closure and nested array", `<?php $value = $callback('mapping', [Type::make(function ($x) { return $x + 1; })->get(), fn($v) => $v]);`},
		{"grouped assignable suffix and dynamic constructor", `<?php ($object)->items[0] = 1; $a = new ($type->name)($argument); $b = new factory($argument)->name();`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ast, err := Frontend(tc.source+` println('tail');`, CreateBuilder().GetAntlrCache())
			require.NoError(t, err)
			reference, parser, err := parsePredictionTree(tc.source+` println('tail');`, antlr.PredictionModeLL, false)
			require.NoError(t, err)
			defer antlr4util.DetachParserATNSimulatorCaches(parser)
			require.Equal(t, predictionTreeShape(reference), predictionTreeShape(ast))
		})
	}
}

func TestFrontendFastPredictionMalformedPrefixes(t *testing.T) {
	for _, source := range []string{
		`<?php if ($x): $a = 1; }`,
		`<?php if ($x) { $a = 1; endif;`,
		`<?php $object->items(]->value = 1;`,
		`<?php $a[0) = 1;`,
		`<?php consume(['one' => 1,,]);`,
		`<?php $object->items()->value += ;`,
		`<?php $value = new $object->className(;`,
		`<?php if ($x) { echo 1; } else { echo 2;`,
		`<?php $value = $left * ;`,
		`<?php $value = $ready ? ;`,
		`<?php foreach ($values as => $value) { echo 1; }`,
		`<?php foreach ($values as $key => &) { echo 1; }`,
		`<?php const X = [1, 2][;`,
		`<?php list($a, $b) = ;`,
	} {
		_, err := Frontend(source, CreateBuilder().GetAntlrCache())
		require.Error(t, err, "fast prediction must preserve malformed-input rejection: %s", source)
		_, parser, err := parsePredictionTree(source, antlr.PredictionModeLL, false)
		require.Error(t, err)
		antlr4util.DetachParserATNSimulatorCaches(parser)
	}
	_, parser, err := parsePredictionTree(`<?php $value = new $object->className(;`, antlr.PredictionModeSLL)
	require.Error(t, err)
	defer antlr4util.DetachParserATNSimulatorCaches(parser)
	require.Equal(t, antlr.PredictionModeSLL, parser.GetInterpreter().GetPredictionMode(), "restore SLL when dynamic type parsing is cancelled")
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
	for _, tc := range []struct{ directory, path string }{
		{"syntax", "cms/src__Http__Controllers__CP__Collections__EntriesController.php"},
		{"syntax", "cms/src__Fieldtypes__Entries.php"},
		{"syntax", "filament/tests__src__Panels__Commands__MakeRelationManagerCommandTest.php"},
		{"syntax", "pfsense/status_dhcp_leases.php"},
		{"syntax", "grav_slow/system__src__Grav__Framework__Flex__FlexCollection.php"},
		{"large", "qloapps/tools__tcpdf__tcpdf.php"},
		{"large", "filament/packages__actions__src__Concerns__CanExportRecords.php"},
		{"large", "filament/packages__forms__src__Components__Select.php"},
		{"large", "filament/packages__infolists__src__Components__TextEntry.php"},
		{"large", "cms/tests__Antlers__Runtime__TemplateTest.php"},
		{"large", "prestashop/classes__controller__AdminController.php"},
		{"large", "prestashop/tests__Integration__Behaviour__Features__Context__Domain__OrderFeatureContext.php"},
	} {
		source, err := os.ReadFile(filepath.Join("..", "tests", tc.directory, tc.path))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(tc.path, func(b *testing.B) {
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
