package php

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestPhpWeakLanguage(t *testing.T) {
	t.Run("const function call", func(t *testing.T) {
		code := `<?php

function A($a){
    println($a);
}
$a = "A";
$a(1);`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
}
