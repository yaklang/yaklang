package tests

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPHPClosureSideEffectMatrix(t *testing.T) {
	t.Run("basic closure by-reference write", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
$a = 1;
$f = function() use (&$a) {
	$a = 2;
};
$f();
println($a);
`, []string{"side-effect(2, $a)"}, t)
	})

	t.Run("boundary closure not called", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
$a = 1;
$f = function() use (&$a) {
	$a = 2;
};
println($a);
`, []string{
			"1",
		}, t)
	})

	t.Run("complex nested closures with branch and multi-call", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
$a = 1;
$factory = function() use (&$a) {
	return function($v) use (&$a) {
		if ($v > 0) {
			$a = 3;
		} else {
			$a = 4;
		}
	};
};
$update = $factory();
$update(1);
$update(0);
println($a);
`, []string{"side-effect(phi($a)[3,4], $a)"}, t)
	})

	t.Run("boundary closure alias chain invoke", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
$a = 1;
$f = function() use (&$a) {
	$a = 6;
};
$g = $f;
$h = $g;
$h();
println($a);
`, []string{"side-effect(6, $a)"}, t)
	})

	t.Run("complex variadic callback with branch and multi-call", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
function run($fn, ...$vals){
	$fn($vals[0]);
	$fn($vals[1]);
}
$a = 1;
$update = function($v) use (&$a) {
	if ($v > 0) {
		$a = 7;
	} else {
		$a = 8;
	}
};
run($update, 1, 0);
println($a);
`, []string{"side-effect(phi($a)[7,8], $a)"}, t)
	})
}
