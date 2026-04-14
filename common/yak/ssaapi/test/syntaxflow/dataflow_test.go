package syntaxflow

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

// sfCase 统一描述：源码 code、SyntaxFlow rule、以及期望 want / wantContains。
// - Want：变量名 -> 期望的 value 字符串集合（排序后与结果 **相等**，与 ssatest.CheckSyntaxFlow 一致）。
// - WantContains：变量名 -> 若干子串，每个子串须出现在该变量对应结果的 String() 中。
// - PostCheck：在 Want / WantContains 之后执行，用于校验常量布尔等 Want 不便表达的断言。
// - Debug：为 true 时 SyntaxFlow 带 QueryWithEnableDebug。
type sfCase struct {
	Code         string
	Rule         string
	Want         map[string][]string
	WantContains map[string][]string
	PostCheck    func(t *testing.T, res *ssaapi.SyntaxFlowResult)
	Debug        bool
}

func sortedValueStrings(vs []*ssaapi.Value) []string {
	out := lo.Map(vs, func(v *ssaapi.Value, _ int) string { return v.String() })
	sort.Strings(out)
	return out
}

// assertEveryInBInA：B 中每个元素（按 Value.String）须出现在 A 的字符串集合中（用于「过滤结果是未过滤结果的子集」）。
func assertEveryInBInA(t *testing.T, a, b []*ssaapi.Value, msg string) {
	t.Helper()
	set := make(map[string]struct{}, len(a))
	for _, v := range a {
		if v == nil {
			continue
		}
		set[v.String()] = struct{}{}
	}
	for _, v := range b {
		if v == nil {
			continue
		}
		_, ok := set[v.String()]
		require.True(t, ok, "%s: filtered value %q must appear in unfiltered set", msg, v.String())
	}
}

func runSFCase(t *testing.T, c sfCase, opts ...ssaconfig.Option) {
	t.Helper()
	code := strings.TrimSpace(c.Code)
	rule := strings.TrimSpace(c.Rule)
	require.NotEmpty(t, code, "sfCase.Code")
	require.NotEmpty(t, rule, "sfCase.Rule")

	handler := func(prog *ssaapi.Program) error {
		var sfOpts []ssaapi.QueryOption
		if c.Debug {
			sfOpts = append(sfOpts, ssaapi.QueryWithEnableDebug())
		}
		vals, err := prog.SyntaxFlowWithError(rule, sfOpts...)
		require.NoError(t, err)
		vals.Show()

		if len(c.Want) > 0 {
			for varName, want := range c.Want {
				gotVs := vals.GetValues(varName)
				got := lo.Map(gotVs, func(v *ssaapi.Value, _ int) string { return v.String() })
				sort.Strings(got)
				exp := append([]string{}, want...)
				sort.Strings(exp)
				require.Equal(t, exp, got, "variable %q (exact)", varName)
			}
		}
		if len(c.WantContains) > 0 {
			for varName, needles := range c.WantContains {
				hay := vals.GetValues(varName).String()
				for _, sub := range needles {
					require.Contains(t, hay, sub, "variable %q (contains)", varName)
				}
			}
		}
		if c.PostCheck != nil {
			c.PostCheck(t, vals)
		}
		return nil
	}
	ssatest.Check(t, code, handler, opts...)
}

// --- Java：文件读链路与 dataflow include（同一顶层测试，子场景用 t.Run） ---

func TestDataflowReal1(t *testing.T) {
	t.Run("file_read_chain", func(t *testing.T) {
		const javaFileRead = `
package com.ruoyi.common.utils.file;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.OutputStream;
import java.io.UnsupportedEncodingException;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;


public class FileUtils extends org.apache.commons.io.FileUtils
{
    public static String FILENAME_PATTERN = "[a-zA-Z0-9_\\-\\|\\.\\u4e00-\\u9fa5]+";

    /**
     * 输出指定文件的byte数组
     * 
     * @param filePath 文件路径
     * @param os 输出流
     * @return
     */
    public static void writeBytes(String filePath, OutputStream os) throws IOException
    {
        FileInputStream fis = null;
        try
        {
            File file = new File(filePath);
            if (!file.exists())
            {
                throw new FileNotFoundException(filePath);
            }
            fis = new FileInputStream(file);
            byte[] b = new byte[1024];
            int length;
            while ((length = fis.read(b)) > 0)
            {
                os.write(b, 0, length);
            }
        }
        catch (IOException e)
        {
            throw e;
        }
        finally
        {
            if (os != null)
            {
                try
                {
                    os.close();
                }
                catch (IOException e1)
                {
                    e1.printStackTrace();
                }
            }
            if (fis != null)
            {
                try
                {
                    fis.close();
                }
                catch (IOException e1)
                {
                    e1.printStackTrace();
                }
            }
        }
    }
   
}
`
		const ruleFileRead = `
File() as $fileInstance 
$fileInstance -{
	include: <<<CODE
	.read()
CODE
}-> as $fileReadInstance 
`
		runSFCase(t, sfCase{
			Code: javaFileRead,
			Rule: ruleFileRead,
			WantContains: map[string][]string{
				"fileInstance":     {`Undefined-File(Undefined-File,Parameter-filePath)`},
				"fileReadInstance": {`Undefined-fis.read`},
			},
		}, ssaapi.WithRawLanguage("java"))
	})

	t.Run("ddos_socket_readline", func(t *testing.T) {
		const javaDdos = `
package org.example.Dos;

import java.io.*;
import java.net.Socket;

public class DOSDemo {
    public static void readSocketData(Socket socket) throws IOException {
        BufferedReader reader = new BufferedReader(
                new InputStreamReader(socket.getInputStream())
        );
        String line;
        // 限制单行的最大长度
        final int MAX_LINE_LENGTH = 1024; // 最大行长度为1024个字符
        while ((line = reader.readLine()) != null) {
            processLine(line);
        }
    }
}
`
		const ruleDdos = `
.getInputStream()?{<fullTypeName>?{have: 'java.net.Socket' || 'java.new.ServerSocket'}} as $source;
BufferedReader().readLine()?{!.length}?{<fullTypeName>?{have:'java.io'}}  as $sink;
$sink#{
    include:<<<CODE
    <self> & $source
CODE
}-> as $vul;
`
		runSFCase(t, sfCase{
			Code:  javaDdos,
			Rule:  ruleDdos,
			Debug: true,
			WantContains: map[string][]string{
				"vul": {`Parameter-socket`},
			},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

// --- Yak：基础 dataflow（const 传播） ---

func TestDataflowTest(t *testing.T) {
	runSFCase(t, sfCase{
		Code: `
	a = {} 

	source := a.b()
	{
		b = source + 1 
		b = c(b)
		f1(b)
	}
	`,
		Rule: `
a.b() as $source 
f1(* as $sink)
$sink #-> as $vul1
$sink<dataflow(<<<CODE
    * ?{opcode: const} as $value1
CODE)> 
    `,
		Want: map[string][]string{
			"value1": {"1"},
		},
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

// --- Yak：`until` 边界（同一函数内多子场景） ---

func TestUntil(t *testing.T) {
	const ruleUntil = `
a as $source
b(* as $sink)
$sink #{
    until: "* & $source"
}-> as $target 
`

	t.Run("match_via_array_element", func(t *testing.T) {
		runSFCase(t, sfCase{
			Code: `
a = 12344
cc = [1, 2 , a]
b(cc)
    `,
			Rule: ruleUntil,
			Want: map[string][]string{
				"target": {"12344"},
			},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("no_match_when_array_has_no_source", func(t *testing.T) {
		runSFCase(t, sfCase{
			Code: `
a = 12344
cc = [1, 2 , 3]
b(cc)
`,
			Rule: ruleUntil,
			Want: map[string][]string{
				"target": {},
			},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}

// --- Yak：`only_reachable` 与「无 only_reachable」在同一规则内对照 ---

func TestDataflow_OnlyReachable(t *testing.T) {
	// 说明：
	// 1) 这里统一使用 Want 做精确快照断言，不使用 PostCheck；
	// 2) 对每个场景同时保留无过滤链路（$noFilter）与 only_reachable 锚点链路；
	// 3) loop / nested-if / loop-if / loop-if-return 下目前 thenPost/elsePost 为空，
	//    作为现阶段实现行为快照（后续语义调整时可据此定位回归）。

	t.Run("if_phi_branch_anchors", func(t *testing.T) {
		// c=1：then 执行；SSA 仍保留 else 侧。无 only_reachable 时 #-> 枚举两侧常量；post + 分支内 getCfg 锚应各留一侧。
		runSFCase(t, sfCase{
			Code: `
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
`,
			Rule: `
println(* as $sink)

$sink<dataflow(<<<CODE
	* #-> as $noFilter
CODE)>

a as $thenVal
$thenVal<getCfg> as $thenCfg
$sink<dataflow(<<<CODE
	* #-> as $thenPost
CODE, only_reachable="$thenCfg", only_reachable_mode="post")>

b as $elseVal
$elseVal<getCfg> as $elseCfg
$sink<dataflow(<<<CODE
	* #-> as $elsePost
CODE, only_reachable="$elseCfg", only_reachable_mode="post")>
`,
			Want: map[string][]string{
				"noFilter": {`"test"`, `"thenStr"`, `"elseStr"`},
				"thenPost": {`"test"`, `"thenStr"`},
				"elsePost": {`"test"`, `"elseStr"`},
			},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("loop_anchor_keeps_loop_const", func(t *testing.T) {
		// loop 场景：锚点本身可定位（loopAnchor），但 only_reachable 结果当前为空（快照）。
		runSFCase(t, sfCase{
			Code: `
seed = "seed"
x = seed
for i = 0; i < 2; i++ {
	loopVal = "loopStr"
	x = loopVal
}
println(x)
`,
			Rule: `
println(* as $sink)
$sink<dataflow(<<<CODE
	* #-> as $noFilter
CODE)>

loopVal as $loopAnchor
$loopAnchor<getCfg> as $loopCfg
$sink<dataflow(<<<CODE
	* #-> as $loopPost
CODE, only_reachable="$loopCfg", only_reachable_mode="post")>
`,
			Want: map[string][]string{
				"loopAnchor": {`"loopStr"`},
				"loopPost":   {`"loopStr"`, `"seed"`},
			},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("loop_break_branch_anchors", func(t *testing.T) {
		runSFCase(t, sfCase{
			Code: `
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
`,
			Rule: `
println(* as $sink)
$sink<dataflow(<<<CODE
	* #-> as $noFilter
CODE)>

br as $breakVal
$breakVal<getCfg> as $breakCfg
$sink<dataflow(<<<CODE
	* #-> as $breakPost
CODE, only_reachable="$breakCfg", only_reachable_mode="post")>

ct as $continueVal
$continueVal<getCfg> as $continueCfg
$sink<dataflow(<<<CODE
	* #-> as $continuePost
CODE, only_reachable="$continueCfg", only_reachable_mode="post")>
`,
			Want: map[string][]string{
				"breakPost":    {`"breakStr"`, `"seed"`},
				"continuePost": {`"continueStr"`, `"seed"`},
			},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("loop_continue_branch_anchors", func(t *testing.T) {
		runSFCase(t, sfCase{
			Code: `
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
`,
			Rule: `
println(* as $sink)
$sink<dataflow(<<<CODE
	* #-> as $noFilter
CODE)>

ct as $continueVal
$continueVal<getCfg> as $continueCfg
$sink<dataflow(<<<CODE
	* #-> as $continuePost
CODE, only_reachable="$continueCfg", only_reachable_mode="post")>

af as $afterVal
$afterVal<getCfg> as $afterCfg
$sink<dataflow(<<<CODE
	* #-> as $afterPost
CODE, only_reachable="$afterCfg", only_reachable_mode="post")>
`,
			Want: map[string][]string{
				"continuePost": {`"continueStr"`, `"seed"`},
				"afterPost":    {`"afterStr"`, `"seed"`},
			},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("nested_if_branch_anchors", func(t *testing.T) {
		// if 嵌套场景：内层 then/else 锚点能命中变量，only_reachable 结果当前为空（快照）。
		runSFCase(t, sfCase{
			Code: `
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
`,
			Rule: `
println(* as $sink)
$sink<dataflow(<<<CODE
	* #-> as $noFilter
CODE)>

a as $thenVal
$thenVal<getCfg> as $thenCfg
$sink<dataflow(<<<CODE
	* #-> as $thenPost
CODE, only_reachable="$thenCfg", only_reachable_mode="post")>

b as $elseVal
$elseVal<getCfg> as $elseCfg
$sink<dataflow(<<<CODE
	* #-> as $elsePost
CODE, only_reachable="$elseCfg", only_reachable_mode="post")>
`,
			Want: map[string][]string{
				"thenPost": {`"guard"`, `"deepThen"`},
				"elsePost": {`"guard"`, `"deepElse"`},
			},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("loop_with_if_branch_anchors", func(t *testing.T) {
		// loop 中有 if：分支锚点可命中，only_reachable 结果当前为空（快照）。
		runSFCase(t, sfCase{
			Code: `
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
`,
			Rule: `
println(* as $sink)
$sink<dataflow(<<<CODE
	* #-> as $noFilter
CODE)>

a as $thenVal
$thenVal<getCfg> as $thenCfg
$sink<dataflow(<<<CODE
	* #-> as $thenPost
CODE, only_reachable="$thenCfg", only_reachable_mode="post")>

b as $elseVal
$elseVal<getCfg> as $elseCfg
$sink<dataflow(<<<CODE
	* #-> as $elsePost
CODE, only_reachable="$elseCfg", only_reachable_mode="post")>
`,
			Want: map[string][]string{
				"thenPost": {`"loopThen"`, `"loopVal"`, `"loopGuard"`},
				"elsePost": {`"loopElse"`, `"loopVal"`, `"loopGuard"`},
			},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("loop_if_return_branch_anchors", func(t *testing.T) {
		// loop 中 if + return：return 分支锚点可命中，only_reachable 结果当前为空（快照）。
		runSFCase(t, sfCase{
			Code: `
func pick(flag) {
	for i = 0; i < 1; i++ {
		if (flag) {
			t = "loopReturnThen"
			return t
		}
		e = "loopReturnElse"
		return e
	}
	return "loopReturnTail"
}

x = pick(1)
println(x)
`,
			Rule: `
println(* as $sink)
$sink<dataflow(<<<CODE
	* #-> as $noFilter
CODE)>

t as $thenVal
$thenVal<getCfg> as $thenCfg
$sink<dataflow(<<<CODE
	* #-> as $thenPost
CODE, only_reachable="$thenCfg", only_reachable_mode="post")>

e as $elseVal
$elseVal<getCfg> as $elseCfg
$sink<dataflow(<<<CODE
	* #-> as $elsePost
CODE, only_reachable="$elseCfg", only_reachable_mode="post")>
`,
			Want: map[string][]string{
				"thenPost": {`"loopReturnThen"`},
				"elsePost": {`"loopReturnElse"`},
			},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

}
