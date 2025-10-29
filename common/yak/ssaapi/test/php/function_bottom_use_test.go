package php

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBottomObject(t *testing.T) {
	t.Run("object member", func(t *testing.T) {
		code := `<?php
$b = $_GET[1];
a($b);
`
		ssatest.CheckSyntaxFlowContain(t, code,
			`_GET.* --> as $sink`,
			map[string][]string{
				"sink": {"Undefined-a(Undefined-$b(valid))"},
			},
			ssaapi.WithLanguage(ssaconfig.PHP))
	})
}
func TestCallBottomUse(t *testing.T) {

	code := `<?php
function a($a){
	println($a);
}
a($_GET[1]);
`
	ssatest.CheckSyntaxFlowContain(t, code,
		`_GET --> as $sink`,
		map[string][]string{
			"sink": {"Function-println(Parameter-$a)"},
		},
		ssaapi.WithLanguage(ssaconfig.PHP),
	)
}
func TestFunction(t *testing.T) {
	t.Run("function return", func(t *testing.T) {
		code := `<?php
function a(){
	return $_GET[1];
}
$c = a();
println($c);
`
		ssatest.CheckSyntaxFlowContain(t, code, `_GET.* --> as $sink`,
			map[string][]string{
				"sink": {"Function-println(Function-a())"},
			},
			ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("parameter function call", func(t *testing.T) {
		code := `<?php
	function bBB($a){
	   echo($a);
	}
	function A($a){
	   $a(1);
	}
	A(bBB);`
		ssatest.CheckSyntaxFlowContain(t, code,
			`bBB --> as $sink`,
			map[string][]string{
				"sink": {"Parameter-$a(1)"},
			},
			ssaapi.WithLanguage(ssaconfig.PHP),
		)
	})

	t.Run("parameter function call value", func(t *testing.T) {
		code := `<?php
		function bBB($a){
		   echo($a);
		}
		function B($e,$f){
			A($e,$f);
		}
		function A($a,$c){
		   $a($c);
		}
		B(bBB,"cc");
	`
		ssatest.CheckSyntaxFlowContain(t, code,
			`e"cc" as $source
	$source --> as $sink
	`,
			map[string][]string{
				"sink": {"Function-echo(Parameter-$a)"},
			},
			ssaapi.WithLanguage(ssaconfig.PHP),
		)
	})
}

func TestBlueprintBottomUse(t *testing.T) {
	code := `<?php

class A{
    public $a;
}
function printValue($a){
    println($a->a);
}

$classA = new A();
$classA->a = 2;
printValue($classA);
`
	ssatest.CheckSyntaxFlowContain(t, code, `A() --> as $sink`, map[string][]string{
		"sink": {"Function-println(ParameterMember-parameter[0].a)"},
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}
func TestParameterMemberIsCall(t *testing.T) {
	t.Run("parameterMember is called", func(t *testing.T) {
		code := `<?php
	class A{
		public function b($a){
			println($a);
		}
	}
	function bBB($a){
		   echo($a->b(1));
	}
	$a = new A();
	bBB($a);
	`
		ssatest.CheckSyntaxFlowContain(t, code, `A() --> as $sink`, map[string][]string{
			"sink": {"Function-echo(ParameterMember-parameter[0].b(Parameter-$a,1))"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}

func TestObject(t *testing.T) {
	code := `<?php
function functionCC($a){
	return $a;
}
$get = $_GET[1];
$cd = functionCC($get);
echo($cd);
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		result, err := prog.SyntaxFlowWithError(`_GET.* --> as $sink`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		result.Show(sfvm.WithShowDot(true))
		values := result.GetValues("sink")
		require.Contains(t, values.String(), "Function-echo(Function-functionCC(Undefined-$get(valid)))")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}
