package tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Check complete operand multisets, including the marker after each ambiguous
// expression. An AST which drops a suffix or consumes the next statement must
// fail even if its remaining nodes can be compiled successfully.
func checkPredictionSemanticOperands(t *testing.T, source string, want []string) {
	t.Helper()
	prog, err := ssaapi.Parse(source, ssaapi.WithLanguage(ssaconfig.PHP))
	require.NoError(t, err)
	var got []string
	for _, call := range prog.Ref("println").GetUsers() {
		operand := call.GetOperand(1)
		require.NotNil(t, operand)
		got = append(got, operand.String())
	}
	require.ElementsMatch(t, want, got)
}

func TestFrontendPredictionPreservesOperatorSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []string
	}{
		{
			"arithmetic bitwise and shifts",
			`<?php println(1 + 2 * 3); println((1 + 2) * 3); println(10 - 3 - 2);
println(20 / 2 / 2); println(5 + 2 * 3 << 1); println(5 | 2 & 1);
println(1 << 2 + 1); println(4 ^ 3 & 1); println(10 % 4 + 1); println("tail");`,
			[]string{"7", "9", "5", "5", "22", "5", "8", "5", "3", `"tail"`},
		},
		{
			"parameter binding and right associative power",
			`<?php function inspect($a, $b, $c) {
println($a + $b * $c); println(($a + $b) * $c); println($a - $b - $c);
println($a ** $b ** $c); println($a | $b & $c); } println("tail");`,
			[]string{
				"add(Parameter-$a, mul(Parameter-$b, Parameter-$c))",
				"mul(add(Parameter-$a, Parameter-$b), Parameter-$c)",
				"sub(sub(Parameter-$a, Parameter-$b), Parameter-$c)",
				"pow(Parameter-$a, pow(Parameter-$b, Parameter-$c))",
				"or(Parameter-$a, and(Parameter-$b, Parameter-$c))", `"tail"`,
			},
		},
		{
			"cast operands and enclosing parentheses",
			`<?php $x = 3; println((int)$x); println((float)$x + 0.5);
println((string)$x); println((bool)$x); println((int)$x + 2 * 3);
println(((int)$x) + 2 * 3); println("tail");`,
			[]string{"castType(number, 3)", "add(castType(number, 3), 0.5)",
				"castType(string, 3)", "castType(boolean, 3)",
				"add(castType(number, 3), 6)", "add(castType(number, 3), 6)", `"tail"`},
		},
		{
			"nested assignment operators",
			`<?php $a = 1; $b = 2; println($a = $b = 1 + 2 * 3); println($a);
println($b); println($a += ($b *= 2)); println($a); println($b); println("tail");`,
			[]string{"7", "7", "7", "21", "21", "14", `"tail"`},
		},
		{
			"short circuit conjunction retains branch side effects",
			`<?php $b = ($a = 1) && ($a = 0); println($b); println($a); println("tail");`,
			[]string{"phi($b)[eq(0, true),false]", "phi($a)[0,1]", `"tail"`},
		},
		{
			"short circuit disjunction retains branch side effects",
			`<?php $b = ($a = 0) || ($a = 1); println($b); println($a); println("tail");`,
			[]string{"phi($b)[true,eq(1, true)]", "phi($a)[0,1]", `"tail"`},
		},
		{
			"named and variable callbacks retain callee and arguments",
			`<?php function named($x) { return $x + 1; } $f = fn($x) => $x * 2;
println(named(1 + 2 * 3)); println($f(1 + 2 * 3));
println((named)(10 - 3 - 2)); println(($f)(10 - 3 - 2)); println("tail");`,
			[]string{"Function-named(7)", "Function-$f(7)",
				"Function-named(5)", "Function-$f(5)", `"tail"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkPredictionSemanticOperands(t, tc.source, tc.want)
		})
	}
}

func TestFrontendPredictionPreservesCallResultAssignments(t *testing.T) {
	// Cross the receiver spelling, member/index suffix and assignment kind.
	// These calls are intentionally unresolved: the assignment RHS and its
	// side effects must still be preserved by the frontend and SSA builder.
	for _, receiver := range []string{
		"factory()", "Factory::make()", "$callback()", "($callback)()",
		"factory()()", "$object->make()", "(factory())()",
	} {
		for _, suffix := range []string{"->value", "[0]", "->items[0]", "[0]->value"} {
			for _, reference := range []bool{false, true} {
				name := receiver + suffix
				assignment := " = $target = 1 + 2 * 3"
				if reference {
					name += "/reference"
					assignment = " = &$target"
				}
				t.Run(name, func(t *testing.T) {
					source := fmt.Sprintf(`<?php $target = 7; println(%s%s%s); println($target); println("tail");`, receiver, suffix, assignment)
					checkPredictionSemanticOperands(t, source, []string{"7", "7", `"tail"`})
				})
			}
		}
	}
	t.Run("assignment in index is retained", func(t *testing.T) {
		checkPredictionSemanticOperands(t,
			`<?php $index = 0; factory()[$index = 1]->value = $target = 7; println($index); println($target); println("tail");`,
			[]string{"1", "7", `"tail"`})
	})
}

func TestFrontendPredictionPreservesArrayAndForeachValues(t *testing.T) {
	t.Run("key expressions references and nested values", func(t *testing.T) {
		checkPredictionSemanticOperands(t, `<?php $key = 1; $value = 7;
$items = ['one' => 1 + 2 * 3, ($key + 1) => &$value, 'nested' => ['x' => 9]];
println($items['one']); println($items[2]); println($items['nested']['x']); println("tail");`,
			[]string{"7", "7", "9", `"tail"`})
	})
	// A singleton iterable makes the SSA values precise; no runtime iteration,
	// network fixture or timing assumption is needed to distinguish the paths.
	for _, tc := range []struct{ name, source string }{
		{"variable iterable", `<?php $items = ['key' => 7]; foreach ($items as $key => $value) { println($key); println($value); } println("tail");`},
		{"literal iterable", `<?php foreach (['key' => 7] as $key => $value) { println($key); println($value); } println("tail");`},
		{"grouped iterable", `<?php $items = ['key' => 7]; foreach (($items) as $key => $value) { println($key); println($value); } println("tail");`},
		{"reference binding", `<?php $items = ['key' => 7]; foreach ($items as $key => &$value) { println($key); println($value); } println("tail");`},
		{"keyword array iterable", `<?php foreach (array('key' => 7) as $key => $value) { println($key); println($value); } println("tail");`},
		{"reference binding in alternative syntax", `<?php $items = ['key' => 7]; foreach ($items as $key => &$value): println($key); println($value); endforeach; println("tail");`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkPredictionSemanticOperands(t, tc.source, []string{`"key"`, "7", `"tail"`})
		})
	}
}

func TestFrontendPredictionPreservesDynamicReceiverSideEffects(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"variable constructor type", `<?php $type = 'Item'; $item = new $type($argument = 1 + 2 * 3); println($argument); println("tail");`},
		{"indirect variable constructor type", `<?php $alias = 'type'; $type = 'Item'; $item = new $$alias($argument = 1 + 2 * 3); println($argument); println("tail");`},
		{"indexed constructor type", `<?php $types = ['item' => 'Item']; $item = new $types['item']($argument = 1 + 2 * 3); println($argument); println("tail");`},
		{"member constructor type", `<?php $item = new $holder->type($argument = 1 + 2 * 3); println($argument); println("tail");`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkPredictionSemanticOperands(t, tc.source, []string{"7", `"tail"`})
		})
	}
	t.Run("call result dynamic static receiver", func(t *testing.T) {
		checkPredictionSemanticOperands(t,
			`<?php factory($argument = 1 + 2 * 3)->getClass()::method($other = 10 - 3 - 2); println($argument); println($other); println("tail");`,
			[]string{"7", "5", `"tail"`})
	})
	t.Run("grouped dynamic static property", func(t *testing.T) {
		checkPredictionSemanticOperands(t,
			`<?php class Item { public static $value = 7; } $type = 'Item'; println($type::$value); println(($type)::$value); println("tail");`,
			[]string{"7", "7", `"tail"`})
	})
}

func TestFrontendPredictionPreservesSyntaxFlowArguments(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"reference array element", `<?php function route($input) { $items = ['ref' => &$input]; exec($items['ref']); }`},
		{"call result member assignment", `<?php function route($input) { factory()[0]->value = $assigned = $input; exec($assigned); }`},
		{"foreach reference binding", `<?php function route($input) { $items = ['ref' => $input]; foreach ($items as $key => &$value) { exec($value); } }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := ssaapi.Parse(tc.source, ssaapi.WithLanguage(ssaconfig.PHP))
			require.NoError(t, err)
			result, err := prog.SyntaxFlowWithError(`exec(* as $payload)`, ssaapi.QueryWithMemory())
			require.NoError(t, err)
			values := result.GetValues("payload")
			require.Len(t, values, 1)
			require.Equal(t, "Parameter-$input", values[0].String())
		})
	}
}
