package php

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestParseSSA_Eval1(t *testing.T) {
	code := `<?php
$key = "password";
$fun = base64_decode($_GET['func']);
$c=$a.$s.$_GET["func2"];
$c($fun);
`
	_ = code
	ssatest.CheckSyntaxFlow(t, code,
		"_GET.* -{until: `* ?{opcode:call} as $sink`}-> *",
		map[string][]string{"sink": {"Function-base64_decode(Undefined-_GET.func(valid))", "add(add(Undefined-$a, Undefined-$s), Undefined-_GET.func2(valid))(Function-base64_decode(Undefined-_GET.func(valid)))"}},
		ssaapi.WithLanguage(ssaapi.PHP),
	)
}

func TestNativeCallFilename(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("a.php", `<?php
	phpinfo();
`)
	fs.AddFile("b.php", `<?php
println("1");
`)
	ssatest.CheckSyntaxFlowWithFS(t, fs, `
println(* as $param)
$param<FilenameByContent> as $output`, map[string][]string{
		"output": {`"b.php"`},
	}, false, ssaapi.WithLanguage(ssaapi.PHP))
}
