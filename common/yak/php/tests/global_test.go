package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestGlobal(t *testing.T) {
	t.Run("not set global", func(t *testing.T) {
		code := `<?php
		$a = $_GET[1];
		println($a);
`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> * as $param)`,
			map[string][]string{},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("globals", func(t *testing.T) {
		code := `<?php
$GLOBALS['a']=1;


function test(){
	return $GLOBALS['a'];
}

$d = test();

println($d);
`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"make(any)", "1", "Undefined-.global.a(valid)"}},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
}
