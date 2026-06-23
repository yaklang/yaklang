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

func TestDynamicVariableForeachTopDef(t *testing.T) {
	code := `<?php
foreach (array('_POST') as $_request) {
    foreach ($$_request as $_key => $_value) {
        println($_key);
    }
}
`
	ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"Function-array", `"_POST"`})
}
