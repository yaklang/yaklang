package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_SideEffect(t *testing.T) {
	t.Run("normal side-effect", func(t *testing.T) {
		code := `
a = 1
b = () => {
	a = 2
}
b()
c = a;
`
		ssatest.Check(t, code,
			ssatest.CheckTopDef_Contain("c", []string{"2"}, true),
		)
	})

	t.Run("phi side-effect", func(t *testing.T) {
		ssatest.Check(t, `
a = 1
b = () => {
	a = 2
}
if e {b()}
c = a;
		`, ssatest.CheckTopDef_Contain("c", []string{"2", "1", "Undefined-e"}, true))
	})

	t.Run("if-else phi side-effect", func(t *testing.T) {
		ssatest.Check(t, `
		d = "kkk"
		ok = foo("ooo", d)
		a= 1 
		if ok{
			a= 1
		}else{
			a = 2
		}
		b = a

		`, ssatest.CheckTopDef_Contain("b", []string{
			"1",
			"2",
			"Undefined-foo",
			"ooo",
			"kkk",
		}, true))
	})

	t.Run("simple if else-if phi side-effect ", func(t *testing.T) {
		ssatest.Check(t, `
		a = 3
		if c{
			a= 1
		}else if d{
			a = 2
		}
		b = a

		`, ssatest.CheckTopDef_Contain("b", []string{
			"1",
			"2",
			"3",
			"Undefined-c",
			"Undefined-d",
		}, true))
	})

	t.Run("complex if else-if phi side-effect", func(t *testing.T) {
		ssatest.Check(t, `
		a = 1

		ok = false
		if e {
			ok = true
		}else{
			ok = false
		}

		if c{
			a= 11
		}else if ok{
			a = 111
		}
		b = a

		`, ssatest.CheckTopDef_Contain("b", []string{
			"1",
			"111",
			"11",
			"Undefined-c",
			"true",
			"false",
			"Undefined-e",
		}, true))
	})
}

func Test_SideEffect_Double(t *testing.T) {
	// 平级 b() 继承平级 a() 的 side-effect
	code := `
n = 1
a=()=>{
    n = 2 // modify
}
b=()=>{
    a()
}
b()
`
	ssatest.CheckWithName("side-effect: a->b", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("a as $a").GetValues("a")
		b := prog.SyntaxFlow("b as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun2.SideEffects))

		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

func Test_SideEffect_Double_lower(t *testing.T) {
	// 外部 b() 继承内部 a() 的 side-effect
	code := `
n = 1
b=()=>{
	a=()=>{
		n = 2 // modify
	}
	a()
}
b()
`
	ssatest.CheckWithName("side-effect: a->b", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("a as $a").GetValues("a")
		b := prog.SyntaxFlow("b as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun2.SideEffects))

		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

func Test_SideEffect_Double_more(t *testing.T) {
	// 内部 b() 继承外部 a() 的 side-effect
	code := `
n = 1
b=()=>{
	n = 2 // modify
	a=()=>{
        b()
	}
}
`
	ssatest.CheckWithName("side-effect: b->a", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("a as $a").GetValues("a")
		b := prog.SyntaxFlow("b as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun2.SideEffects))

		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

/* TODO: 继承外部side-effect的情况下，外部的scope可能没有执行完毕 */
func Test_SideEffect_Double_moreEx(t *testing.T) {
	t.Skip()
	// 内部 b() 继承外部 a() 的 side-effect
	code := `
n = 1
b=()=>{
	a=()=>{
        b()
	}
	n = 2 // modify
}
`
	ssatest.CheckWithName("side-effect: b->a", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("a as $a").GetValues("a")
		b := prog.SyntaxFlow("b as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun2.SideEffects))

		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}
