package tests

import (
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func Test_Include(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/1.php", `
	<?php
		$a = 1;
	`)
	vf.AddFile("src/2.php", `
	include("1.php");
	<?php
		println($a);
	`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `println(* #-> * as $param)`, map[string][]string{}, false, ssaapi.WithLanguage(ssaapi.PHP))
}
