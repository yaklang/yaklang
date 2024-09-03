package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestGlobal(t *testing.T) {
	t.Run("not set global", func(t *testing.T) {
		code := `<?php
		$a = $_GET[1];
		$b = base64_decode($a);
		println($b);
`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"Function-base64_decode", "Undefined-$a(valid)", "make(any)"}},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test globals", func(t *testing.T) {
		code := `<?php

$GLOBALS["a"] = 1;
println($GLOBALS['a']);
`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})
	t.Run("test globals in function", func(t *testing.T) {
		code := `<?php

$GLOBALS["a"] = 1;
function test(){
    println($GLOBALS['a']);
}
`
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})

	t.Run("test globals in function,and function used", func(t *testing.T) {
		code := `<?php
	
	$GLOBALS["a"] = 1;
	function test(){
	   return $GLOBALS['a'];
	}
	$a = test();
	println($a);
	`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"1"}},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
}
