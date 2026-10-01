package php2ssa

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

// Generated from the original LL predictor with SetFastPrediction(false).
// Hash all typed nodes, tokens and ranges, rather than accepting any successful
// parse. YAK_PHP_PREDICTION_ORACLE=1 repeats the full independent LL / fast LL /
// production comparison and prints new reference hashes after grammar changes.
// Normal CI runs the same complete matrices without rerunning the slow oracle.
var predictionAmbiguityReferenceHashes = map[string]string{
	"TestFrontendPredictionClassConstructorLegacyIndex":                    "3d5fce310d974741c831a82b2930b9b4270eedb87062fb3faf004c7ea900276c",
	"TestFrontendPredictionClassMemberKeywordBoundaries":                   "fc350319dcaaabe9c3b5d62e6be1e4ca0bccac114698938ca1666c99a6ea3845",
	"TestFrontendPredictionAnonymousClassNewExprBoundaries":                "7939417abb854403de6bcedbac1471cf47626c30cfde3e2209773ada92f3c0a1",
	"TestFrontendPredictionPrimaryOperatorCrossProduct/binding":            "3b7bb7b8bbaeff9c48d414b9163c307d8e4b9a73351b6886b07a72d5f562c34e",
	"TestFrontendPredictionPrimaryOperatorCrossProduct/recursive_suffixes": "83c1892e2f6d9d5078dfba0bbeb67d76172c9ff4c2527f6738035ea2536b5e74",
	"TestFrontendPredictionArrayItemCrossProduct":                          "f025b4f22491190266d4b8afb28d3a5afd8008743b98368044d9d8bc17c0106b",
	"TestFrontendPredictionAssignmentSuffixCrossProduct":                   "42e89f77e17301008d93d0b2befbb116924838e640e46d347a21b2a09b079da9",
	"TestFrontendPredictionCastCallableCrossProduct":                       "e2761286d4e400332e342076721b26b04b4dc79aa4b43d669c6fd6a13a87ffbf",
	"TestFrontendPredictionForeachTypeRefCrossProduct":                     "d63a9026f534e190bd5dfe8ba0a2b58666aaefba2b8006f24e391d05b31239eb",
	"TestFrontendPredictionStaticVariableSuffixes":                         "9b834a5eb792ce5bfc6889c829d3c6ac2686db870192ef6716bb7b3736fb3f61",
	"TestFrontendPredictionConstantInitializerBoundaries":                  "4b7310392571a8c0445064b1fb0a45f35ad183c5cf3b5aabe701566271ae570f",
	"TestFrontendPredictionParenthesizedStaticReceivers":                   "af10eff28e15b88311c084a222eaf86e9997c54eee926ba8723a381754ed5817",
}

// Each batch shares an ATN/DFA initialization. Bound the source size so neither
// production LL recovery nor diagnostic full-context exploration traverses a
// long file made from independent samples.
func assertPredictionAmbiguityBatch(t *testing.T, statements []string) {
	t.Helper()
	expected, ok := predictionAmbiguityReferenceHashes[t.Name()]
	require.True(t, ok, "matrix must have an independently generated reference hash")
	oracle := os.Getenv("YAK_PHP_PREDICTION_ORACLE") == "1"
	if !oracle {
		require.NotEmpty(t, expected, "generate the matrix hash with the independent oracle first")
	}
	digest := sha256.New()
	// Exercise worker-cache reuse as well as the cold-cache oracle. Each
	// matrix owns its cache; parsing every batch need not deserialize its ATN.
	cache := CreateBuilder().GetAntlrCache()
	defer cache.Clear()
	var batches [][]string
	const batchSize = 16
	for start := 0; start < len(statements); start += batchSize {
		end := start + batchSize
		if end > len(statements) {
			end = len(statements)
		}
		batch := statements[start:end]
		batches = append(batches, batch)
		var shape string
		if oracle {
			shape = assertPredictionAmbiguitySource(t, batch)
		} else {
			source := predictionAmbiguitySource(batch)
			ast, err := Frontend(source, cache)
			require.NoError(t, err, "production parser rejected matrix batch %d", len(batches))
			shape = predictionTreeShape(ast)
		}
		fmt.Fprint(digest, shape)
	}
	actual := fmt.Sprintf("%x", digest.Sum(nil))
	if oracle {
		t.Logf("independent LL matrix hash: %s", actual)
	}
	if expected != "" && expected != actual {
		t.Errorf("complete matrix typed-tree hash differs: original LL %s, actual %s", expected, actual)
		// Rebuild the reference only on failure, and report the first differing
		// node/token region together with the small reproducing source batch.
		if !oracle {
			for _, batch := range batches {
				assertPredictionAmbiguitySource(t, batch)
			}
		}
	}
}

func predictionAmbiguitySource(statements []string) string {
	return "<?php\n" + strings.Join(statements, "\n") + "\nprintln('matrix tail');"
}

func assertPredictionAmbiguitySource(t *testing.T, statements []string) string {
	t.Helper()
	source := predictionAmbiguitySource(statements)
	reference, original, err := parsePredictionTree(source, antlr.PredictionModeLL, false)
	require.NoError(t, err, "reference grammar rejected batch")
	defer antlr4util.DetachParserATNSimulatorCaches(original)
	expected := predictionTreeShape(reference)
	ast, err := Frontend(source, CreateBuilder().GetAntlrCache())
	require.NoError(t, err, "production parser rejected reference input")
	assertPredictionShapeEqual(t, expected, predictionTreeShape(ast), "production", source)
	fastLL, parser, err := parsePredictionTree(source, antlr.PredictionModeLL)
	require.NoError(t, err, "fast LL rejected reference input")
	defer antlr4util.DetachParserATNSimulatorCaches(parser)
	require.Equal(t, antlr.PredictionModeLL, parser.GetInterpreter().GetPredictionMode())
	assertPredictionShapeEqual(t, expected, predictionTreeShape(fastLL), "fast LL", source)
	return expected
}

func assertPredictionShapeEqual(t *testing.T, expected, actual, mode, source string) {
	t.Helper()
	if expected == actual {
		return
	}
	common := 0
	for common < len(expected) && common < len(actual) && expected[common] == actual[common] {
		common++
	}
	start := common - 80
	if start < 0 {
		start = 0
	}
	endExpected, endActual := common+220, common+220
	if endExpected > len(expected) {
		endExpected = len(expected)
	}
	if endActual > len(actual) {
		endActual = len(actual)
	}
	t.Errorf("%s typed-tree mismatch at byte %d\nreference: %s\nactual: %s\nsource:\n%s", mode, common, expected[start:endExpected], actual[start:endActual], source)
}

func TestFrontendPredictionPrimaryOperatorCrossProduct(t *testing.T) {
	// Include prefixes for all 36 primary grammar arms. Some overlap with an
	// earlier arm (e.g. Label/identifier and define/functionCall); retaining the
	// original winner is part of the contract, rather than forcing each arm.
	primaries := []string{
		`clone $object`, `new Item($argument)`, `function ($x) use (&$captured) { return $x; }`,
		`fn($x) => $x`, `match($x) { 1 => 2, default => 3 }`, `factory($argument)`,
		`Ns\Value`, `parent::value`, `$object->getClass()::value`, `Type::value`,
		`VALUE`, `\VALUE`, `$value`, `&$reference`, `[1, 'key' => 2]`,
		`12`, `0x12`, `0b11`, `1.5`, `'string'`, `"{$value}"`,
		`define('KEY', $value)`, `defined('KEY')`, `print $value`, "`text`",
		`($a + $b)`, `include 'local.php'`, `set_include_path $value`, `yield`,
		`list($first, , $last) = $values`, `throw $exception`, `(int)$value`,
		`~$value`, `@$value`, `!$value`, `+$value`, `-$value`,
		`++$value`, `$value--`, `[$first, $last] = $values`,
		`factory()->item = &$reference`, `factory()->item += 2`,
		`$value = &$reference`, `$value += 2`,
	}
	operators := []string{"**", "instanceof", "*", "+", "<<", "<", "===", "&", "^", "|", "&&", "||", "??", "<=>", "and", "xor", "or"}
	var binding, suffixes []string
	for _, primary := range primaries {
		for _, operator := range operators {
			binding = append(binding, fmt.Sprintf("consume(%s %s $right);", primary, operator))
		}
		binding = append(binding, fmt.Sprintf("consume(%s ? $yes : $no);", primary))
		binding = append(binding, fmt.Sprintf("consume(%s ?: $fallback);", primary))
		// The five recursive suffix arms cross every primary, with a grouped
		// receiver so executable-PHP restrictions do not change this grammar.
		for _, suffix := range []string{"->item", "?->item", "[0]", "{0}", "($argument)", "->{}"} {
			suffixes = append(suffixes, fmt.Sprintf("consume((%s)%s + $right);", primary, suffix))
		}
	}
	t.Run("binding", func(t *testing.T) { assertPredictionAmbiguityBatch(t, binding) })
	t.Run("recursive suffixes", func(t *testing.T) { assertPredictionAmbiguityBatch(t, suffixes) })
}

func TestFrontendPredictionArrayItemCrossProduct(t *testing.T) {
	items := []string{
		`$value`, `($a ? $b : $c)`, `[1, 2]`, `array(1, 2)`, `array('key' => 2)`,
		`factory(fn($x) => $x)->items()[0]`, `$callback(function ($x) { return $x; })`,
		`fn($x): int => $x`, `static fn($x) => $x`, `function &($x) use (&$value) { return $value; }`,
		`($a + $b) * $c`, `'text'`, `"{$value['key']}"`, `($value = 1)`,
	}
	keys := []string{`'key'`, `$key`, `($key)`, `[1]`, `$key + 1`, `factory()->key`, `fn($x) => $x`}
	var statements []string
	for _, item := range items {
		for _, wrapper := range []string{"[%s]", "array(%s)", "list(%s)"} {
			for _, elements := range []string{item, "..." + item, item + ",", item + ", &$reference"} {
				statements = append(statements, fmt.Sprintf("$array = "+wrapper+";", elements))
			}
		}
		for _, key := range keys {
			statements = append(statements, fmt.Sprintf("$array = [%s => %s, %s => &$reference];", key, item, key))
		}
	}
	// Top-level =>/fn/: must be distinguished from the same token inside a
	// nested closure, match, array, named argument or interpolation.
	statements = append(statements,
		`$array = array(factory([fn($x) => $x]), ['nested' => [1]], 'key' => 1);`,
		`$array = array(foo: fn($x): int => $x);`,
		`$array = list(foo: 1);`,
		`$array = [fn($x) => &$reference, ...fn($x) => $x, ($ready ? $a : $b) => &$reference];`,
		`$array = [... ...$values];`,
		`list($a, , list($b, $c), $key => &$reference) = $values;`,
		`[,$a,,$b,] = $values;`,
		`['first' => $a, 'second' => &$b] = $values;`,
	)
	assertPredictionAmbiguityBatch(t, statements)
}

func TestFrontendPredictionAssignmentSuffixCrossProduct(t *testing.T) {
	lvalues := []string{
		`$value`, `$value[0]`, `$object->item`, `$object->items()[0]->value`,
		`$$name`, `${$name}`, `$object->{$field}[0]`, `Type::$item`,
		`factory()[0]`, `factory()->item`, `$callback()->item`,
		`factory()()->items()[0]`, `Type::make()->item`, `($object)->item`,
		`($object->items())[0]`,
	}
	operators := []string{"=", "+=", "-=", "*=", "**=", "/=", ".=", "%=", "&=", "|=", "^=", "<<=", ">>=", "??="}
	var statements []string
	for _, lhs := range lvalues {
		for _, operator := range operators {
			statements = append(statements, fmt.Sprintf("%s %s $a = $b + $c * $d ?? $e and $f;", lhs, operator))
		}
		statements = append(statements, fmt.Sprintf("%s = &$reference;", lhs))
		// functionCallAssignable does not occur in the increment alternatives;
		// a static-method/grouped origin does, through assignableChain.
		if !strings.HasPrefix(lhs, "factory(") && lhs != `$callback()->item` {
			statements = append(statements, fmt.Sprintf("++%s; %s--;", lhs, lhs))
		}
	}
	statements = append(statements,
		`factory([fn($x) => $x], function ($x) { return $x + 1; })[Type::index()] = &$reference;`,
		`(factory([fn($x) => $x])->items())[0] += 1;`,
		`Type::make(fn($x) => $x)->{$field}[0] = $a = $b + $c;`,
	)
	assertPredictionAmbiguityBatch(t, statements)
}

func TestFrontendPredictionCastCallableCrossProduct(t *testing.T) {
	casts := []string{"bool", "int8", "int16", "int", "int64", "uint", "double", "real", "float", "string", "binary", "unicode", "array", "object", "resource", "unset"}
	operands := []string{`$value`, `$$name`, `factory($argument)`, `($a + $b)`, `'text'`, `"{$value}"`, `1`, `new Item`, `clone $object`, `!$value`, `-$value`, `~$value`, `fn($x) => $x`}
	var statements []string
	for _, cast := range casts {
		for _, operand := range operands {
			statements = append(statements, fmt.Sprintf("consume((%s)%s + $right);", cast, operand))
		}
		// These tokens also name functions. No adjacent cast operand means the
		// original parentheses/functionCall choice must remain available.
		statements = append(statements, fmt.Sprintf("consume((%s)($argument));", cast))
	}
	statements = append(statements,
		`($callback)(fn($x) => $x)()->items()[0];`,
		`($object->method)($argument)->items()[0];`,
		`($factory())::method($argument);`,
		`(($factory)->className)::$item;`,
		`($object)->{'method'}($argument)[0];`,
		`($object)->$method($argument);`,
	)
	assertPredictionAmbiguityBatch(t, statements)
}

func TestFrontendPredictionForeachTypeRefCrossProduct(t *testing.T) {
	sources := []string{`$values`, `$values[0]`, `$object->items()`, `config_get('list', [])`, `$callback()`, `[1, 2]`, `array(1, 2)`, `($values ?? [])`, `Type::$values`}
	bindings := []string{`$value`, `&$value`, `$key => $value`, `$key => &$value`, `[$a, $b]`, `list($a, , $b)`, `$key => list($a, $b)`, `$key => [$a, $b]`, `$value[0]`, `$object->item`, `$key => $value[0]`}
	var statements []string
	for _, source := range sources {
		for _, binding := range bindings {
			statements = append(statements, fmt.Sprintf("foreach (%s as %s) { consume($value); }", source, binding))
			statements = append(statements, fmt.Sprintf("foreach (%s as %s): consume($value); endforeach;", source, binding))
		}
	}
	for _, receiver := range []string{`Item`, `Ns\Item`, `\Ns\Item`, `static`, `$class`, `$$class`, `${$name}`, `$object->className`, `$object->typemap[$key]`, `($object->className)`, `factory($argument)->className`} {
		for _, argument := range []string{``, `$a || $object->member`, `fn($x) => $x + 1`, `function ($x) { return $x; }`, `['key' => &$reference]`} {
			statements = append(statements, fmt.Sprintf("$instance = new %s(%s);", receiver, argument))
		}
	}
	assertPredictionAmbiguityBatch(t, statements)
}

func TestFrontendPredictionNamespaceHeaderBoundaries(t *testing.T) {
	for index, source := range []string{
		`namespace First; $a = 1; namespace Second; $b = factory(fn($x) => $x);`,
		`namespace First { $a = 1; } namespace Second { $b = 2; } namespace { consume($a); }`,
		`$before = 1; namespace Foo\Bar { $inside = factory()->items()[0]; } $after = 2;`,
		`namespace Foo\Bar { namespace\helper(fn($x) => $x); $x = namespace\VALUE; }`,
		`namespace First { namespace\helper($argument); } namespace\helper($other); namespace Second { namespace\helper($tail); }`,
		`namespace if\endif { $x = 1; } namespace Second { $y = 2; }`,
		`namespace First { ?>outside<?php namespace\helper($argument); } namespace Second { ?>tail<?php consume($tail); }`,
		`namespace First; ?>outside<?php namespace Second; ?>tail<?php consume($tail);`,
		`namespace First { if ($ready): ?>body<?php echo 1; endif; } namespace Second { while ($ready): echo 2; endwhile; }`,
		`namespace A/* ; } */\B { consume("{$object->field}"); } namespace C { consume($tail); }`,
		"namespace " + strings.TrimSuffix(strings.Repeat("Segment\\", 128), "\\") + ` { consume($tail); } namespace Last { consume($tail); }`,
	} {
		t.Run(fmt.Sprintf("case %d", index), func(t *testing.T) {
			assertPredictionAmbiguitySource(t, []string{source})
		})
	}
}

func TestFrontendPredictionStaticVariableSuffixes(t *testing.T) {
	var statements []string
	for _, receiver := range []string{`$object`, `$objects[0]`, `$$class`, `Type`, `Ns\Type`, `\Ns\Type`, `static`, `parent`} {
		for _, property := range []string{`$definition`, `$definition['fields']`, `$definition[$key][0]`, `${$field}`, `$$field`} {
			access := receiver + "::" + property
			for _, statement := range []string{
				"consume(%s);", "consume(isset(%s));", "consume(%s($argument));",
				"%s = 1;", "%s = &$reference;", "%s += $a + $b * $c;",
				"++%s;", "%s--;", "consume(%s ?? $fallback);",
			} {
				statements = append(statements, fmt.Sprintf(statement, access))
			}
		}
	}
	for _, receiver := range []string{`($object)`, `($objects[0])`, `factory()`, `factory()->type`, `$object->getClass()`} {
		for _, property := range []string{`$definition`, `$definition['fields']`, `${$field}`} {
			access := receiver + "::" + property
			statements = append(statements, "consume("+access+");", "consume(("+access+")($argument));")
		}
	}
	statements = append(statements,
		`$instance = new static($elements, $this->_flexDirectory);`,
		`$instance = new static($a || $object->property, fn($x) => $x);`,
		`$instance = new static(['key' => &$reference])->item;`,
		`$instance = new $object::$definition($argument);`,
		`$instance = new Type::$definition($argument);`,
	)
	assertPredictionAmbiguityBatch(t, statements)
}

func TestFrontendPredictionConstantInitializerBoundaries(t *testing.T) {
	var statements []string
	index := 0
	for _, item := range []string{
		`012`, `0x10`, `0b10`, `1.25`, `null`, `true`, `false`,
		`$value`, `fn($x) => $x`, `fn($x): int => $x`, `&$reference`,
		`'key' => &$reference`, `$key + 1 => &$reference`,
		`factory(['key' => fn($x) => $x])->items()[0]`,
		`function ($x) { return match($x) { 1 => $value, default => null }; }`,
	} {
		for _, wrapper := range []string{"[%s]", "array(%s)", "[%s,]", "array(%s,)"} {
			expression := fmt.Sprintf(wrapper, item)
			statements = append(statements, fmt.Sprintf("const C%d = %s;", index, expression))
			index++
		}
	}
	for _, expression := range []string{
		`array(foo: 1)`, `array(foo: fn($x): int => $x)`, `array(...)`,
		`[...]`, `[... ...$values]`, `array(...$values)`, `list($first, $second)`,
		`[1, 2][0]`, `array(1, 2)[0]`, `[1, 2] + $other`, `array(1, 2) + $other`,
		`[1, 2] ? $yes : $no`, `array($ready ? 1 : 2)`,
		`'left' . 'right'`, `'left' . 12`, `-array(1, 2)`, `+['key' => 1]`,
	} {
		statements = append(statements, fmt.Sprintf("const C%d = %s;", index, expression))
		index++
	}
	assertPredictionAmbiguityBatch(t, statements)
}

func TestFrontendPredictionDynamicStaticRestoresSLL(t *testing.T) {
	for _, source := range []string{
		`<?php consume($object::$definition['fields']); println('tail');`,
		`<?php $value = isset($object::$definition['fields']); println('tail');`,
		`<?php consume($objects[0]::$definition[$key][0], Ns\Type::$definition['fields'], \Ns\Type::$definition, static::$definition, parent::$definition);`,
		`<?php $object::$definition['fields'] += $a + $b * $c; $object::$definition['fields'] = &$reference; $object::$definition['fields']--;`,
		`<?php $instance = new static($elements, $this->_flexDirectory); println('tail');`,
		`<?php $instance = new static($a || $object->property, fn($x) => $x); println('tail');`,
		`<?php factory(fn($x) => $x)->class = 1 + 2 * 3; factory(fn($x) => $x)->class = &$reference; println('tail');`,
		`<?php factory(fn($x) => $x) /* } ] */ -> /* ( */ class += $value; println('tail');`,
	} {
		t.Run(source, func(t *testing.T) {
			ast, parser, err := parsePredictionTree(source, antlr.PredictionModeSLL)
			require.NoError(t, err, "known ambiguity must finish without restarting the file")
			defer antlr4util.DetachParserATNSimulatorCaches(parser)
			require.Equal(t, antlr.PredictionModeSLL, parser.GetInterpreter().GetPredictionMode())
			reference, original, err := parsePredictionTree(source, antlr.PredictionModeLL, false)
			require.NoError(t, err)
			defer antlr4util.DetachParserATNSimulatorCaches(original)
			assertPredictionShapeEqual(t, predictionTreeShape(reference), predictionTreeShape(ast), "SLL", source)
		})
	}
	for _, source := range []string{
		`<?php $value = $object::$definition['fields'][;`,
		`<?php $object::$definition['fields'] = &;`,
		`<?php $instance = new static($elements, $this->);`,
		`<?php $instance = new static(fn($x) => );`,
		`<?php factory(fn($x) => $x)->class = ;`,
		`<?php factory(fn($x) => $x)->class = &;`,
	} {
		t.Run(source, func(t *testing.T) {
			_, parser, err := parsePredictionTree(source, antlr.PredictionModeSLL)
			require.Error(t, err)
			defer antlr4util.DetachParserATNSimulatorCaches(parser)
			require.Equal(t, antlr.PredictionModeSLL, parser.GetInterpreter().GetPredictionMode(), "restore the caller's mode on cancellation")
			_, original, err := parsePredictionTree(source, antlr.PredictionModeLL, false)
			require.Error(t, err)
			defer antlr4util.DetachParserATNSimulatorCaches(original)
		})
	}
}

func TestFrontendPredictionCatchAssertAndStaticCallBoundaries(t *testing.T) {
	assertPredictionAmbiguitySource(t, []string{
		`try { factory(); } catch (Ns\Failure | OtherFailure $error) { consume($error); } catch (static) { consume($tail); } finally { consume($tail); }`,
		`try { factory(); } finally { consume($tail); } catch\helper($argument);`,
		`try { factory(); } catch (Failure $error) { consume($error); } finally\helper($argument);`,
		`try { factory(); } catch (Failure $error) { consume($error); } finally::helper($argument);`,
		`try { factory(); } catch /* ) } */ (Failure $error) { ?>body<?php consume($error); } finally { consume($tail); }`,
		`assert($predicate); assert(fn($x) => $x); assert(function ($x) { return $x; }); assert(foo: $predicate);`,
		`$object->assert($predicate); Type::assert($predicate); assert\helper($predicate); (assert)($predicate);`,
		`$relationship::noConstraints(function () { return $object::$definition['fields']; })->items();`,
		`$relationship::noConstraints(fn($x) => $x)->item += 1; $relationship::noConstraints(fn($x) => $x)[0] = &$reference;`,
		`$relationship::noConstraints(function () { return $value; })->getClass()::name($argument);`,
		`$relationship::fn($value); $relationship::match($value); $relationship::$callback($argument);`,
	})
}

func TestFrontendPredictionParenthesizedStaticReceivers(t *testing.T) {
	receivers := []string{
		`new class extends Base { public function item($value) { return $this->items($value)[0]->get(); } }`,
		`new class($argument) extends Base { public function item() { return $this->value; } }`,
		`new Item($argument)`, `new $type($argument)`, `new static($argument, $this->item)`,
		`factory(fn($x) => $x)`, `$object`, `$objects[0]`, `[1, 'key' => 2]`,
		`(int)$value`, `function ($x) { return $x; }`, `/* )]} */ $object /* ({[ */`,
	}
	var statements []string
	for _, receiver := range receivers {
		for _, member := range []string{`name`, `$property`, `$property['key']`, `${$field}`} {
			access := "(" + receiver + ")::" + member
			for _, suffix := range []string{"", "($argument)", "[0]", "->item", "?->item", "($argument)->items()[0]"} {
				statements = append(statements, "consume("+access+suffix+");")
			}
			// Group the whole static access before reserving an assignable
			// suffix. Its owner decides where the inner member loop stops.
			statements = append(statements,
				"("+access+")->item = 1;",
				"("+access+")[0] = &$reference;",
				"("+access+")->item += $a + $b * $c;",
			)
		}
	}
	assertPredictionAmbiguityBatch(t, statements)
}

func TestFrontendPredictionClassMemberKeywordBoundaries(t *testing.T) {
	var statements []string
	for _, receiver := range []string{`$object`, `$objects[0]`, `$object->items()[0]`, `factory($argument)`, `Type::make($argument)`, `($object)`, `(new Item($argument))`, `(new class {})`} {
		for _, member := range []string{"->class", "?->class", "/* ] } */ -> /* ({ */ class"} {
			access := receiver + member
			for _, suffix := range []string{"", "($argument)", "[0]", "($argument)[0]", "($argument)->items()[0]", "->item"} {
				statements = append(statements, "consume("+access+suffix+");")
			}
			if member != "?->class" {
				statements = append(statements,
					access+"[0] = 1;", access+"[0] = &$reference;", access+"[0] += $a + $b * $c;",
				)
			}
		}
	}
	assertPredictionAmbiguityBatch(t, statements)
}

func TestFrontendPredictionAnonymousClassNewExprBoundaries(t *testing.T) {
	var statements []string
	for _, expression := range []string{
		`new class {}`,
		`new class($argument) { public function item($value) { return $this->class($value); } }`,
		`new class extends Base { public function item($value) { return $this->items($value)[0]->get(); } }`,
		`new class($argument) extends Base implements I, J { public function item() { return factory()->class(); } }`,
		`new class() extends \Ns\Base implements \Ns\I, J { public function item(): int { return 1; } }`,
		`new /* )]} */ class /* ({[ */ ($argument) extends Base { public function item() { return $this->value; } }`,
		`new #[Attribute] class($argument) extends Base { public function item() { return $this->value; } }`,
	} {
		for _, suffix := range []string{"", "($argument)", "[0]", "->item($argument)", "?->item($argument)"} {
			statements = append(statements, "consume("+expression+suffix+");")
		}
		for _, suffix := range []string{"->item($argument)", "::name($argument)", "::$property[0]", "->class($argument)", "[0]"} {
			statements = append(statements, "consume(("+expression+")"+suffix+");")
		}
		statements = append(statements,
			"("+expression+")->item = &$reference;",
			"("+expression+")->item += 1;",
		)
	}
	assertPredictionAmbiguityBatch(t, statements)
}

func TestFrontendPredictionClassConstructorLegacyIndex(t *testing.T) {
	// Class is also a keyword identifier. A balanced {...} after new class
	// may be a legacy index on the named constructor result, rather than an
	// anonymous class body. Invalid class members must not reject that arm.
	var statements []string
	for _, expression := range []string{
		`new class($argument) {$value}`, `new class {1}`, `new class {[1]}`,
		`new class {null}`, `new class {'text'}`, `new class {($a ? $b : $c)}`,
		`new class($argument) {$object->items()[0]}`,
		`new class($argument) {$this->class($argument)}`,
		`new class($argument) {fn($x) => $x}`,
		`new /* }]) */ class($argument) /* ({[ */ {$value}`,
	} {
		for _, suffix := range []string{"", "($argument)", "[0]", "->item", "?->item", "->class($argument)"} {
			statements = append(statements, "consume("+expression+suffix+");")
		}
		statements = append(statements,
			"consume(("+expression+")::name($argument));",
			"("+expression+")->item = 1;",
			"("+expression+")[0] = &$reference;",
			"("+expression+")->item += $a + $b * $c;",
			"("+expression+")->item--;",
		)
	}
	assertPredictionAmbiguityBatch(t, statements)
}

func TestFrontendPredictionAmbiguityRejectsIncompleteOperands(t *testing.T) {
	// Invalid samples cannot share one recovered source: a later error would
	// conceal a fast decision that incorrectly accepted an earlier statement.
	for _, fragment := range []string{
		`$value = [1 => ];`, `$value = [($key) => &];`, `$value = [...$value, ,];`,
		`$value = [fn($x) => ];`, `$value = array('key' => &$value, ,);`,
		`$value = (int)$value + ;`, `$value = (int)$object->;`, `$value = ($a ? $b);`,
		`$value = ($a ?? );`, `$value = $a ** ;`, `$value = $a ? $b : ;`,
		`$value = $object?->;`, `$value = $object[;`, `factory()->item = &;`,
		`foreach ($values as [$a, $b) { }`, `foreach ($values as $key => list($a,) { }`,
		`foreach (factory(fn($x) => ) as $value) { }`,
		`$value = new ($object->className)(;`, `$value = new $object->map[($key)();`,
		`$value = factory()->getClass()::;`, `$value = ($callback)(fn($x) => )();`,
		`++factory()[0];`, `factory()->item--;`, `[,$a,,[$b,$c],] = $values;`,
		`namespace \Foo\Bar { consume($argument); } namespace Tail; consume($tail);`,
		`($object)::$property = 1;`, `(new Item)::$property = &$reference;`,
		`($object)::method()->item += 1;`, `(new class {})::name[0] = 1;`,
		`(new Item)::method(];`, `(new class { public function item() { return 1; })::method();`,
		`$object?->class[0] = 1;`, `new class($argument) extends { };`,
		`new class($argument) extends Base {$value};`, `new class implements I {[1]};`,
	} {
		t.Run(fragment, func(t *testing.T) {
			source := "<?php " + fragment + " println('tail');"
			_, original, err := parsePredictionTree(source, antlr.PredictionModeLL, false)
			require.Error(t, err, "reference must reject this incomplete operand")
			antlr4util.DetachParserATNSimulatorCaches(original)
			_, err = Frontend(source, CreateBuilder().GetAntlrCache())
			require.Error(t, err, "production must preserve reference rejection")
			_, parser, err := parsePredictionTree(source, antlr.PredictionModeLL)
			require.Error(t, err, "fast LL must preserve reference rejection")
			antlr4util.DetachParserATNSimulatorCaches(parser)
		})
	}
}
