package tests

import (
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestInclude(t *testing.T) {
	t.Run("custom include", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("var/www/html/1.php", `<?php
$a = 1;
$b = $a.$f;
`)
		fs.AddFile("var/www/html/2.php", `<?php
include("1.php");
println($a);
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"1"}},
			false,
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("include return", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("var/www/html/1.php", `<?php
$a = 1;
$b = $a.$f;
function test(){
	$a = 123;
	return $a;
}
return 1;
`)
		fs.AddFile("var/www/html/2.php", `<?php
include("1.php");
println($a);
$a = test();
println($a);
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"1", "123"}},
			false,
			ssaapi.WithLanguage(ssaapi.PHP))
	})
}
