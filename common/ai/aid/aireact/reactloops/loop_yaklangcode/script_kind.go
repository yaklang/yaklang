package loop_yaklangcode

import "strings"

// YakScriptKind classifies generated yak scripts for auto-run policy.
type YakScriptKind string

const (
	YakScriptKindUnknown       YakScriptKind = "unknown"
	YakScriptKindHookHotpatch    YakScriptKind = "hook_hotpatch"    // MITM / global / fuzzer hotpatch hooks
	YakScriptKindCodecPlugin     YakScriptKind = "codec_plugin"     // right-click codec handle(input)
	YakScriptKindNativePlugin    YakScriptKind = "native_plugin"    // yak plugin with cli + runPlugin
	YakScriptKindCLITool         YakScriptKind = "cli_tool"         // top-level cli.check + network/file IO
	YakScriptKindPureLogicScript YakScriptKind = "pure_logic"       // has runSelfTest or testable funcs, no live network in main
)

// yakHookVarNames are MixPluginCaller hook assignment names (mirror*/hijack*/beforeRequest/...).
var yakHookVarNames = []string{
	"mirrorHTTPFlow",
	"mirrorFilteredHTTPFlow",
	"mirrorNewWebsite",
	"mirrorNewWebsitePath",
	"mirrorNewWebsitePathParams",
	"hijackHTTPRequest",
	"hijackHTTPResponse",
	"hijackHTTPResponseEx",
	"hijackSaveHTTPFlow",
	"beforeRequest",
	"afterRequest",
	"mockHTTPRequest",
	"retryHandler",
}

// YakScriptRunPolicy describes whether/how the loop should run code after lint.
type YakScriptRunPolicy struct {
	Kind              YakScriptKind
	HasYAKMain        bool
	HasRunSelfTest    bool
	ShouldExecuteRun  bool // run YAK_MAIN self-test now
	BlockExitNoSelfTest bool // hook/codec missing YAK_MAIN block — ask AI to add runSelfTest
	SkipReason        string
	HintForAI         string
}

// ClassifyYakScriptRunPolicy decides auto-run behavior from full_code content.
func ClassifyYakScriptRunPolicy(code string) YakScriptRunPolicy {
	code = strings.TrimSpace(code)
	p := YakScriptRunPolicy{Kind: YakScriptKindUnknown}
	if code == "" {
		p.SkipReason = "empty code"
		return p
	}

	p.HasYAKMain = strings.Contains(code, "YAK_MAIN")
	p.HasRunSelfTest = strings.Contains(code, "runSelfTest")

	switch {
	case containsAnyAssign(code, yakHookVarNames...):
		p.Kind = YakScriptKindHookHotpatch
		p.HintForAI = hookHotpatchSelfTestHint()
	case containsAssignName(code, "handle"):
		p.Kind = YakScriptKindCodecPlugin
		p.HintForAI = codecPluginSelfTestHint()
	case strings.Contains(code, "cli.check()") || strings.Contains(code, "cli.Check()"):
		if p.HasRunSelfTest || strings.Contains(code, "runPlugin") {
			p.Kind = YakScriptKindNativePlugin
			p.HintForAI = nativePluginSelfTestHint()
		} else {
			p.Kind = YakScriptKindCLITool
			p.HintForAI = cliToolSelfTestHint()
		}
	case p.HasRunSelfTest:
		p.Kind = YakScriptKindPureLogicScript
	default:
		p.Kind = YakScriptKindUnknown
	}

	if p.HasYAKMain && (p.HasRunSelfTest || p.Kind == YakScriptKindHookHotpatch || p.Kind == YakScriptKindCodecPlugin) {
		p.ShouldExecuteRun = true
		return p
	}

	switch p.Kind {
	case YakScriptKindHookHotpatch, YakScriptKindCodecPlugin:
		p.BlockExitNoSelfTest = true
		p.SkipReason = "hook/codec script missing YAK_MAIN runSelfTest block"
	case YakScriptKindNativePlugin:
		if !p.HasYAKMain {
			p.BlockExitNoSelfTest = true
			p.SkipReason = "native plugin missing YAK_MAIN self-test guard"
		}
	case YakScriptKindCLITool:
		p.SkipReason = "CLI tool without YAK_MAIN runSelfTest — skip live run to avoid network/poll hang; add runSelfTest() for pure logic"
		if !p.HasYAKMain && !p.HasRunSelfTest && looksLikeTestableCLITool(code) {
			p.BlockExitNoSelfTest = true
		}
	case YakScriptKindPureLogicScript:
		if !p.HasYAKMain {
			p.BlockExitNoSelfTest = true
			p.SkipReason = "runSelfTest defined but not guarded by if YAK_MAIN"
		}
	default:
		if p.HasYAKMain {
			p.ShouldExecuteRun = true
		} else {
			p.SkipReason = "no YAK_MAIN self-test block"
		}
	}
	return p
}

// ShouldAutoRunYakSelfTest reports whether lint-clean code should enter YAK_MAIN execution.
func ShouldAutoRunYakSelfTest(code string) bool {
	return ClassifyYakScriptRunPolicy(code).ShouldExecuteRun
}

func containsAnyAssign(code string, names ...string) bool {
	for _, name := range names {
		if containsAssignName(code, name) {
			return true
		}
	}
	return false
}

func containsAssignName(code, name string) bool {
	return strings.Contains(code, name+" = func") ||
		strings.Contains(code, name+"=func") ||
		strings.Contains(code, name+" = (") ||
		strings.Contains(code, name+"=(")
}

func hookHotpatchSelfTestHint() string {
	return `热加载 Hook 脚本必须带 runSelfTest + if YAK_MAIN：
- mirror*：mock []byte req/rsp（双引号 "\r\n"），直接调 hook，assert 外层状态（如去重 map 长度）
- hijack*：传入 forward/drop 闭包捕获改写后的包，assert 字段变化
- map 计数：缺键读出来是 nil，禁止 counter[k]+1；先 n=counter[k]; if n==nil{n=0}; counter[k]=n+1
- beforeRequest/afterRequest：mock 请求包，assert 返回值或 body 变化
- mockHTTPRequest：传 mockResponse 回调，assert 拦截行为
- retryHandler：mock retry 回调统计调用次数
勿在自测里触真实 poc.HTTP/servicescan；hook 内对 err 容错。`
}

func codecPluginSelfTestHint() string {
	return `Codec 右键插件必须 handle(input) + runSelfTest + if YAK_MAIN：
自测用固定 sample 字符串调 handle(input)，assert 输出含预期片段；非法输入 assert 友好错误文案。`
}

func nativePluginSelfTestHint() string {
	return `Yakit 原生插件：把可测逻辑抽成纯函数，runSelfTest 只测纯函数；if YAK_MAIN { if len(cli.Args())>1 { runPlugin() } else { runSelfTest() } }。
自测不调 cli.check、不触网。`
}

func cliToolSelfTestHint() string {
	return `CLI 扫描/热更新监视器：顶层 cli.check() 会阻塞或无参失败，系统不会整脚本直跑。
请把核心逻辑抽成纯函数/可注入 load 函数，追加 runSelfTest + if YAK_MAIN 测纯逻辑；真实 --target 运行留给用户。
time.Sleep 参数单位是秒(float)，如 time.Sleep(0.4)；AfterFunc 才用 time.ParseDuration("300ms")~。`
}

// looksLikeTestableCLITool detects CLI scripts with extractable func logic worth unit-testing.
func looksLikeTestableCLITool(code string) bool {
	return strings.Contains(code, "= func") ||
		strings.Contains(code, "func load") ||
		strings.Contains(code, "func run") ||
		strings.Contains(code, "dyn.Eval")
}
