package php

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestParameter(t *testing.T) {
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
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
func TestParameterMember(t *testing.T) {
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
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
