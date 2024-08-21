package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestParseSSA_functionDecl(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
function A(int $a){
	println($a);
}
A();
`, []string{
			"Parameter-$a",
		}, t)
	})

	t.Run("mock function1", func(t *testing.T) {
		test.MockSSA(t, `<?php
function testFunction2($a, $b='1', $c=array(1,2,3,), $d) {
	1&&1;
	return 1;
}
`)
	})

	t.Run("test php not freeValue", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
function test() {
	println($a); 
}
`, []string{"Undefined-$a"}, t)
	})
}

func TestParseSSA_Function_SideEffect(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php 
		function A($a){
			$a = 33;
		}
		$a = 1;
		println($a);
		A($a);
		println($a);
		`, []string{"1", "1"}, t)
	})

	t.Run("reference", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
		function A(&$a){
			$a = 33;
		}
		$a = 1;
		println($a);
		A($a);
		println($a);
		`, []string{
			"1",
			"side-effect(33, $a)",
		}, t)
	})

	t.Run("multiple mix reference parameter", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
		function A(&$a, $b, &$c){
			$a = 33;
			$b = 33;
			$c = 33;
		}
		$a = 1;
		$b = 1;
		$c = 1;
		println($a);
		println($b);
		println($c);
		A($a, $b, $c);
		println($a);
		println($b);
		println($c);
		`, []string{
			"1", "1", "1",
			"side-effect(33, $a)",
			"1",
			"side-effect(33, $c)",
		}, t)
	})

}

func TestParseSSA_FuncCall_DefaultParameter(t *testing.T) {
	t.Run("no default", func(t *testing.T) {
		test.MockSSA(t, `<?php
function a1($a, $b) {return "2";}
`)
	})
	t.Run("variable in outside", func(t *testing.T) {
		test.MockSSA(t, `<?php
$a = 1;
function Test(){
$a=3;
println($a);
}
`)
	})

	t.Run("default float", func(t *testing.T) {
		test.MockSSA(t, `<?php
function a2($a, $b="1"."2") {return "2";}
`)
	})

	t.Run("default array", func(t *testing.T) {
		test.MockSSA(t, `<?php
function a3($a, $b=["1", "2"], $dd=null) {return $b[0];}
`)
	})

	t.Run("default array 2", func(t *testing.T) {
		test.MockSSA(t, `<?php
function a3($a, $b=["1", "2"], $dd=array(1,2,3), $e=1) {return "2";}
`)
	})
}

func TestParseSSA_Function(t *testing.T) {
	code := `<?php
function a(string $a){
$a = "1";
}
$a = "2";
println($a);
a($a);
println($a);
`
	test.MockSSA(t, code)
}

func TestParseSSA_FuncCall(t *testing.T) {
	t.Run("test-1", func(t *testing.T) {
		code := `<?php
function funcName() {return "2";}
funcName().$a;`
		test.CheckError(t, test.TestCase{
			Code: code,
			Want: []string{ssa.ValueUndefined("$a")},
		})
	})
	t.Run("test-2", func(t *testing.T) {
		code := `<?php
function A(int $a){
	println($a);
}
A(1);`
		test.CheckPrintlnValue(code, []string{"Parameter-$a"}, t)
	})
}

func TestParseSSA_Array(t *testing.T) {
	code := `<?php
$a[1]=1;
println($a[1]);
`
	test.CheckPrintlnValue(code, []string{"1"}, t)
}

func TestParseSSA_Closure(t *testing.T) {
	t.Run("syntax_Closure", func(t *testing.T) {
		code := `<?php
$a = function(){return 1;};`
		test.MockSSA(t, code)
	})
	t.Run("variable is inner for Closure", func(t *testing.T) {
		code := `<?php
$a = function(){
   $d = 1;
   println($d);
};
$a();`
		test.CheckPrintlnValue(code, []string{"1"}, t)
	})
	t.Run("variable is outside for Closure", func(t *testing.T) {
		code := `<?php
$d = 1;
$a = function()use($d){
    println($d);
};
`
		test.CheckPrintlnValue(code, []string{"FreeValue-$d"}, t)
	})
	t.Run("use Closure but not use", func(t *testing.T) {
		code := `<?php
$d = 1;
$a = function()use(&$d){
    $d = 2;
};
println($d);
`
		test.CheckPrintlnValue(code, []string{"1"}, t)
	})
	t.Run("use Closure but not use", func(t *testing.T) {
		code := `<?php
$d = 1;
$a = function()use(&$d){
    $d = 2;
};
$a();
println($d);
`
		test.CheckPrintlnValue(code, []string{"side-effect(2, $d)"}, t)
	})

}
func TestParseSSA_DefinedFunc(t *testing.T) {
	t.Run("include", func(t *testing.T) {
		code := `<?php
include "syntax/for.php";`
		test.MockSSA(t, code)
	})
	t.Run("include-2", func(t *testing.T) {
		code := `<?php
include("syntax/for.php");`
		test.MockSSA(t, code)
	})
	t.Run("include_once", func(t *testing.T) {
		code := `<?php
include_once("syntax/for.php");`
		test.MockSSA(t, code)
	})
	t.Run("require_once", func(t *testing.T) {
		code := `<?php
require_once("syntax/for.php");`
		test.MockSSA(t, code)
	})
	t.Run("eval_execute", func(t *testing.T) {
		code := `<?php
$a =eval("echo 1");`
		test.MockSSA(t, code)
	})
	t.Run("assert_execute", func(t *testing.T) {
		code := `<?php
$a =assert("echo 1");`
		test.MockSSA(t, code)
	})
}

func Test_Function_WithMemberCall(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintlnValue(`
		<?php
		class A {
			function fun1() {
				return "1";
			}
		}
		$a = new A();
		$b = $a->fun1();
		println($b);
		`, []string{"Undefined-$a.fun1(valid)(Undefined-$a)"}, t)
	})

	t.Run("multiple member call", func(t *testing.T) {
		test.CheckPrintlnValue(`

		<?php
		class A {
			function fun1() {
				return "1";
			}
		}
		class B {
			var A $a;
		}
		$b = new B();
		$b->a = new A();
		$call = $b->a->fun1();
		println($call);
		`, []string{"Undefined-$b.a.fun1(valid)(Undefined-$b.a)"}, t)
	})
}

func Test_InnerFunctionCall(t *testing.T) {
	t.Run("test-1", func(t *testing.T) {
		var code = `<?php
Phpinfo();
`
		test.MockSSA(t, code)
	})
	t.Run("test-2", func(t *testing.T) {
		var code = `<?php
$a = PHPINFO;
$a();
`
		test.MockSSA(t, code)
	})
	t.Run("test-3", func(t *testing.T) {
		code := `<?php
$a = <<<a
dad
a;
`
		test.MockSSA(t, code)
	})
}
func TestClosure(t *testing.T) {
	t.Run("global", func(t *testing.T) {
		code := `<?php
	$a = 1;
	function a(){
		global $a;
		println($a);
	}
	a();
`
		test.CheckSyntaxFlow(t, code,
			`println(* #-> *  as $param)`,
			map[string][]string{
				"param": {"1"},
			},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("lamda-has-free_value", func(t *testing.T) {
		code := `<?php
$d = 1;
$a = function($ba)use($d){
    println($d);
};
$a(1);`
		test.CheckPrintlnValue(code, []string{`FreeValue-$d`}, t)
		test.CheckSyntaxFlow(t, code,
			`println(* #-> * as $param)`,
			map[string][]string{
				"param": {"1"},
			},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("lamda-free-value", func(t *testing.T) {
		t.Run("lamda", func(t *testing.T) {
			code := `<?php
$a = function($ba)use($d){
    println($d);
};
$a(1);`
			test.CheckPrintlnValue(code, []string{`FreeValue-$d`}, t)
		})
	})
	t.Run("test-function", func(t *testing.T) {
		code := `<?php
function c($cmd){
	exec($cmd);
}
function a(){
	c("whoami");
}

`
		test.CheckSyntaxFlow(t, code,
			`exec(* #-> * as $param)`,
			map[string][]string{"param": {`"whoami"`}},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
}
func TestFunction(t *testing.T) {
	t.Run("prev function", func(t *testing.T) {
		code := `<?php

 b("whoami");

function b($cmd)
{
    exec("$cmd");
}`
		test.CheckSyntaxFlow(t, code,
			`exec(* #-> * as $param)`,
			map[string][]string{"param": {`"whoami"`}},
			ssaapi.WithLanguage(ssaapi.PHP),
		)
	})
	t.Run("test function in function", func(t *testing.T) {
		code := `<?php
function a()
{
    b("whoami");
}

function b($cmd)
{
    exec("$cmd");
}`
		test.CheckSyntaxFlow(t, code,
			`exec(* #-> * as $param)`,
			map[string][]string{"param": {`"whoami"`}},
			ssaapi.WithLanguage(ssaapi.PHP),
		)
	})

	t.Run("test function in namespace", func(t *testing.T) {
		code := `<?php

namespace a {
    function test($a)
    {
        exec("$a");
    }
}

namespace {
    use function a\test;
    function teee()
    {
        test("whoami");
    }
}`
		test.CheckSyntaxFlow(t, code,
			`exec(* #-> * as $param)`,
			map[string][]string{"param": {`"whoami"`}},
			ssaapi.WithLanguage(ssaapi.PHP),
		)
	})
	//todo： get top define有点问题
	t.Run("test function spin", func(t *testing.T) {
		code := `<?php

function a($cmd)
{
    b("whoam");
}

function b($cmd)
{
    if ($cmd == "whoami") {
        echo exec($cmd);
    } else {
        a($cmd . "i");
    }
}
`
		test.CheckSyntaxFlow(t, code,
			`exec(* #-> * as $param)`,
			map[string][]string{"param": {`"whoam"`}},
			ssaapi.WithLanguage(ssaapi.PHP),
		)
	})
	t.Run("test undefined function", func(t *testing.T) {
		code := `<?php
a($a);`
		test.CheckSyntaxFlow(t, code,
			`a as $target`,
			map[string][]string{"target": {"Undefined-a"}},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
}
