package php

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPhpWeakLanguage(t *testing.T) {
	t.Run("const function call", func(t *testing.T) {
		code := `<?php

function A($a){
    println($a);
}
$a = "A";
$a(1);`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("phi function call", func(t *testing.T) {
		code := `<?php
function A($a){
	println($a);
}
$a = "c";
if($c){
	$a = A;
}
$a(1);
`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("phi edge all call", func(t *testing.T) {
		code := `<?php
	function A($a){
		println($a);
	}
	function B($c){
		println($c);
	}
	$a = "A";
	if($c){
		$a = "B";
	}
	$a(1);
	`
		ssatest.CheckSyntaxFlow(t, code, `B() as $functionB
A() as $functionA`, map[string][]string{
			"functionA": {`phi($a)["B","A"](1)`},
			"functionB": {`phi($a)["B","A"](1)`},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("side effect call", func(t *testing.T) {
		code := `
/*
ir:
	function_A:
		parameter: $a
		call println(parameter[0])
	anymous_func:
		freeValue: $a
	function main:
		t1 = call anonymous_func() bind[{$a:"c"}]
		t2 = sideEffect: $a = "A" by t1

		t3 = call t2(1) args[1]
*/
<?php
function A($a){
	println($a);
}
$a = "c";
$b = function()use(&$a){
	$a = "A";
};
$b();
$a(1);
`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("side-effect value is phi", func(t *testing.T) {
		code := `<?php
function A($a){
	println($a);
}
$a = "c";
$b = function()use(&$a){
	if($c){
		$a = "A";
	}
};
$b();
$a(1);
`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("side effect value is phi", func(t *testing.T) {
		code := `<?php
function A($a){
	println($a);
}
function B($a){
	println($a);
}
$a = "c";
$b = function()use(&$a){
	if($c){
		$a = "A";
	}else{
		$a= "B";
	}
};
$b();
$a(1);
`
		ssatest.CheckSyntaxFlow(t, code, `B() as $functionB
A() as $functionA`, map[string][]string{
			"functionA": {`side-effect(phi($a)["A","B"], $a)(1)`},
			"functionB": {`side-effect(phi($a)["A","B"], $a)(1)`},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("phi side-effect", func(t *testing.T) {
		code := `<?php
function A($a){
	println($a);
}
function B($a){
	println($a);
}
function c($a){
	println($a);
}
$a = "c";
$b = function()use(&$a){
	if($c){
		$a = "A";
	}else{
		$a= "B";
	}
};
if($c){
	$b();
}
$a(1);
/*phi[$a](
	"c",
	sideEffect($a)[phi(A,B)]
)*/
`
		ssatest.CheckSyntaxFlow(t, code, `B() as $functionB
A() as $functionA
c() as $functionC
`, map[string][]string{
			"functionA": {`phi($a)[side-effect(phi($a)["A","B"], $a),"c"](1)`},
			"functionB": {`phi($a)[side-effect(phi($a)["A","B"], $a),"c"](1)`},
			"functionC": {`phi($a)[side-effect(phi($a)["A","B"], $a),"c"](1)`},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}
