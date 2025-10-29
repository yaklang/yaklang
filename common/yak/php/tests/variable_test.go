package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestVariable(t *testing.T) {
	t.Run("test variable", func(t *testing.T) {
		code := `
		<?php
		$a = 1 ;
		$b = $a + 22 ;
		`
		ssatest.CheckSyntaxFlow(t, code, `a as $a`, map[string][]string{
			"a": {"1"},
		}, ssaapi.WithLanguage(ssaconfig.PHP),
		)
	})

	t.Run("test function", func(t *testing.T) {
		code := `
		<?php
		$a = 1;
		echo($a);
		`
		ssatest.CheckSyntaxFlow(t, code, `echo as $echo`, map[string][]string{
			"echo": {"Function-echo"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})

	t.Run("only use pkgName,use blueprint static", func(t *testing.T) {
		code := `<?php
namespace{
    println(11);
}`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param) as $println`, map[string][]string{
			"param":   {"11"},
			"println": {"Function-println(11)"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}
