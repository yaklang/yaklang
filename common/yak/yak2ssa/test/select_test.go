package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSelectSSA(t *testing.T) {
	for _, source := range []string{
		`ch=make(chan int,1); select { case v, ok := <-ch: println(v); println(ok); default: println(0) }`,
		`ch=make(chan int,1); v=0; select { case v = <-ch: println(v); case ch <- 2: println(2) }; println(v)`,
		`ch=make(chan int,1); select { default: println(0); case <-ch: println(1) }`,
		`select { default: break }; println(1)`,
		`for i in 3 { select { default: continue }; println(i) }`,
		`ch=make(chan int); select { default: select {default: break}; switch <-ch {case 1: fallthrough; default: println(1)} }`,
		`f=()=>{ ch=make(chan int); select {case v:=<-ch: return v; default: return 0} }; println(f())`,
		`select {}`,
		`select=1; select
{ println(select) }; obj={"select":()=>2}; println(obj.select())`,
	} {
		t.Run(source, func(t *testing.T) { test.CheckNoError(t, source) })
	}
}

func TestSelectSSARejectsInvalidCases(t *testing.T) {
	for _, source := range []string{`select {case 1:}`, `select {default: default:}`, `select {case a,b,c:=<-ch:}`, `select {case a.x:=<-ch:}`, `select {default: fallthrough}`, `select {default: if true {fallthrough}}`, `select {case v:=1:}`, `select {case <-1:}`} {
		t.Run(source, func(t *testing.T) {
			p, err := ssaapi.Parse(source)
			require.NoError(t, err)
			found := false
			for _, diagnostic := range p.GetErrors() {
				if diagnostic.Kind == ssa.Error && strings.Contains(diagnostic.Message, "select") {
					found = true
				}
			}
			require.True(t, found, "missing select diagnostic: %s", p.GetErrors())
		})
	}
}

func TestSelectSSACaseScopeAndJoin(t *testing.T) {
	test.CheckPrintlnValue(`
ch=make(chan int)
v=9
select {case v:=<-ch: println(v); default: println(0)}
println(v)
`, []string{"castType(number, Undefined-.1(valid))", "0", "9"}, t)
	test.CheckPrintlnValue(`
ch=make(chan int)
x=0
select {case <-ch: x=1; default: x=2}
println(x)
`, []string{"phi(x)[1,2]"}, t)
}

func TestSelectSSAReceiveTypesAndOperandDependencies(t *testing.T) {
	p, err := ssaapi.Parse(`ch=make(chan string); out=make(chan string); payload="tainted"; select {case v,ok:=<-ch: println(v); println(ok); case out<-payload: println(payload)}`)
	require.NoError(t, err)
	for _, tc := range []struct {
		name string
		kind ssa.TypeKind
	}{{"v", ssa.StringTypeKind}, {"ok", ssa.BooleanTypeKind}} {
		values := p.Ref(tc.name)
		require.NotEmpty(t, values)
		for _, value := range values {
			require.Equal(t, tc.kind, value.GetTypeKind())
		}
	}
	calls := p.Ref("ch").GetUsers().Filter(func(value *ssaapi.Value) bool { return value.IsCall() })
	require.Len(t, calls, 1, "all operands must belong to one select call")
	require.Contains(t, calls[0].GetOperand(0).String(), yakvm.SelectBuiltinName)
	args := calls[0].GetOperands()
	require.True(t, strings.Contains(args.String(), "tainted"), "send payload missing from dataflow: %s", args)
	// An exhaustive no-default select has no unchanged-value join edge either.
	test.CheckPrintlnValue(`ch=make(chan int); out=make(chan int); x=0; select {case <-ch: x=1; case out<-2: x=2}; println(x)`, []string{"phi(x)[1,2]"}, t)
}
