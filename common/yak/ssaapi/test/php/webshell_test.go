package php

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

//go:embed phpcode/webshell/ants.php
var antsShell string

func TestAntsShell(t *testing.T) {
	ssatest.CheckSyntaxFlowPrintWithPhp(t, antsShell, []string{"Function-array", `"_POST"`})
}
