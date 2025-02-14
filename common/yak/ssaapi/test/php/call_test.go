package php

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestCall(t *testing.T) {
	t.Run("test const sideEffect call", func(t *testing.T) {
		code := `
/*
function a:
	parameter: $a
	call println($a)
function main:


function anymousFunc_C:
	freeValue: $b
	sideEffect: $b
	b = "a"

function main:
	t12 = call anymousFunc_C() bind[{$b: "c"}]
	b1 = sideEffect("a") by t12
	call b1(1);

topDef:
	param $a -> getFunc -> function_a
	function_a -> getCallBy()
*/
<?php
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
	t.Run("test function sideEffect call", func(t *testing.T) {
		code := `
<?php
	function a($a){
	   println($a);
	}
	$b = "c";
	$c = function()use(&$b){
		$b = a;
	};
	$c();
	$b(1);`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
}
