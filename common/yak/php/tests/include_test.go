package tests

import (
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestInclude(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("var/www/html/1.php", `<?php
$a = 1;
$b = $a.$f;
`)
	fs.AddFile("var/www/html/2.php", `<?php
include("1.php");
println($a);
`)
	fs.AddFile("var/www/html/3.php", `<?php
	class test{
	public $a=1;
}
	$c = new test;
`)
	ssatest.CheckSyntaxFlowWithFS(t, fs,
		`println(* #-> * as $param)`,
		map[string][]string{"param": {"1"}},
		false,
		ssaapi.WithLanguage(ssaapi.PHP))
}
