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
func TestConstructorDataFlow(t *testing.T) {
	t.Run("constructor", func(t *testing.T) {
		code := `<?php
$a = new AA(1);
println($a->a);
`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"Undefined-AA", "Undefined-AA", "1", "Undefined-AA.AA-destructor", "make(any)"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("have constructor", func(t *testing.T) {
		code := `<?php
class A{
	public function __construct(){}
}
$a = new A();
$a->bb();
`
		ssatest.CheckSyntaxFlow(t, code, `
A() as $output
$output -> as $sink
`, map[string][]string{
			"output": {"Function-A(Undefined-A)"},
			"sink":   {"Undefined-$a.bb(Function-A(Undefined-A))", "Undefined-A.A-destructor(Function-A(Undefined-A))"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("no constructor", func(t *testing.T) {
		code := `<?php
$a = new A();
$a->bb();
`
		ssatest.CheckSyntaxFlow(t, code, `
A() as $output
$output -> as $sink
`, map[string][]string{
			"output": {"Undefined-A(Undefined-A)"},
			"sink":   {"Undefined-$a.bb(Undefined-A(Undefined-A))", "Undefined-A.A-destructor(Undefined-A(Undefined-A))"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
}
