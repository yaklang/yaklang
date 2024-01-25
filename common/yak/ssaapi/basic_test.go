package ssaapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestYaklangBasic_variable(t *testing.T) {
	prog, err := Parse(`
	a = 1
	f = (a) => {
	}
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Ref("a").ShowWithSource()
}

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

func TestYaklangBasic_SinglePhi(t *testing.T) {
	const code = `for i:=0; i<n; i ++ { dump(i) }`
	prog, err := Parse(code)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	phi, ok := prog.GetValueByIdMust(9).node.(*ssa.Phi)
	if ok {
		log.Infof("phi: %v", phi.String())
	}
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
		if _, ok := value.node.(*ssa.Phi); ok {
			value.GetUsers().ForEach(func(value *Value) {
				if _, ok := value.node.(*ssa.Call); ok {
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

func MustParse(code string, t *testing.T) *Program {
	prog, err := Parse(code)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	return prog
}

func TestYaklangBasic_Foreach(t *testing.T) {
	t.Run("for each with chan", func(t *testing.T) {
		test := assert.New(t)
		prog := MustParse(`
		ch = make(chan int)

		for i in ch { 
			_ = i 
		}
		`, t)
		prog.Show()

		vs := prog.Ref("i")
		test.Equal(1, len(vs))

		v := vs[0]
		test.NotNil(v)

		kind := v.GetTypeKind()
		log.Info("type kind", kind)
		test.Equal(kind, ssa.Number)
	})
}

func TestYaklangParameter(t *testing.T) {
	t.Run("test parameter used", func(t *testing.T) {
		test := assert.New(t)
		prog := MustParse(`
		f = (a) => {
			return a
		}
		`, t)
		as := prog.Ref("a").ShowWithSource()
		test.Equal(1, len(as))
		test.Equal("a", *as[0].GetRange().SourceCode)
	})

	t.Run("test parameter not used", func(t *testing.T) {
		test := assert.New(t)
		prog := MustParse(`
		f = (a) => {
			return 1
		}
		`, t)
		as := prog.Ref("a").ShowWithSource()
		test.Equal(1, len(as))
		test.Equal("a", *as[0].GetRange().SourceCode)
	})

	t.Run("test free value used", func(t *testing.T) {
		test := assert.New(t)
		prog := MustParse(`
		f = () => {
			return a
		}
		`, t)
		as := prog.Ref("a").ShowWithSource()
		test.Equal(1, len(as))
		test.Equal("a", *as[0].GetRange().SourceCode)
	})
}
func TestExternLibInClosure(t *testing.T) {
	test := assert.New(t)
	prog, err := Parse(`
	a = () => {
		lib.method()
	}
	`,
		WithExternLib("lib", map[string]any{
			"method": func() {},
		}),
	)
	test.Nil(err)
	libVariables := prog.Ref("lib").ShowWithSource()
	// TODO: handler this
	// test.Equal(1, len(libVariables))
	test.NotEqual(0, len(libVariables))
	libVariable := libVariables[0]

	test.False(libVariable.IsParameter())
	test.True(libVariable.IsExtern())
}
