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
		require.Greater(t, res.GetValues("guards").Count(), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

