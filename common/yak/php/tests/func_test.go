package tests

import (
	"testing"

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
function testFunction2($a, $b='1', $c=array(1,2,3,), string $d) {
	1&&1;
	return 1;
}
`)
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
	t.Run("test", func(t *testing.T) {
		test.MockSSA(t, `<?php
function funcName() {return "2";}
funcName().$a;
`)
	})

	t.Run("test-2", func(t *testing.T) {
		test.MockSSA(t, `<?php
function A(int $a){
	println($a);
}
A(1);
`)
	})

}
