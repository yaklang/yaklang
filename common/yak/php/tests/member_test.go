package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"

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
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("array assign", func(t *testing.T) {
		code := `<?php
$files[] = array(
    'url' => substr($path2, a),
    'mtime' => filemtime($path2)
);
println($files);
`
		test.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{"param": {"make(any)", "Undefined-substr", "Undefined-$path2", "Undefined-a", "Undefined-filemtime"}}, ssaapi.WithLanguage(ssaapi.PHP))
	})
}
