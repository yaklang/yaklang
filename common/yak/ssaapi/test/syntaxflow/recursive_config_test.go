package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSF_Config_Until(t *testing.T) {
	t.Run("until", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		// match until 
		a = 11
		b1 = f(a,1)

		// no match until get undefined 
		b3 = ccc 
		`,
			"b* #{until:`* ?{opcode:call}`}-> * as $result",
			map[string][]string{
				"result": {"Undefined-f(11,1)"},
			})
	})

	t.Run("util in dataflow path", func(t *testing.T) {
		/*
			a
				-- f
					-- 1  // const
					-- 1  // actual-parameter
				-- f2
					-- 2  // const
					-- b //  actual-parameter  // only this path
		*/
		code := `
	f = (i) => {
		return i + 1
	}

	f2 = (i) => {
		return i + 2 
	}
	b = 11 
	a = f(1) + f2(b)
	`

		t.Run("test until contain include", func(t *testing.T) {
			ssatest.CheckSyntaxFlow(t, code, `
		b as $b
		a #{
			until: "* & $b"
		}-> as $output
		`, map[string][]string{
				"output": {"11"},
			})
		})
	})

}

func TestSF_Config_HOOK(t *testing.T) {
	t.Run("hook", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `
		a = 11
		b = f(a,1)
		`,
			"b #{hook:`* as $num`}-> as $result",
			map[string][]string{
				"num": {"Undefined-f(11,1)"},
			})
	})

}

func TestSF_Config_Exclude(t *testing.T) {
	t.Run("exclude in top value", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		// match exclude 
		b = f1(a1,1)

		// no match exclude get undefined
		b2 = f2(a2)
		`,
			"b* #{exclude:`* ?{opcode:const}`}-> as $result",
			map[string][]string{
				"result": {
					"Undefined-a1", "Undefined-f1",
					"Undefined-a2", "Undefined-f2",
				},
			})
	})

	t.Run("exclude in dataflow path ", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		b = f1(1 + d)

		b2 = 11 + c 
		`, "b* #{exclude: `* ?{opcode:call}`}-> as $result", map[string][]string{
			"result": {"Undefined-c", "11"},
		})
	})
}

func TestSF_Config_Include(t *testing.T) {
	t.Run("include", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		b0 = f1 + 0 
		b1 = f1(1)
		b2 = f2(2)
		b3 = f3(3)
		`,
			"b* #{include:`* ?{have:f1}`}-> as $result",
			map[string][]string{
				"result": {"Undefined-f1", "1", "0"},
			})
	})

	t.Run("include in dataflow path", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		b0 = f1 + 0 
		b1 = f1(1)
		b2 = f2(2)
		b3 = f3(3)
		`,
			"b* #{include:`* ?{have:f1 && opcode:call}`}-> as $result; ",
			map[string][]string{
				"result": {"Undefined-f1", "1"},
			})
	})
}

// topDefPruneCompareIncVsReach 在同一段源码上对比「仅 include（路径与分支符号相交，做分支侧剪枝）」与「仅 include_reachable（CFG 锚点）」：
// 通常路径相交更严：include 的 TopDef 集合 ⊆ include_reachable（后者仍可能保留分支外入口常量）。
func topDefPruneCompareIncVsReach(t *testing.T, code, ruleInc, ruleReach string) {
	t.Helper()
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		resInc, err := prog.SyntaxFlowWithError(ruleInc)
		require.NoError(t, err)
		resReach, err := prog.SyntaxFlowWithError(ruleReach)
		require.NoError(t, err)
		vi := resInc.GetValues("outInc")
		vr := resReach.GetValues("outReach")
		assertEveryInBInA(t, vr, vi, "include ⊆ include_reachable")
		require.GreaterOrEqual(t, len(vr), len(vi), "include_reachable 解集不应更小")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

// TestSF_Config_TopDefReachable：多段控制流材料 + include / include_reachable 剪枝对比，并穿插 exclude_reachable、only_reachable 与组合键。
func TestSF_Config_TopDefReachable(t *testing.T) {
	const codeIfPhi = `
c = "test"
x = c
if (c) {
	a = "thenStr"
	x = a
} else {
	b = "elseStr"
	x = b
}
println(x)
`
	t.Run("if_flat_include_path_prune_vs_include_reachable_cfg", func(t *testing.T) {
		// include：路径上须出现 then 侧赋值目标（与 then 分支数据流相交）；include_reachable：SSA 点须能 CFG 到达 then 块锚点。
		rInc := `
println(* as $sink)
a as $thenVal
$sink * #{
include: ` + "`* & $thenVal`" + `,
}-> as $outInc
`
		rReach := `
println(* as $sink)
a as $thenVal
$thenVal<getCfg> as $thenCfg
$sink * #{
include_reachable: ` + "`$thenCfg`" + `,
}-> as $outReach
`
		topDefPruneCompareIncVsReach(t, codeIfPhi, rInc, rReach)
		ssatest.CheckSyntaxFlow(t, codeIfPhi, rInc, map[string][]string{"outInc": {`"thenStr"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
		ssatest.CheckSyntaxFlow(t, codeIfPhi, rReach, map[string][]string{"outReach": {`"test"`, `"thenStr"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("if_flat_exclude_reachable_then_anchor", func(t *testing.T) {
		rule := `
println(* as $sink)
a as $thenVal
$thenVal<getCfg> as $thenCfg
$sink * #{
include: ` + "`*`" + `,
exclude_reachable: ` + "`$thenCfg`" + `,
}-> as $outExc
`
		ssatest.CheckSyntaxFlow(t, codeIfPhi, rule, map[string][]string{"outExc": {`"elseStr"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("exclude_reachable_two_same_key_lines", func(t *testing.T) {
		// 同一 key 写两行时 native$call 曾用 map 覆盖；#{}-> 侧保留多条。结果应与只写一次 exclude 相同（冗余相同锚点）。
		base := `
println(* as $sink)
a as $thenVal
$thenVal<getCfg> as $thenCfg
`
		single := base + `
$sink * #{
include: ` + "`*`" + `,
exclude_reachable: ` + "`$thenCfg`" + `,
}-> as $out
`
		twoLines := base + `
$sink * #{
include: ` + "`*`" + `,
exclude_reachable: ` + "`$thenCfg`" + `,
exclude_reachable: ` + "`$thenCfg`" + `,
}-> as $out
`
		want := map[string][]string{"out": {`"elseStr"`}}
		ssatest.CheckSyntaxFlow(t, codeIfPhi, single, want, ssaapi.WithLanguage(ssaconfig.Yak))
		ssatest.CheckSyntaxFlow(t, codeIfPhi, twoLines, want, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("if_flat_only_reachable_then_anchor", func(t *testing.T) {
		rule := `
println(* as $sink)
a as $thenVal
$thenVal<getCfg> as $thenCfg
$sink * #{
only_reachable: ` + "`$thenCfg`" + `,
}-> as $outOnly
`
		ssatest.CheckSyntaxFlow(t, codeIfPhi, rule, map[string][]string{"outOnly": {`"test"`, `"thenStr"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	const codeNestedIf = `
c = "guard"
x = c
if (c) {
	if (c) {
		a = "deepThen"
		x = a
	} else {
		b = "deepElse"
		x = b
	}
}
println(x)
`
	t.Run("nested_if_include_vs_include_reachable", func(t *testing.T) {
		rInc := `
println(* as $sink)
a as $deepThenVal
$sink * #{
include: ` + "`* & $deepThenVal`" + `,
}-> as $outInc
`
		rReach := `
println(* as $sink)
a as $deepThenVal
$deepThenVal<getCfg> as $deepThenCfg
$sink * #{
include_reachable: ` + "`$deepThenCfg`" + `,
}-> as $outReach
`
		topDefPruneCompareIncVsReach(t, codeNestedIf, rInc, rReach)
		ssatest.CheckSyntaxFlow(t, codeNestedIf, rInc, map[string][]string{"outInc": {`"deepThen"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
		ssatest.CheckSyntaxFlow(t, codeNestedIf, rReach, map[string][]string{"outReach": {`"deepThen"`, `"guard"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	const codeLoop = `
seed = "seed"
x = seed
for i = 0; i < 2; i++ {
	loopVal = "loopStr"
	x = loopVal
}
println(x)
`
	t.Run("loop_include_vs_include_reachable", func(t *testing.T) {
		rInc := `
println(* as $sink)
loopVal as $loopSym
$sink * #{
include: ` + "`* & $loopSym`" + `,
}-> as $outInc
`
		rReach := `
println(* as $sink)
loopVal as $loopSym
$loopSym<getCfg> as $loopCfg
$sink * #{
include_reachable: ` + "`$loopCfg`" + `,
}-> as $outReach
`
		topDefPruneCompareIncVsReach(t, codeLoop, rInc, rReach)
		ssatest.CheckSyntaxFlow(t, codeLoop, rInc, map[string][]string{"outInc": {`"loopStr"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
		ssatest.CheckSyntaxFlow(t, codeLoop, rReach, map[string][]string{"outReach": {`"loopStr"`, `"seed"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	const codeLoopBreak = `
seed = "seed"
x = seed
for i = 0; i < 3; i++ {
	if (i) {
		br = "breakStr"
		x = br
		break
	}
	ct = "continueStr"
	x = ct
}
println(x)
`
	t.Run("loop_break_include_vs_include_reachable", func(t *testing.T) {
		rInc := `
println(* as $sink)
br as $brSym
$sink * #{
include: ` + "`* & $brSym`" + `,
}-> as $outInc
`
		rReach := `
println(* as $sink)
br as $brSym
$brSym<getCfg> as $brCfg
$sink * #{
include_reachable: ` + "`$brCfg`" + `,
}-> as $outReach
`
		topDefPruneCompareIncVsReach(t, codeLoopBreak, rInc, rReach)
		ssatest.CheckSyntaxFlow(t, codeLoopBreak, rInc, map[string][]string{"outInc": {`"breakStr"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
		ssatest.CheckSyntaxFlow(t, codeLoopBreak, rReach, map[string][]string{"outReach": {`"breakStr"`, `"seed"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	const codeLoopContinue = `
seed = "seed"
x = seed
for i = 0; i < 3; i++ {
	if (i) {
		ct = "continueStr"
		x = ct
		continue
	}
	af = "afterStr"
	x = af
}
println(x)
`
	t.Run("loop_continue_include_vs_include_reachable", func(t *testing.T) {
		rInc := `
println(* as $sink)
ct as $ctSym
$sink * #{
include: ` + "`* & $ctSym`" + `,
}-> as $outInc
`
		rReach := `
println(* as $sink)
ct as $ctSym
$ctSym<getCfg> as $ctCfg
$sink * #{
include_reachable: ` + "`$ctCfg`" + `,
}-> as $outReach
`
		topDefPruneCompareIncVsReach(t, codeLoopContinue, rInc, rReach)
		ssatest.CheckSyntaxFlow(t, codeLoopContinue, rInc, map[string][]string{"outInc": {`"continueStr"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
		ssatest.CheckSyntaxFlow(t, codeLoopContinue, rReach, map[string][]string{"outReach": {`"continueStr"`, `"seed"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	const codeNestedLoop = `
seed = "s"
x = seed
for i = 0; i < 2; i++ {
	for j = 0; j < 2; j++ {
		inn = "inner"
		x = inn
	}
}
println(x)
`
	t.Run("nested_loop_include_vs_include_reachable", func(t *testing.T) {
		rInc := `
println(* as $sink)
inn as $innSym
$sink * #{
include: ` + "`* & $innSym`" + `,
}-> as $outInc
`
		rReach := `
println(* as $sink)
inn as $innSym
$innSym<getCfg> as $innCfg
$sink * #{
include_reachable: ` + "`$innCfg`" + `,
}-> as $outReach
`
		topDefPruneCompareIncVsReach(t, codeNestedLoop, rInc, rReach)
		ssatest.CheckSyntaxFlow(t, codeNestedLoop, rInc, map[string][]string{"outInc": {`"inner"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
		ssatest.CheckSyntaxFlow(t, codeNestedLoop, rReach, map[string][]string{"outReach": {`"inner"`, `"s"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	const codeLoopInnerIf = `
flag = "loopGuard"
x = flag
for i = 0; i < 2; i++ {
	i = "loopVal"
	if (i) {
		a = "loopThen"
		x = a
	} else {
		b = "loopElse"
		x = b
	}
}
println(x)
`
	t.Run("loop_inner_if_include_vs_include_reachable", func(t *testing.T) {
		rInc := `
println(* as $sink)
a as $loopThenSym
$sink * #{
include: ` + "`* & $loopThenSym`" + `,
}-> as $outInc
`
		rReach := `
println(* as $sink)
a as $loopThenSym
$loopThenSym<getCfg> as $loopThenCfg
$sink * #{
include_reachable: ` + "`$loopThenCfg`" + `,
}-> as $outReach
`
		topDefPruneCompareIncVsReach(t, codeLoopInnerIf, rInc, rReach)
		ssatest.CheckSyntaxFlow(t, codeLoopInnerIf, rInc, map[string][]string{"outInc": {`"loopThen"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
		ssatest.CheckSyntaxFlow(t, codeLoopInnerIf, rReach, map[string][]string{"outReach": {`"loopThen"`, `"loopVal"`, `"loopGuard"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("if_flat_include_plus_include_reachable_combined", func(t *testing.T) {
		rule := `
println(* as $sink)
a as $thenVal
$thenVal<getCfg> as $thenCfg
$sink * #{
include: ` + "`*`" + `,
include_reachable: ` + "`$thenCfg`" + `,
}-> as $outBoth
`
		ssatest.CheckSyntaxFlow(t, codeIfPhi, rule, map[string][]string{"outBoth": {`"test"`, `"thenStr"`}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}

func TestSF_config_WithNameVariableInner(t *testing.T) {
	/*
		utils/include/exclude can use variable, but `__next__` is magic name,
		variable len:
			0:  just use `_` variable
			1:  use this  variable
			>1: use `__next__` variable
	*/
	check := func(t *testing.T, code string) {
		ssatest.CheckSyntaxFlow(t, `
		b0 = f1(1)

		b1 = f2 + 22
		`,
			code, map[string][]string{
				"result": {"Undefined-f1(1)"},
			})
	}
	t.Run("check no name", func(t *testing.T) {
		check(t, "b* #{until:`* ?{opcode:call}`}-> as $result")
	})

	t.Run("check only one name", func(t *testing.T) {
		check(t, "b* #{until:`* ?{opcode:call} as $name`}-> as $result")
	})

	t.Run("check only magic name", func(t *testing.T) {
		check(t, `
b* #{until: <<<UNTIL
	* ?{opcode:call} as $__next__
UNTIL
}-> as $result`)
	})

	t.Run("check mix magic name", func(t *testing.T) {
		check(t, `
b* #{until: <<<UNTIL
	* as $value;
	* ?{opcode:call} as $__next__
UNTIL
}-> as $result`)
	})
}

func TestSF_Config_MultipleConfig(t *testing.T) {
	code := `
f1 = () => {
	return 22
}

b = 11
if c1 {
	b = f1()
}else if c1 {
	b = f(b, 33)
}else {
	b = 44
}

println(b) // phi 
`
	t.Run("hook and exclude", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
println(* as $para);
$para #{
		hook: <<<HOOK
			*?{opcode:const} as $const
HOOK,
		exclude: <<<EXCLUDE
			*?{opcode:call}
EXCLUDE,
}-> as $result 
			`,
			map[string][]string{
				"const":  {"11", "22", "33", "44"},
				"result": {"44", "Undefined-c1"},
			})
	})
	t.Run("hook and until", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`
println(* as $para)
$para #{
	hook: <<<HOOK
			*?{opcode:const} as $const
HOOK,
	until: <<<UNTIL
		*?{opcode:call}
UNTIL,
}-> 

			`,
			map[string][]string{
				"const": {"44"},
			})
	})
}

func TestSF_NativeCall_DataFlow_DFS(t *testing.T) {
	code := `

/*
getCmd()
function-getCmd -> return(binaryOpAdd)
filter(param1) 
function_filter	-> return(binaryOpAdd)
parameter2 --> getFunction
actx pop call
...
*/
getCmd = (param1) => {
	return filter(param1) + "-al"
}

filter = (param2) => {
	return param2 - "-t" 
}

cmd = "ls"
if c1{
	cmd += "-l"
}else{
	cmd = getCmd()
}
exec(cmd)
`

	t.Run("exclude all paths", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
exec(* as $end);
$end #-> as $start;

$start<dataflow(
exclude:<<<EXCLUDE
	*?{have:'getCmd'}?{opcode:call}
EXCLUDE,
end:'end',
)>as $result;
			`,
			map[string][]string{
				"start":  {"\"-al\"", "\"-l\"", "\"-t\"", "\"ls\"", "Parameter-param1", "Undefined-c1"},
				"result": {"\"-l\"", "\"ls\"", "Undefined-c1"},
				"end":    {"phi(cmd)[\"ls-l\",Function-getCmd() binding[Function-filter]]"},
			})
	})

	t.Run("exclude some of paths", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
exec(* as $end);
$end #-> as $start;

$start<dataflow(
exclude:<<<EXCLUDE
	filter?{opcode:function}
EXCLUDE,
end:'end',
)>as $result;
			`,
			map[string][]string{
				"start":  {"\"-al\"", "\"-l\"", "\"-t\"", "\"ls\"", "Parameter-param1", "Undefined-c1"},
				"result": {"\"-al\"", "\"-l\"", "\"ls\"", "Parameter-param1", "Undefined-c1"},
				"end":    {"phi(cmd)[\"ls-l\",Function-getCmd() binding[Function-filter]]"},
			})
	})

	t.Run("include some of paths", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
exec(* #->as $start);
getCmd?{opcode:function} as $end;

$start<dataflow(
include:<<<INCLUDE
	*?{have:'-t'}
INCLUDE,
end:'end',
)>as $result;
			`,
			map[string][]string{
				"start":  {"\"-al\"", "\"-l\"", "\"-t\"", "\"ls\"", "Parameter-param1", "Undefined-c1"},
				"result": {"\"-t\"", "Parameter-param1"},
				"end":    {"Function-getCmd"},
			})
	})
}

func TestSF_Until_Real_Demo(t *testing.T) {
	t.Run("test until edge demo", func(t *testing.T) {
		code := ` 
package com.example;
	class Main{
    public R vul(@RequestParam("file") MultipartFile file, HttpServletRequest request) {
         String res;
        String suffix = file.getOriginalFilename().substring(file.getOriginalFilename().lastIndexOf(".") + 1);
        if (!uploadUtil.checkFileSuffixWhiteList(suffix)){
            return R.error("文件后缀不合法");
        }
        String path = request.getScheme() + "://" + request.getServerName() + ":" + request.getServerPort() + "/file/";
        target=file+ suffix+ path;
    }
}`

		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			vals, err := prog.SyntaxFlowWithError(`
MultipartFile?{opcode:param} as $source
target* #{until: <<<UNTIL
 * & $source
UNTIL
}-> as $result;
`)
			require.NoError(t, err)
			result := vals.GetValues("result")
			result.Show()
			require.Equal(t, 1, len(result))
			require.Equal(t, "Parameter-file", result[0].String())
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
