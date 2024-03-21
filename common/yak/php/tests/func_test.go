package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func TestParseSSA_functionDecl(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
function A(int $a){
	println($a);
}
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

func TestParseSSA_FuncCall_DefaultParameter(t *testing.T) {
	t.Run("no default", func(t *testing.T) {
		test.MockSSA(t, `<?php
function a1($a, $b) {return "2";}
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
