package tests

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestReferenceParameterEdgeCases(t *testing.T) {
	t.Run("pass through by-reference chain", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
function step2(&$v){
	$v = 8;
}
function step1(&$v){
	step2($v);
}
$a = 1;
step1($a);
println($a);
`, []string{"side-effect(side-effect(8, $v), $a)"}, t)
	})

	t.Run("by-value chain should not mutate outer variable", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
function step2($v){
	$v = 8;
}
function step1($v){
	step2($v);
}
$a = 1;
step1($a);
println($a);
`, []string{
			"1",
		}, t)
	})

	t.Run("alias chain by-reference", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
function step2(&$v){
	$v = 9;
}
function step1(&$v){
	step2($v);
}
$a = 1;
$b = &$a;
step1($b);
println($a);
`, []string{"side-effect(side-effect(9, $v), $a)"}, t)
	})

	t.Run("variadic by-reference entry", func(t *testing.T) {
		test.CheckPrintlnValue(`<?php
function update_all(&$v, ...$vals){
	$v = $vals[0];
}
$a = 1;
update_all($a, 10, 11);
println($a);
`, []string{"side-effect(10, $a)"}, t)
	})
}
