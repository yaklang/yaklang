package php

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
	t.Run("blueprint println in function", func(t *testing.T) {
		code := `<?php

class A{
    public $a;
}
function printValue($a){
    println($a->a);
}

$classA = new A();
$classA->a = 2;
printValue($classA);
`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"2"})
	})
	t.Run("blueprint test", func(t *testing.T) {
		code := `<?php

class A{
    public B $a;
}
class B{
    public static function bb($a){
        println($a);
    }
}
$c = new A();
$c->a::bb(1);`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`,
			map[string][]string{
				"param": {"1"},
			}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
	//todo:
	//	t.Run("Loop func", func(t *testing.T) {
	//		code := `f1 = (a1) =>{
	//    return a1;
	//}
	//f2 = (a2) =>{
	//    target = f1(a2)
	//    if target<2{
	//        f2(2)
	//    }
	//}
	//
	///*
	//a1 #-> as $param
	//
	//a1 -(getCallBy)(getAParam)-> a2 跨过程，刷新上下文
	//a2 ->(getCallBy)(getAParam)-> f1(a2) 非跨过程
	//f1(a2) -> f1 跨过程
	//f1 -> a1  非跨过程
	//a1 -> a2 跨过程，检测到有这个过程
	//*/
	//`
	//		ssatest.CheckSyntaxFlow(t, code, `a1 #-> as $param`, map[string][]string{
	//			"param": {"2"},
	//		}, ssaapi.WithLanguage(ssaconfig.Yak))
	//	})
}
