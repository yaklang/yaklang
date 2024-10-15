package php

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestStatic(t *testing.T) {
	code := `
<?php

class A{
    public static $a =1;
}
println(A::$a);
`
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
		"param": {"1"},
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
func TestConstructor(t *testing.T) {
	code := `<?php
$a = new AA(1);
println($a->a);
`
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
		"param": {"Undefined-AA-constructor", "Undefined-AA", "1"},
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
