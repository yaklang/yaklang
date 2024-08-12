package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
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
