package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestStatic(t *testing.T) {
	code := `<?php
function test(){
    static $a=1;
    return $a;
}
$a = test();
println($a);
`
	ssatest.CheckSyntaxFlow(t, code,
		`println(* #-> * as $param)`,
		map[string][]string{},
		ssaapi.WithLanguage(ssaapi.PHP))
	//ssatest.CheckPrintlnValue(code, []string{""}, t)
}
