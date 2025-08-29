package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
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
		ssatest.CheckTopDef(t, code, "c", []string{"2"}, true)
	})

	t.Run("phi side-effect", func(t *testing.T) {
		ssatest.CheckTopDef(t, `
a = 1
b = () => {
	a = 2
}
if e {b()}
c = a;
		`, "c", []string{"2", "1", "Undefined-e"}, true)
	})

	t.Run("if-else phi side-effect", func(t *testing.T) {
		code := `
		d = "kkk"
		ok = foo("ooo", d)
		a= 1 
		if ok{
			a= 1
		}else{
			a = 2
		}
		b = a

		`
		ssatest.CheckTopDef(t, code, "b", []string{
			"1",
			"2",
			"Undefined-foo",
			"ooo",
			"kkk",
		}, true)
	})

	t.Run("simple if else-if phi side-effect ", func(t *testing.T) {
		ssatest.CheckTopDef(t, `
		a = 3
		if c{
			a= 1
		}else if d{
			a = 2
		}
		b = a

		`, "b", []string{
			"1",
			"2",
			"3",
			"Undefined-c",
			"Undefined-d",
		}, true)
	})

	t.Run("complex if else-if phi side-effect", func(t *testing.T) {
		ssatest.CheckTopDef(t, `
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

		`, "b", []string{
			"1",
			"111",
			"11",
			"Undefined-c",
			"true",
			"false",
			"Undefined-e",
		}, true)
	})

	t.Run("side-effect without bind", func(t *testing.T) {
		code := `
n = 1
b=()=>{
	n = 2 // modify
}
{
	var n = 3
	b()
	println(n)
}
println(n)
`
		ssatest.CheckPrintlnValue(code, []string{"1", "side-effect(2, n)"}, t)
	})
}
func checkSideeffect(values ssaapi.Values, num int) error {
	have := false
	for _, value := range values {
		fun1, ok := ssa.ToFunction(value.GetSSAInst())
		if !ok {
			continue
		}
		have = true
		funtype1, ok := ssa.ToFunctionType(fun1.Type)
		if !ok {
			return utils.Errorf("BUG::value is function but type not function type ")
		}
		if num != len(funtype1.SideEffects) {
			return utils.Errorf("side effect num not match, want %d, got %d", num, len(funtype1.SideEffects))
		}
	}
	if !have {
		return utils.Errorf("no function found ")
	} else {
		return nil
	}
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
		require.NoError(t, checkSideeffect(a, 1))
		b := prog.SyntaxFlow("b as $b").GetValues("b")
		require.NoError(t, checkSideeffect(b, 1))
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
		require.NoError(t, checkSideeffect(a, 1))
		b := prog.SyntaxFlow("b as $b").GetValues("b")
		require.NoError(t, checkSideeffect(b, 1))
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
		require.NoError(t, checkSideeffect(a, 1))
		b := prog.SyntaxFlow("b as $b").GetValues("b")
		require.NoError(t, checkSideeffect(b, 1))
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
		require.NoError(t, checkSideeffect(a, 1))
		b := prog.SyntaxFlow("b as $b").GetValues("b")
		require.NoError(t, checkSideeffect(b, 1))
		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

func Test_SideEffect_Double_lower_exclude(t *testing.T) {
	// 外部f1()继承内部f2()的side-effect,但在当前函数作用域之內的變量不會繼承
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
		require.NoError(t, checkSideeffect(a, 1))
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")
		require.NoError(t, checkSideeffect(b, 2))
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
