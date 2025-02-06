package ssaapi_test

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestYaklangBasic_Const(t *testing.T) {
	code := `
	a = 1
	b = a + 1 
	println(b)
	`
	ssatest.CheckSyntaxFlow(t, code, `b as $b`, map[string][]string{
		"b": {"2"},
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

func TestYaklangBasic_RecursivePhi_1(t *testing.T) {
	const code = `
count = 100

b = 1
a = (ffff) => {
	b ++
	if b > 100 {
		return
	}
	for i = 0; i < b; i ++ {
		dump(b)
	}
	c = () => { d = a(b); sink(d) }
	c()
}
e = a(1)
`
	prog, err := ssaapi.Parse(code)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
	prog.Ref("a")
}

func TestYaklangBasic_RecursivePhi_2(t *testing.T) {
	const code = `
count = 100

a = (b) => {
	b ++
	if b > 100 {
		return
	}
	for i = 0; i < b; i ++ {
		b := virtual(i, b)
		dump(b)
	}
	a(b)
}
e = a(1)          // e
f = a(v2(e))      // f
`
	prog, err := ssaapi.Parse(code)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
	prog.Ref("a")
}

func TestYaklangBasic_DoublePhi(t *testing.T) {
	const code = `var a = 1; for i:=0; i<n; i ++ { a += i }; println(a)`
	prog, err := ssaapi.Parse(code)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
	prog.Ref("a")
}

func TestYaklangBasic_Used(t *testing.T) {
	token := utils.RandStringBytes(10)
	code := fmt.Sprintf(`
	var a, b 
	%s(a)
	`, token)
	ssatest.CheckSyntaxFlowContain(t, code, `
a -> as $a_top
`, map[string][]string{
		"a_top": {token},
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

func TestYaklangBasic_if_phi(t *testing.T) {
	// prog, err := Parse(
	code := `
var a, b

dump(a)

if cond {
	a = a + b
} else {
	c := 1 + b 
}
println(a)
`
	ssatest.CheckSyntaxFlowContain(t, code, `
a -> ?{opcode:phi}  as $phi
$phi -> ?{opcode:call}  as $call
	`, map[string][]string{
		"call": {"println"},
	}, ssaapi.WithLanguage(ssaapi.Yak))
}
