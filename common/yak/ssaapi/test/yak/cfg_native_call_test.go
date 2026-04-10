package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestNativeCall_GetCFG_BlockInstInfo(t *testing.T) {
	code := `
c = 1
if (c) {
    x = "a"
} else {
    x = "b"
}
println(x)
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
println(* #-> as $arg)
$arg<getCfg> as $argCfg
$argCfg<cfgBlock> as $blk
$argCfg<cfgInst> as $ins
`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("blk").String())
		require.NotEmpty(t, res.GetValues("ins").String())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

func TestNativeCall_CFGDominatesReachable(t *testing.T) {
	code := `
c = 1
if (c) {
    x = "a"
} else {
    x = "b"
}
println(x)
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
c as $cond
println(* #-> as $arg)
$cond<getCfg> as $condCfg
$arg<getCfg> as $argCfg
$condCfg<cfgDominates(target="$argCfg")> as $dom
$condCfg<cfgReachable(target="$argCfg")> as $reach
`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		require.Contains(t, res.GetValues("dom").String(), "true")
		require.Contains(t, res.GetValues("reach").String(), "true")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

func TestNativeCall_CFGGuards(t *testing.T) {
	code := `
a = 1
if (a) {
    return
}
println("ok")
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
println(* #-> as $arg)
$arg<getCfg> as $sinkCfg
$sinkCfg<cfgGuards> as $guards
`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		require.Contains(t, res.String(), "guard(kind=earlyReturn")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

func TestNativeCall_CFGGuards_GetFields_Filter(t *testing.T) {
	code := `
a = 1
if (a) {
    return
}
println("ok")
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
println(* #-> as $arg)
$arg<getCfg> as $sinkCfg
$sinkCfg<cfgGuards> as $guards

$guards.kind as $kind
$kind?{have: "earlyReturn"} as $hit
`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		require.Greater(t, res.GetValueCount("hit"), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

func TestNativeCall_CFGReachable_ICFG_CallIntoCallee(t *testing.T) {
	code := `
func foo() {
    println("infoo")
}

foo()
println("after")
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
println(*?{have: "after"} #-> as $afterArg)
println(*?{have: "infoo"} #-> as $infooArg)

$afterArg<getCfg> as $callerCfg
$infooArg<getCfg> as $calleeCfg

$callerCfg<cfgReachable(target="$calleeCfg", icfg=true)> as $reach
`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		require.Contains(t, res.GetValues("reach").String(), "true")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

func TestNativeCall_CFGReachable_IntraProc_Default(t *testing.T) {
	code := `
func foo() {
    println("infoo")
}

foo()
println("after")
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
println(*?{have: "after"} #-> as $afterArg)
println(*?{have: "infoo"} #-> as $infooArg)

$afterArg<getCfg> as $callerCfg
$infooArg<getCfg> as $calleeCfg

$callerCfg<cfgReachable(target="$calleeCfg")> as $reach
`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		require.Contains(t, res.GetValues("reach").String(), "false")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}
