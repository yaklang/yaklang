package php

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestBlueprintParameterMember(t *testing.T) {
	t.Run("parameterMember-custom-value", func(t *testing.T) {
		code := `<?php
class B{
	public $b = 1;
}
class A{
	public $a;
	public function A(){
		println($this->a->b);
	}
}
$a =new A();
$a->a = new B();
$a->A();
`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("parameterMember-Method", func(t *testing.T) {
		code := `<?php
class B{
	public function b($a){
		println($a);
	}
}
class A{
	public $a;
	public function A(){
		$this->a->b(1);
	}
}
$a =new A();
$a->a = new B();
$a->A();
`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
	t.Run("parameter func", func(t *testing.T) {
		code := `<?php
class A{
	public function b($a){
		println($a);
	}
}
function func_A($a){
	$b = $a->b($_POST[1]);
}

$a = new A();
func_A($a);
`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"_POST"})
	})
}
