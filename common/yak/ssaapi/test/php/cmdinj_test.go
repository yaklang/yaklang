package php

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

func mustCompile(raw ...string) []*ssaapi.Program {
	fs := filesys.NewVirtualFs()
	for i := 0; i < len(raw); i++ {
		var suffix = ""
		if i > 0 {
			suffix = fmt.Sprint(i)
		}
		fs.AddFile("index"+suffix+".php", raw[0])
	}
	programs, err := ssaapi.ParseProject(fs, ssaapi.WithFileSystemEntry("index.php"), ssaapi.WithLanguage(ssaapi.PHP))
	if err != nil {
		panic(err)
	}
	if len(programs) <= 0 {
		panic("ssaapi.ParseProject error, programs is empty")
	}
	return programs
}

func mustCompileFirst(raw ...string) *ssaapi.Program {
	return mustCompile(raw...)[0]
}

func TestPHP_CMDInj(t *testing.T) {
	code := `<?php
$command = 'ping -c 1 '.$_GET['ip'];
system($command); //system函数特性 执行结果会自动打印
?>`
	pg := mustCompileFirst(code).Show()
	values, err := pg.SyntaxFlowWithError("system(-)")
	if err != nil {
		t.Fatal(err)
	}
	values.Show()
}
