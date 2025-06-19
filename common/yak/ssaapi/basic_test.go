package ssaapi

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

func TestYaklangBasic_Const(t *testing.T) {
	code := `
	a = 1
	b = a + 1 
	println(b)
	`

	prog, err := Parse(code)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	// prog.Ref("a").Show()
	prog.Ref("b").Show().ForEach(func(v *Value) {
		if len(v.GetOperands().Show()) != 2 {
			t.Fatalf("const 2 should have 2 operands")
		}
	})
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
	prog, err := Parse(code)
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
	prog, err := Parse(code)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
	prog.Ref("a")
}

func TestYaklangBasic_DoublePhi(t *testing.T) {
	const code = `var a = 1; for i:=0; i<n; i ++ { a += i }; println(a)`
	prog, err := Parse(code)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
	prog.Ref("a")
}

func TestYaklangBasic_Used(t *testing.T) {
	token := utils.RandStringBytes(10)
	prog, err := Parse(`var a, b
` + token + `(a)
`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	traceFinished := false
	prog.Ref("a").ForEach(func(value *Value) {
		value.GetUsers().ForEach(func(value *Value) {
			log.Infof("a's uses include: %v", value.String())
			if strings.Contains(value.String(), token+"(") {
				traceFinished = true
			}
		})
	})
	if !traceFinished {
		t.Error("trace failed: var cannot trace to call actual arguments")
	}
}

func TestYaklangBasic_if_phi(t *testing.T) {
	prog, err := Parse(`var a, b

dump(a)

if cond {
	a = a + b
} else {
	c := 1 + b 
}
println(a)
`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	var traceToCall_via_if bool
	prog.Ref("a").ForEach(func(value *Value) {
		if value.IsPhi() {
			value.GetUsers().ForEach(func(value *Value) {
				if value.IsCall() {
					traceToCall_via_if = true
					log.Infof("a's deep uses include: %v", value.String())
				}
			})
		}
	})
	if !traceToCall_via_if {
		t.Error("trace failed: var cannot trace to call actual arguments")
	}
}

func TestYaklangBasic(t *testing.T) {
	prog, err := Parse(`
a = 1
a = 2
a = 3
b = a
`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	_ = prog
}
