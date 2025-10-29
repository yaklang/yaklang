package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestStatic(t *testing.T) {
	code := `<?php
function test(){
	static $a = 1;
	return function()use($a){
	};
}
$a = test();
println($a);
`
	ssatest.CheckPrintlnValue(code, []string{"Function-test()"}, t)
}
func TestStaticSelf(t *testing.T) {
	code := `<?php
class A{
    public static function test($a){
        println($a);
    }
    public static function testb(){
        self::test("aa");
    }
}
`
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{}, ssaapi.WithLanguage(ssaconfig.PHP))
}
func TestStatic2(t *testing.T) {
	code := `<?php
class A{
	public function test(){
		Request::post();
	}
}
`
	ssatest.CheckSyntaxFlow(t, code, `.post() as $sink`, map[string][]string{
		"sink": {"Undefined-Request.post()"},
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}
