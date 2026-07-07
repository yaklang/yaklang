package php

import (
	_ "embed"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed phpcode/webshell/ants.php
var antsShell string

func TestAntsShell(t *testing.T) {
	ssatest.CheckSyntaxFlowPrintWithPhp(t, antsShell, []string{"Function-array", `"_POST"`})
}
