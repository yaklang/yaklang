package php

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestParamCall(t *testing.T) {
	t.Run("parameterCall is called", func(t *testing.T) {
		code := `<?php
	function bBB($a){
	   echo($a);
	}
	function A($a){
	   $a(1);
	}
	A(bBB);
	`
		ssatest.CheckSyntaxFlow(t, code, `echo(* #-> * as $param)`, map[string][]string{
			"param": {"1"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
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
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"1"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("test parameterMember call and parameter", func(t *testing.T) {
		code := `<?php


class B{
    public function b($f){
        println($f);
    }
}

class A{
    public $field_b;
    public function run($c){
        $c->field_b->b(1);
    }
}

$a = new A();
$a->field_b = new B();

$b = new A();
$b->run($a);
`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"1"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("test freeValue call", func(t *testing.T) {
		code := `<?php

function test($a){
    println($a);
}
function A($a){
    $a(1);
}

$a = test;
$b = function()use(&$a){
    A($a);
};
$b();
`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})

	t.Run("test sideEffect call parameter", func(t *testing.T) {
		code := `<?php

function test1($c){
	println($c);
}
function test($c){
	println($c);
}
function A($f,$v){
    $f($v);
}

$a = test1;
$b = function()use(&$a){
	$a = test;
};
if($c){
	//$a = test
	$b();
	A($a,1);
}else{
	//$a = test1
	A($a,2);
}
`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1", "2"})
	})

	t.Run("big test for more parameterMember", func(t *testing.T) {
		code := `<?php
	/*
	
	*/
	class A{
	 public $c;
	 public function FunctionA($a){
	     println($a->c);
	 }
	}
	function C($c){
	 $c->FunctionA($c);
	}
	$a = new A();
	$a->c = 2;
	C($a);
	`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"2"})
	})
	t.Run("big test for parameter and feeevalue", func(t *testing.T) {
		code := `<?php

/*
	function_test:
		call println($a)
	function_A:
		parameterMember: [$a->test]
			call $a->test(1) argsMembers[]
	anymousFunc:
        fv: [$a]
        parameterMember: [$a->test]
        call FuncA($a) Bind[$a] ArgsMember[$a->test]

	topDef:
		println(* #-> * as $param)
	
		param($a) -> getFunc test
		test      -> getCallBy() -> foreach ArgsMember -> $a->test ->parameterMember anymousFunc 
		$a->test  -> getCallBy() -> foreach ArgsMember -> $a->test ->parameterMember FuncA
		inst
*/

class A{
    public function test($a){
        println($a);    
    }
}
function FuncA($a){
    $a->test(1);
}

$a = new A();
$b = function()use(&$a){
    FuncA($a);
};
$b();
`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("test const sideEffect call", func(t *testing.T) {
		code := `<?php
	
	function a($a){
	  println($a);
	}
	
	$b = "c";
	$c = function()use(&$b){
		$b = "a";
	};
	$c();
	$b(1);`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("test blueprint parameterMember", func(t *testing.T) {
		code := `
/*
println(* #-> * as $param)
topDef:
	$a->c parameterMember
*/
<?php
 class A{
    public $c;
    public function FunctionA($a){
        println($a->c);
    }
 }
 function C($c){
    $c->FunctionA($c); //undefined argsMember
 }
 $a = new A(); //call -> return -> make -> 
 $a->c = 2;
 C($a);
 // parameterMemberCall/MoreParameterMemberCall`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"2"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}

func TestCode(t *testing.T) {
	code := `<?php
a(1,2,3,4,5);
`
	ssatest.CheckSyntaxFlow(t, code, `a(,* as $param)`, map[string][]string{}, ssaapi.WithLanguage(ssaconfig.PHP))
}
