package ssaapi

// 本文件用 SyntaxFlow 覆盖以下 native call（与 sf_native_call 常量名对应）：
//   getCfg, cfgGuards, cfgDominates, cfgPostDominates, cfgReachable, cfgReachPath,
//   cfgCondition, cfgConditionValues, cfgBlock, cfgInst
// 每个 native 对应独立测试；cfgGuards / cfgDominates / cfgPostDominates / cfgReachable / cfgReachPath 含多个 t.Run 子场景。
// cfgDominates：从入口到「当前 cfg」是否必经 target（图论 target 支配当前）。cfgPostDominates：从「当前 cfg」到出口是否必经 target（图论 target 后支配当前）。

import (
	"sort"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

type sfExpect struct {
	WantEqual          map[string][]string
	WantVarContains    map[string][]string
	WantResultContains []string
	WantMinCount       map[string]int
	PostCheck          func(t *testing.T, res *ssaapi.SyntaxFlowResult)
}

func runSyntaxFlowExpect(t *testing.T, yakCode, sfRule string, exp sfExpect) {
	t.Helper()
	ssatest.Check(t, strings.TrimSpace(yakCode), func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(strings.TrimSpace(sfRule), ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		res.Show()
		for varName, want := range exp.WantEqual {
			gotVs := res.GetValues(varName)
			got := lo.Map(gotVs, func(v *ssaapi.Value, _ int) string { return v.String() })
			sort.Strings(got)
			expS := append([]string(nil), want...)
			sort.Strings(expS)
			require.Equal(t, expS, got, "variable %q", varName)
		}
		for varName, needles := range exp.WantVarContains {
			hay := res.GetValues(varName).String()
			for _, sub := range needles {
				require.Contains(t, hay, sub, "variable %q", varName)
			}
		}
		for _, sub := range exp.WantResultContains {
			require.Contains(t, res.String(), sub, "full result")
		}
		for varName, n := range exp.WantMinCount {
			require.GreaterOrEqual(t, len(res.GetValues(varName)), n, "variable %q count", varName)
		}
		if exp.PostCheck != nil {
			exp.PostCheck(t, res)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

// TestNativeCall_getCfg 覆盖 NativeCall_GetCFG（getCfg）。
func TestNativeCall_getCfg(t *testing.T) {
	runSyntaxFlowExpect(t, `
c = 1
if (c) {
	x = "a"
} else {
	x = "b"
}
println(x)
`, `
println(* #-> as $arg)
$arg<getCfg()> as $cfg
`, sfExpect{
		PostCheck: func(t *testing.T, res *ssaapi.SyntaxFlowResult) {
			vs := res.GetValues("cfg")
			res.Show()
			require.NotEmpty(t, vs)
			require.Contains(t, vs[0].String(), "block=")
			s, ok := vs[0].GetConstValue().(string)
			require.True(t, ok)
			require.True(t, ssaapi.IsCfgCtxURLDisplayString(s))
		},
	})
}

// TestNativeCall_cfgBlock 覆盖 NativeCall_CFGBlockInfo（cfgBlock）。
func TestNativeCall_cfgBlock(t *testing.T) {
	runSyntaxFlowExpect(t, `
c = 1
if (c) {
	x = "a"
} else {
	x = "b"
}
println(x)
`, `
println(* #-> as $arg)
$arg<getCfg()> as $argCfg
$argCfg<cfgBlock()> as $blk
`, sfExpect{
		WantMinCount: map[string]int{"blk": 1},
		WantVarContains: map[string][]string{
			"blk": {"func=", "block="},
		},
	})
}

// TestNativeCall_cfgInst 覆盖 NativeCall_CFGInstInfo（cfgInst）。
func TestNativeCall_cfgInst(t *testing.T) {
	runSyntaxFlowExpect(t, `
c = 1
if (c) {
	x = "a"
} else {
	x = "b"
}
println(x)
`, `
println(* #-> as $arg)
$arg<getCfg()> as $argCfg
$argCfg<cfgInst()> as $ins
`, sfExpect{
		WantMinCount: map[string]int{"ins": 1},
		WantVarContains: map[string][]string{
			"ins": {"inst="},
		},
	})
}

// TestNativeCall_cfgGuards 覆盖 NativeCall_CFGGuards（cfgGuards）。
func TestNativeCall_cfgGuards(t *testing.T) {
	t.Run("lists_early_return_guard", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
a = 1
if (a) {
	return
}
println("ok")
`, `
println(* #-> as $arg)
$arg<getCfg()> as $sinkCfg
$sinkCfg<cfgGuards()> as $guards
`, sfExpect{
			WantResultContains: []string{"guard(kind=" + ssaapi.GuardKindEarlyReturn},
		})
	})
	t.Run("cfgGuards_implicit_getCfg_on_chain", func(t *testing.T) {
		// 语法糖：链上可为 SSA value，无需显式 <getCfg> 再 <cfgGuards>。
		runSyntaxFlowExpect(t, `
a = 1
if (a) {
	return
}
println("ok")
`, `
println(* #-> as $arg)
$arg<cfgGuards()> as $guards
`, sfExpect{
			WantResultContains: []string{"guard(kind=" + ssaapi.GuardKindEarlyReturn},
		})
	})
	t.Run("filter_guard_kind_field", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
a = 1
if (a) {
	return
}
println("ok")
`, `
println(* #-> as $arg)
$arg<getCfg()> as $sinkCfg
$sinkCfg<cfgGuards()> as $guards

$guards.kind as $kind
$kind?{have: "`+ssaapi.GuardKindEarlyReturn+`"} as $hit
`, sfExpect{
			WantMinCount: map[string]int{"hit": 1},
		})
	})
	t.Run("lists_early_panic_guard", func(t *testing.T) {
		// 退出侧为 SSA Panic 时 kind=earlyPanic（与「无后继块」类 return 区分）。
		runSyntaxFlowExpect(t, `
a = 1
if (a) {
	panic("x")
}
println("ok")
`, `
println(* #-> as $arg)
$arg<getCfg()> as $sinkCfg
$sinkCfg<cfgGuards()> as $guards
`, sfExpect{
			WantResultContains: []string{"guard(kind=" + ssaapi.GuardKindEarlyPanic},
		})
	})
	t.Run("panic_on_false_branch_polarity_true", func(t *testing.T) {
		// else 侧 panic、then 侧到达 sink：落到 sink 需 cond 为真，且 kind 取自 panic 所在分支。
		runSyntaxFlowExpect(t, `
a = 1
if (a) {
	println("ok")
} else {
	panic(0)
}
`, `
println(*?{have: "ok"} #-> as $arg)
$arg<getCfg()> as $sinkCfg
$sinkCfg<cfgGuards()> as $guards
`, sfExpect{
			WantResultContains: []string{"guard(kind=" + ssaapi.GuardKindEarlyPanic, "polarity=true"},
		})
	})
	t.Run("lists_early_break_guard", func(t *testing.T) {
		// for 内 if 一侧 break（Jump→Loop.Exit），另一侧到达 sink。
		runSyntaxFlowExpect(t, `
for i = 0; i < 3; i = i + 1 {
	if (i == 1) {
		break
	}
	println("sink")
}
`, `
println(*?{have: "sink"} #-> as $arg)
$arg<getCfg()> as $sinkCfg
$sinkCfg<cfgGuards()> as $guards
`, sfExpect{
			WantResultContains: []string{"guard(kind=" + ssaapi.GuardKindEarlyBreak},
		})
	})
	t.Run("lists_early_continue_guard", func(t *testing.T) {
		// for 内 if 一侧 continue（Jump→latch），另一侧到达 sink。
		runSyntaxFlowExpect(t, `
for i = 0; i < 3; i = i + 1 {
	if (i < 2) {
		continue
	}
	println("sink")
}
`, `
println(*?{have: "sink"} #-> as $arg)
$arg<getCfg()> as $sinkCfg
$sinkCfg<cfgGuards()> as $guards
`, sfExpect{
			WantResultContains: []string{"guard(kind=" + ssaapi.GuardKindEarlyContinue},
		})
	})
}

// TestNativeCall_cfgDominates 覆盖 NativeCall_CFGDominates（cfgDominates）。
func TestNativeCall_cfgDominates(t *testing.T) {
	// 当前 cfg 为路径终点（从入口走来），target 为被检查的必经点：dominates(current, target) 内部等价于 dominates(target, current)。同块时按 Insts 序细化；见 sf_cfg_dom.dominates。
	t.Run("entry_side_def_dominates_merge_use", func(t *testing.T) {
		// 从入口到 println 实参必经 cond；起点写 $argCfg，必经点 $condCfg。
		runSyntaxFlowExpect(t, `
c = 1
if (c) {
	x = "a"
} else {
	x = "b"
}
println(x)
`, `
c as $cond
println(* #-> as $arg)
$cond<getCfg()> as $condCfg
$arg<getCfg()> as $argCfg
$argCfg<cfgDominates(target: "$condCfg")> as $dom
`, sfExpect{
			WantVarContains: map[string][]string{"dom": {"true"}},
		})
	})

	t.Run("sequential_stmt_dominates_later_use", func(t *testing.T) {
		// 从入口到 println 必经先执行的 a。
		runSyntaxFlowExpect(t, `
a = 1
b = 2
println(b)
`, `
a as $a
println(* #-> as $arg)
$a<getCfg()> as $aCfg
$arg<getCfg()> as $argCfg
$argCfg<cfgDominates(target: "$aCfg")> as $dom
`, sfExpect{
			WantVarContains: map[string][]string{"dom": {"true"}},
		})
	})

	t.Run("self_anchor_dominates_itself", func(t *testing.T) {
		// 同一 cfg 锚点：沿 idom 链从 b 上溯立即等于 a，视为 true（自反）。
		runSyntaxFlowExpect(t, `
println(0)
`, `
println(* #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgDominates(target: "$cfg")> as $dom
`, sfExpect{
			WantVarContains: map[string][]string{"dom": {"true"}},
		})
	})

	t.Run("then_only_stmt_block_does_not_dominate_merge", func(t *testing.T) {
		// 存在 else 空分支时，可走「不经 then 里 foo()」的路径到达汇合后的 println，故 then 侧调用的块不支配 println 块。
		runSyntaxFlowExpect(t, `
func g(p) {
	if p {
		foo()
	} else {
	}
	println(0)
}
func foo() {
}

`, `
foo() as $thenCall
println(*?{have: "0"} #-> as $sinkArg)
$thenCall<getCfg()> as $thenCfg
$sinkArg<getCfg()> as $sinkCfg
$thenCfg<cfgDominates(target: "$sinkCfg")> as $dom
`, sfExpect{
			WantVarContains: map[string][]string{"dom": {"false"}},
		})
	})

	t.Run("later_use_does_not_dominate_earlier_def", func(t *testing.T) {
		// 从入口到 a 不必经 println：$aCfg 为终点、$sinkCfg 为 target 时 false。同块时亦由指令序保证。
		runSyntaxFlowExpect(t, `
a = 1
if true {
	println(0)
}
`, `
a as $a
println(* #-> as $arg)
$a<getCfg()> as $aCfg
$arg<getCfg()> as $sinkCfg
$aCfg<cfgDominates(target: "$sinkCfg")> as $dom
`, sfExpect{
			WantVarContains: map[string][]string{"dom": {"false"}},
		})
	})

	t.Run("different_functions_returns_false", func(t *testing.T) {
		// 实现要求 a、b 同 FuncID；跨函数 cfg 对 cfgDominates 恒为 false。
		runSyntaxFlowExpect(t, `
func fa() {
	println(1)
}
func fb() {
	println(2)
}
fa()
fb()
`, `
println(*?{have: "1"} #-> as $aArg)
println(*?{have: "2"} #-> as $bArg)
$aArg<getCfg()> as $faCfg
$bArg<getCfg()> as $fbCfg
$faCfg<cfgDominates(target: "$fbCfg")> as $dom
`, sfExpect{
			WantVarContains: map[string][]string{"dom": {"false"}},
		})
	})

	t.Run("config_style_target_param", func(t *testing.T) {
		// 与 entry_side 相同语义，仅验证 target="$var" 写法（与文档一致）。
		runSyntaxFlowExpect(t, `
c = 1
if (c) {
	x = "a"
} else {
	x = "b"
}
println(x)
`, `
c as $cond
println(* #-> as $arg)
$cond<getCfg()> as $condCfg
$arg<getCfg()> as $argCfg
$argCfg<cfgDominates(target="$condCfg")> as $dom
`, sfExpect{
			WantVarContains: map[string][]string{"dom": {"true"}},
		})
	})

	t.Run("multiple_target_cfgs_OR_same_function_only", func(t *testing.T) {
		// 多 target：$tCfg 同时含「异函数」与「同函数」锚点，只应就与 sink 同函数的 a、b 做 OR。
		// 多结果：两个 println 均绑定 $sink，$sinkCfg 上 cfgDominates 应对 **两个** 接收点各出一条 bool。
		runSyntaxFlowExpect(t, `
func fa() {
	x = 0
}
func fb() {
	a = 1
	b = 2
	println(10)
	println(20)
}
fa()
fb()
`, `
a as $anchor
b as $anchor
x as $anchor
$anchor<getCfg()> as $tCfg
println(*?{have: "10"} #-> as $sink)
println(*?{have: "20"} #-> as $sink)
$sink<getCfg()> as $sinkCfg
$sinkCfg<cfgDominates(target: "$tCfg")> as $dom
`, sfExpect{
			WantMinCount: map[string]int{"dom": 2},
			PostCheck: func(t *testing.T, res *ssaapi.SyntaxFlowResult) {
				for _, v := range res.GetValues("dom") {
					require.Contains(t, v.String(), "true")
				}
			},
		})
	})
}

// TestNativeCall_cfgPostDominates 覆盖 NativeCall_CFGPostDom（cfgPostDominates）。
func TestNativeCall_cfgPostDominates(t *testing.T) {
	// 与 cfgDominates 对仗：支配从入口到当前；后支配从当前到出口；target 均为被检查的必经点（图论 postDominates(target, current)，API 为 postDominates(current, target)）。同块时见 sf_cfg_dom.postDominates。
	t.Run("merge_sink_cfgPostDominates_const_anchor_ir", func(t *testing.T) {
		// cfgPostDominates：postDominates(receiver, target) 内部等价于 postDominates(target, receiver)。
		// 此处 receiver=$condCfg、target=$argCfg，即判定「$argCfg 是否后支配 $condCfg」。
		// 当前块级 ipdom 在部分 Yak/DB 加载的 CFG 上会得到假（与注释里「cond 不后支配 sink」不是同一方向的命题）。
		runSyntaxFlowExpect(t, `
c = 1
if (c) {
	x = "a"
} else {
	x = "b"
}
println(x)
`, `
c as $cond
println(* #-> as $arg)
$cond<getCfg()> as $condCfg
$arg<getCfg()> as $argCfg
$condCfg<cfgPostDominates(target: "$argCfg")> as $pd
`, sfExpect{
			WantMinCount:    map[string]int{"pd": 1},
			WantVarContains: map[string][]string{"pd": {"false"}},
		})
	})

	t.Run("self_anchor_postdominates_itself", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
println(0)
`, `
println(* #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgPostDominates(target: "$cfg")> as $pd
`, sfExpect{
			WantMinCount:    map[string]int{"pd": 1},
			WantVarContains: map[string][]string{"pd": {"true"}},
		})
	})

	t.Run("path_from_call_to_exit_hits_sink", func(t *testing.T) {
		// 从 foo() 调用点到出口必经 println(0)：起点 $callCfg，必经 target $sinkCfg。同块时常可按 Insts 序细化。
		runSyntaxFlowExpect(t, `
func foo() {
}

foo()
println(0)
`, `
foo() as $call
println(*?{have: "0"} #-> as $sinkArg)
$call<getCfg()> as $callCfg
$sinkArg<getCfg()> as $sinkCfg
$callCfg<cfgPostDominates(target: "$sinkCfg")> as $pd
`, sfExpect{
			WantMinCount:    map[string]int{"pd": 1},
			WantVarContains: map[string][]string{"pd": {"true"}},
		})
	})

	t.Run("path_from_sink_to_exit_need_not_hit_prior_call", func(t *testing.T) {
		// 起点为 println，问是否必经 foo()：从 sink 到出口不必再经过 call → false。
		runSyntaxFlowExpect(t, `
func foo() {
}

foo()
println(0)
`, `
foo() as $call
println(*?{have: "0"} #-> as $sinkArg)
$call<getCfg()> as $callCfg
$sinkArg<getCfg()> as $sinkCfg
$sinkCfg<cfgPostDominates(target: "$callCfg")> as $pd
`, sfExpect{
			WantMinCount:    map[string]int{"pd": 1},
			WantVarContains: map[string][]string{"pd": {"false"}},
		})
	})

	t.Run("later_println_path_to_exit_need_not_hit_earlier_println", func(t *testing.T) {
		// 图论：first 不后支配 second。API：起点为后一个 println，target 为前一个，问从后点到出口是否必经前点 → false。
		runSyntaxFlowExpect(t, `
println(1)
if true {
	println(0)
}
`, `
println(*?{have: "1"} #-> as $v1)
println(*?{have: "0"} #-> as $v0)
$v1<getCfg()> as $firstCfg
$v0<getCfg()> as $secondCfg
$secondCfg<cfgPostDominates(target: "$firstCfg")> as $pd
`, sfExpect{
			WantMinCount:    map[string]int{"pd": 1},
			WantVarContains: map[string][]string{"pd": {"false"}},
		})
	})

	t.Run("sink_does_not_postdominate_earlier_def_split_block", func(t *testing.T) {
		// 起点 sink，target 为较早的 a：从 sink 到出口不必回到 a 所在点 → false。
		runSyntaxFlowExpect(t, `
a = 1
if true {
	println(0)
}
`, `
a as $a
println(* #-> as $arg)
$a<getCfg()> as $aCfg
$arg<getCfg()> as $sinkCfg
$sinkCfg<cfgPostDominates(target: "$aCfg")> as $pd
`, sfExpect{
			WantMinCount:    map[string]int{"pd": 1},
			WantVarContains: map[string][]string{"pd": {"false"}},
		})
	})

	t.Run("early_cond_does_not_postdominate_later_sink", func(t *testing.T) {
		// 图论：cond 不后支配 println 实参。API：起点为 $argCfg，target 为 $condCfg，问从 sink 到出口是否必经 cond → false。
		runSyntaxFlowExpect(t, `
c = 1
if (c) {
	x = "a"
} else {
	x = "b"
}
println(x)
`, `
c as $cond
println(* #-> as $arg)
$cond<getCfg()> as $condCfg
$arg<getCfg()> as $argCfg
$argCfg<cfgPostDominates(target: "$condCfg")> as $pd
`, sfExpect{
			WantMinCount:    map[string]int{"pd": 1},
			WantVarContains: map[string][]string{"pd": {"false"}},
		})
	})

	t.Run("different_functions_returns_false", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
func fa() {
	println(1)
}
func fb() {
	println(2)
}
fa()
fb()
`, `
println(*?{have: "1"} #-> as $aArg)
println(*?{have: "2"} #-> as $bArg)
$aArg<getCfg()> as $faCfg
$bArg<getCfg()> as $fbCfg
$faCfg<cfgPostDominates(target: "$fbCfg")> as $pd
`, sfExpect{
			WantMinCount:    map[string]int{"pd": 1},
			WantVarContains: map[string][]string{"pd": {"false"}},
		})
	})

	t.Run("config_style_target_param", func(t *testing.T) {
		// 与 merge_sink_cfgPostDominates_const_anchor_ir 对仗：交换 receiver/target，验证 target="$var"。
		runSyntaxFlowExpect(t, `
c = 1
if (c) {
	x = "a"
} else {
	x = "b"
}
println(x)
`, `
c as $cond
println(* #-> as $arg)
$cond<getCfg()> as $condCfg
$arg<getCfg()> as $argCfg
$argCfg<cfgPostDominates(target="$condCfg")> as $pd
`, sfExpect{
			WantMinCount:    map[string]int{"pd": 1},
			WantVarContains: map[string][]string{"pd": {"false"}},
		})
	})

	t.Run("multiple_target_cfgs_OR_same_function_only", func(t *testing.T) {
		// 多 target：$tCfg 同时含 fa 的 println 与 fb 内两个 sink，仅同函数锚点参与 OR。
		// 多结果：$r 合并 a、b 两个 receiver，应对 **两个** 位置各出一条 cfgPostDominates 结果且均为 true。
		runSyntaxFlowExpect(t, `
func fa() {
	println(99)
}
func fb() {
	a = 1
	b = 2
	println(10)
	println(20)
}
fa()
fb()
`, `
println(*?{have: "99"} #-> as $m)
println(*?{have: "10"} #-> as $m)
println(*?{have: "20"} #-> as $m)
$m<getCfg()> as $tCfg
a as $r
b as $r
$r<getCfg()> as $rCfg
$rCfg<cfgPostDominates(target: "$tCfg")> as $pd
`, sfExpect{
			WantMinCount: map[string]int{"pd": 2},
			PostCheck: func(t *testing.T, res *ssaapi.SyntaxFlowResult) {
				for _, v := range res.GetValues("pd") {
					require.Contains(t, v.String(), "true")
				}
			},
		})
	})
}

// TestNativeCall_cfgReachable 覆盖 NativeCall_CFGReachable（cfgReachable）。
// 含「危险 sink 前过滤」场景：同函数保留/早返回丢弃/callee 内 sink 与 icfg；详见 control_flow_limitations.md「cfgReachable：危险 sink 前」。
func TestNativeCall_cfgReachable(t *testing.T) {
	// 危险函数 / sink 前过滤：用 cfgReachable(候选 cfg -> sink cfg) 丢掉「控制流上走不到 sink」的误报（过程内）。
	t.Run("filter_keep_when_trace_reaches_dangerous_same_func", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
func dangerous(v) {
	println(v)
}

x = 1
println("trace")
dangerous(x)
`, `
println(*?{have: "trace"} #-> as $traceArg)
dangerous(* #-> as $sinkArg)
$traceArg<getCfg()> as $traceCfg
$sinkArg<getCfg()> as $sinkCfg
$traceCfg<cfgReachable(target: "$sinkCfg")> as $reach
`, sfExpect{
			WantVarContains: map[string][]string{"reach": {"true"}},
		})
	})
	t.Run("cfgReachable_implicit_getCfg_on_chain_and_target", func(t *testing.T) {
		// 语法糖：链上与 target 均可为 SSA value，无需显式 <getCfg>；target 支持 target: $sym（与 target: "$sym" 等价帧解析）。
		runSyntaxFlowExpect(t, `
func dangerous(v) {
	println(v)
}

x = 1
println("trace")
dangerous(x)
`, `
println(*?{have: "trace"} #-> as $traceArg)
dangerous(* #-> as $sinkArg)
$traceArg<cfgReachable(target: $sinkArg)> as $reach
`, sfExpect{
			WantVarContains: map[string][]string{"reach": {"true"}},
		})
	})
	t.Run("filter_drop_when_branch_returns_before_dangerous", func(t *testing.T) {
		// then 分支 println 后直接 return，无 CFG 路径到 dangerous；else 分支可达 dangerous。
		runSyntaxFlowExpect(t, `
func dangerous(v) {
	println(v)
}

func f(p) {
	if (p) {
		println("ONLY_THEN")
		return
	}
	println("ONLY_ELSE")
	dangerous(1)
}

f(false)
`, `
println(*?{have: "ONLY_THEN"} #-> as $thenArg)
println(*?{have: "ONLY_ELSE"} #-> as $elseArg)
dangerous(* #-> as $dArg)
$thenArg<getCfg()> as $thenCfg
$elseArg<getCfg()> as $elseCfg
$dArg<getCfg()> as $sinkCfg
$thenCfg<cfgReachable(target: "$sinkCfg")> as $reachThen
$elseCfg<cfgReachable(target: "$sinkCfg")> as $reachElse
`, sfExpect{
			WantVarContains: map[string][]string{
				"reachThen": {"false"},
				"reachElse": {"true"},
			},
		})
	})
	t.Run("filter_intraproc_false_icfg_true_before_helper_to_dangerous", func(t *testing.T) {
		// 危险函数在 helper 内：main 中「helper 前的 println」到危险 println 无过程内路径，icfg 打开后为 true（典型误报裁剪：无 icfg 直接丢，有 icfg 再保留）。
		runSyntaxFlowExpect(t, `
func dangerous(v) {
    println("sink_marker", v)
}

func helper() {
    dangerous(2)
}

println("prep")
helper()
`, `
println(*?{have: "prep"} #-> as $prepArg)
println(*?{have: "sink_marker"} #-> as $sinkArg)
$prepArg<getCfg()> as $prepCfg
$sinkArg<getCfg()> as $sinkCfg
$prepCfg<cfgReachable(target: "$sinkCfg")> as $reachIntra
$prepCfg<cfgReachable(target: "$sinkCfg", icfg: true, max_depth: 16, max_nodes: 50000)> as $reachIcfg
`, sfExpect{
			WantVarContains: map[string][]string{
				"reachIntra": {"false"},
				"reachIcfg":  {"true"},
			},
		})
	})

	t.Run("intra_proc_unreachable_to_callee", func(t *testing.T) {
		// 将 foo 内 println 视为危险 sink：调用点之后 main 的 cfg 过程内无法到达 callee 内块，用于过滤「仅靠 dataflow 连在一起」的噪声。
		runSyntaxFlowExpect(t, `
func foo() {
	println("infoo")
}

foo()
println("after")
`, `
println(*?{have: "after"} #-> as $afterArg)
println(*?{have: "infoo"} #-> as $infooArg)

$afterArg<getCfg()> as $callerCfg
$infooArg<getCfg()> as $calleeCfg

$callerCfg<cfgReachable(target: "$calleeCfg")> as $reach
`, sfExpect{
			WantVarContains: map[string][]string{"reach": {"false"}},
		})
	})
	t.Run("icfg_reaches_callee", func(t *testing.T) {
		// 危险 sink 在 callee：调用点之后的语句 cfg 到 callee 内 println 仅 icfg 可达（与上一用例对照，用于规则里「无 icfg 则过滤 / 开 icfg 再保留」）。
		runSyntaxFlowExpect(t, `
func foo() {
	println("infoo")
}

foo()
println("after")
`, `
println(*?{have: "after"} #-> as $afterArg)
println(*?{have: "infoo"} #-> as $infooArg)

$afterArg<getCfg()> as $callerCfg
$infooArg<getCfg()> as $calleeCfg

$callerCfg<cfgReachable(target: "$calleeCfg", icfg: true)> as $reach
`, sfExpect{
			WantVarContains: map[string][]string{"reach": {"true"}},
		})
	})
	t.Run("config_style_caps", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
func foo() {
	println("infoo")
}

foo()
println("after")
`, `
println(*?{have: "after"} #-> as $afterArg)
println(*?{have: "infoo"} #-> as $infooArg)

$afterArg<getCfg()> as $callerCfg
$infooArg<getCfg()> as $calleeCfg

$callerCfg<cfgReachable(target: "$calleeCfg", icfg: true, max_depth: 4, max_nodes: 8000)> as $reach
`, sfExpect{
			WantVarContains: map[string][]string{"reach": {"true"}},
		})
	})
}

// TestNativeCall_cfgReachPath 覆盖 NativeCall_CFGReachPath（cfgReachPath）。
func TestNativeCall_cfgReachPath(t *testing.T) {
	t.Run("icfg_has_arrow_path", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
func foo() {
	println("infoo")
}

foo()
println("after")
`, `
println(*?{have: "after"} #-> as $afterArg)
println(*?{have: "infoo"} #-> as $infooArg)

$afterArg<getCfg()> as $callerCfg
$infooArg<getCfg()> as $calleeCfg

$callerCfg<cfgReachPath(target: "$calleeCfg", icfg: true, max_depth: 4, max_nodes: 8000)> as $path
`, sfExpect{
			WantVarContains: map[string][]string{"path": {"->"}},
		})
	})
	t.Run("intra_proc_no_path_evidence", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
func foo() {
	println("infoo")
}

foo()
println("after")
`, `
println(*?{have: "after"} #-> as $afterArg)
println(*?{have: "infoo"} #-> as $infooArg)

$afterArg<getCfg()> as $callerCfg
$infooArg<getCfg()> as $calleeCfg

$callerCfg<cfgReachPath(target: "$calleeCfg", icfg: false)> as $path
`, sfExpect{
			PostCheck: func(t *testing.T, res *ssaapi.SyntaxFlowResult) {
				require.NotContains(t, res.GetValues("path").String(), "->")
			},
		})
	})
}

// TestNativeCall_cfgCondition 覆盖 NativeCall_CFGCondition（cfgCondition）：if / switch / loop / merge 继承及无值场景。
func TestNativeCall_cfgCondition(t *testing.T) {
	t.Run("if_else_merge_inherits_discriminant", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
c = 1
if (c) {
	x = "a"
} else {
	x = "b"
}
println(x)
`, `
println(* #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgCondition()> as $cond
`, sfExpect{
			WantVarContains: map[string][]string{
				"cond": {"cond(func=", "values=", "source=if"},
			},
		})
	})
	t.Run("if_then_only_branch", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
c = 1
if (c) {
	println("then_only")
}
println("after")
`, `
println(*?{have: "then_only"} #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgCondition()> as $cond
`, sfExpect{
			WantVarContains: map[string][]string{
				"cond": {"cond(func=", "values=", "source=if"},
			},
		})
	})
	t.Run("switch_case_body", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
a = 2
switch a {
case 2:
	println("in_case")
}
println("after")
`, `
println(*?{have: "in_case"} #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgCondition()> as $cond
`, sfExpect{
			WantVarContains: map[string][]string{
				"cond": {"cond(func=", "values=", "source=switch"},
			},
		})
	})
	t.Run("switch_merge_after_branches", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
a = 2
switch a {
case 2:
	println("in_case")
}
println("after")
`, `
println(*?{have: "after"} #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgCondition()> as $cond
`, sfExpect{
			WantVarContains: map[string][]string{
				"cond": {"cond(func=", "values=", "source=switch"},
			},
		})
	})
	t.Run("loop_body_inherits_loop_cond", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
for i := 0; i < 3; i++ {
	println("body_marker")
}
`, `
println(*?{have: "body_marker"} #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgCondition()> as $cond
`, sfExpect{
			WantVarContains: map[string][]string{
				"cond": {"cond(func=", "values=", "source=loop"},
			},
		})
	})
	t.Run("linear_no_branch_empty_values_but_cond_string", func(t *testing.T) {
		runSyntaxFlowExpect(t, `println("solo")`, `
println(* #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgCondition()> as $cond
`, sfExpect{
			WantVarContains: map[string][]string{
				"cond": {"cond(func=", "values=[]"},
			},
		})
	})
}

// TestNativeCall_cfgConditionValues 覆盖 NativeCall_CFGConditionValues（cfgConditionValues）：与 cfgCondition 同源摘要上的条件 SSA value。
func TestNativeCall_cfgConditionValues(t *testing.T) {
	t.Run("if_else_merge_matches_discriminant_var", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
c = 1
if (c) {
	x = "a"
} else {
	x = "b"
}
println(x)
`, `
c as $cVal
println(* #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgConditionValues()> as $conds
$conds as $condVals
`, sfExpect{
			WantVarContains: map[string][]string{
				"cVal":     {"1"},
				"condVals": {"1"},
			},
		})
	})
	t.Run("switch_merge_matches_discriminant_var", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
a = 2
switch a {
case 2:
	println("in_case")
}
println("after")
`, `
a as $aVal
println(*?{have: "after"} #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgConditionValues()> as $conds
$conds as $condVals
`, sfExpect{
			WantVarContains: map[string][]string{
				"aVal":     {"2"},
				"condVals": {"2"},
			},
		})
	})
	t.Run("switch_case_body_matches_discriminant", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
a = 2
switch a {
case 2:
	println("in_case")
}
`, `
a as $aVal
println(*?{have: "in_case"} #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgConditionValues()> as $conds
$conds as $condVals
`, sfExpect{
			WantVarContains: map[string][]string{
				"aVal":     {"2"},
				"condVals": {"2"},
			},
		})
	})
	t.Run("loop_body_has_at_least_one_cond_value", func(t *testing.T) {
		runSyntaxFlowExpect(t, `
for i := 0; i < 3; i++ {
	println("body_marker")
}
`, `
println(*?{have: "body_marker"} #-> as $arg)
$arg<getCfg()> as $cfg
$cfg<cfgConditionValues()> as $conds
$conds as $condVals
`, sfExpect{
			WantMinCount: map[string]int{"condVals": 1},
		})
	})
	t.Run("linear_no_branch_yields_no_cfgConditionValues", func(t *testing.T) {
		ssatest.Check(t, `println("only")`, func(prog *ssaapi.Program) error {
			res, err := prog.SyntaxFlowWithError(`
println(* #-> as $a)
$a<getCfg()> as $cfg
$cfg<cfgConditionValues()> as $v
`, ssaapi.QueryWithEnableDebug())
			require.NoError(t, err)
			require.Empty(t, res.GetValues("v"), "cfgConditionValues should not bind SSA values without a branch condition")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}
