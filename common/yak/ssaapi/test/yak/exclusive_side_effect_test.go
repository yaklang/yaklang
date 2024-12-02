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
	// 平级b()继承平级a()的side-effect
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
	ssatest.CheckWithNameOnlyInMemory("side-effect: a->b", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("a as $a").GetValues("a")
		b := prog.SyntaxFlow("b as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype1, ok := ssa.ToFunctionType(fun1.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype2, ok := ssa.ToFunctionType(fun2.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype2.SideEffects))
		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

func Test_SideEffect_Double_lower(t *testing.T) {
	// 外部b()继承内部a()的side-effect
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
	ssatest.CheckWithNameOnlyInMemory("side-effect: a->b", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("a as $a").GetValues("a")
		b := prog.SyntaxFlow("b as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype1, ok := ssa.ToFunctionType(fun1.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype2, ok := ssa.ToFunctionType(fun2.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype2.SideEffects))
		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

func Test_SideEffect_Double_more(t *testing.T) {
	// 内部a()继承外部b()的side-effect
	code := `
n = 1
b=()=>{
	n = 2 // modify
	a=()=>{
        b()
	}
}
`
	ssatest.CheckWithNameOnlyInMemory("side-effect: b->a", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("a as $a").GetValues("a")
		b := prog.SyntaxFlow("b as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype1, ok := ssa.ToFunctionType(fun1.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype2, ok := ssa.ToFunctionType(fun2.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype2.SideEffects))
		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

/* TODO: 继承外部side-effect的情况下，外部的scope可能没有执行完毕 */
func Test_SideEffect_Double_moreEx(t *testing.T) {
	t.Skip()
	// 内部a()继承外部b()的side-effect
	code := `
n = 1
b=()=>{
	a=()=>{
        b()
	}
	n = 2 // modify
}
`
	ssatest.CheckWithNameOnlyInMemory("side-effect: b->a", t, code, func(prog *ssaapi.Program) error {
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

func Test_SideEffect_Double_lower_exclude(t *testing.T) {
	// 外部f1()继承内部f2()的side-effect,但在当前函数作用域之内的变量不会继承
	code := `
b = 1  
f1=()=>{  
    a = 1
    f2=()=>{ 
		b = 2 // side-effect f2(b) 
		a = 3 // side-effect f2(a)
	}  
    f2() // call-side: f1 will append f2(b), but not f2(a)
}  
`
	ssatest.CheckWithNameOnlyInMemory("side-effect: f2->f1", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("f1 as $a").GetValues("a")
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype1, ok := ssa.ToFunctionType(fun1.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype2, ok := ssa.ToFunctionType(fun2.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 2, len(funtype2.SideEffects))
		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

func Test_SideEffect_panic(t *testing.T) {
	code := `
mirrorNewWebsitePath = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
    newRisk = (title,payload,param,levelIndex)=>{
        rsp = param.rsp
    }
    
    checkPayloadsByVersionScope = (param)=>{
		for exp in exps{
			exp = exp[0]
			rsp,err = param.delayFuzz(exp)
			newRisk("目标 fastjson 框架可能存在 RCE 漏洞 (DNSLog Check)",exp,param,2)
			break
		}
    }
}
`
	ssatest.CheckWithName("link-side-effect cannot participate in generating phi", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}
