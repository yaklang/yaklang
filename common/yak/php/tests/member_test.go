package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestParseSSA_BasicMember(t *testing.T) {
	t.Run("slice normal", func(t *testing.T) {
		test.Check(t, `<?php
function dump($a){}
		$c=[1,2,3];
		dump($c[2]);
		echo 1,2,3,5;
		`, func(prog *ssaapi.Program) error {
			prog.Show()
			return nil
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("array assign", func(t *testing.T) {
		code := `<?php
$files[] = array(
    'url' => substr($path2, a),
    'mtime' => filemtime($path2)
);
println($files);
`
		test.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{"param": {"make(any)", "Undefined-substr", "Undefined-$path2", "Undefined-a", "Undefined-filemtime"}}, ssaapi.WithLanguage(ssaconfig.PHP))
	})

	//todo:

	//	t.Run("test loop", func(t *testing.T) {
	//		code := `<?php
	//
	//$rs = new obb;
	//$rs->a=  2;
	//for ($i = 1; $i < 10; $i++) {
	//    $rs->a = 1;
	//    for ($j = 1;$j < 10; $j++) {
	//        $rs->a = $rs->a+$j;
	//    }
	//    /*
	//		$rs #-1.a
	//		$j	   phi
	//		$rs->a phi(#-1.a)[1,phi()]
	//	*/
	//    println($rs->a);
	//}
	//// 头 nil
	//// 生成唯一
	//
	//`
	//
	//		test.CheckPrintlnValue(code, []string{}, t)
	//	})
}
