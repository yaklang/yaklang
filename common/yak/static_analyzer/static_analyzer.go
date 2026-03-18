package static_analyzer

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalyzer"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yak/yaklang"

	// for init function
	_ "github.com/yaklang/yaklang/common/yak/static_analyzer/rules"
	_ "github.com/yaklang/yaklang/common/yak/static_analyzer/ssa_option"
)

type StaticAnalyzeKind uint8

const (
	Analyze StaticAnalyzeKind = iota
	Score
	Compile
)

func YaklangScriptChecking(code, pluginType string) []*result.StaticAnalyzeResult {
	return StaticAnalyzeWithContext(context.Background(), code, pluginType, Analyze)
}

// SyntaxFlowRuleChecking performs syntax check on SyntaxFlow rule content
func SyntaxFlowRuleChecking(code string) []*result.StaticAnalyzeResult {
	return StaticAnalyzeWithContext(context.Background(), code, "syntaxflow", Analyze)
}

func StaticAnalyze(code, codeTyp string, kind StaticAnalyzeKind) []*result.StaticAnalyzeResult {
	return StaticAnalyzeWithContext(context.Background(), code, codeTyp, kind)
}

func init() {
	ssaapi.RegisterExport("YaklangScriptChecking", YaklangScriptChecking)
	ssaapi.RegisterExport("SyntaxFlowRuleChecking", SyntaxFlowRuleChecking)
}

// plugin type : "yak" "mitm" "port-scan" "codec" "syntaxflow"

func StaticAnalyzeWithContext(ctx context.Context, code, codeTyp string, kind StaticAnalyzeKind) []*result.StaticAnalyzeResult {
	var results []*result.StaticAnalyzeResult
	addSourceCodeError := func(errs antlr4util.SourceCodeErrors) {
		for _, e := range errs {
			results = append(results, &result.StaticAnalyzeResult{
				Message:         fmt.Sprintf("基础语法错误（Syntax Error）：%v", e.Message),
				Severity:        result.Error,
				StartLineNumber: int64(e.StartPos.GetLine()),
				StartColumn:     int64(e.StartPos.GetColumn()),
				EndLineNumber:   int64(e.EndPos.GetLine()),
				EndColumn:       int64(e.EndPos.GetColumn() + 1),
				From:            "compiler",
			})
		}
	}

	select {
	case <-ctx.Done():
		log.Info("Static analysis cancelled before start")
		return results
	default:
	}

	// lifecycle: 仅 LevelHigh 时创建 recorder 并输出表格，节约内存（与 AST/Build/Scan/Database 统一）
	var perfRecorder *diagnostics.Recorder
	if diagnostics.Enabled(diagnostics.LevelHigh) {
		perfRecorder = diagnostics.NewRecorder()
	}
	defer func() {
		if rec := perfRecorder; rec != nil {
			snap := rec.Snapshot()
			if len(snap) > 0 {
				payload := diagnostics.TablePayloadFromMeasurements("Static Analysis Performance", snap)
				diagnostics.LogTable(ssa.TrackKindStaticAnalyze, payload, true)
			}
		}
	}()

	// compiler
	switch codeTyp {
	case "yak", "mitm", "port-scan", "codec":
		// Yaklang 编译
		var err error
		runYakCompile := func() error {
			newEngine := yaklang.New()
			newEngine.SetStrictMode(false)
			_, e := newEngine.Compile(code)
			return e
		}
		if perfRecorder != nil {
			_, err = perfRecorder.ForKind(ssa.TrackKindStaticAnalyze).Track("Yaklang StaticAnalyze", runYakCompile)
		} else {
			err = diagnostics.RunStepsWithoutRecording([]func() error{runYakCompile})
		}

		select {
		case <-ctx.Done():
			log.Info("Static analysis cancelled after Yaklang compile")
			return results
		default:
		}

		if err != nil {
			switch ret := err.(type) {
			case antlr4util.SourceCodeErrors:
				addSourceCodeError(ret)
			default:
				log.Error("静态分析失败：Yaklang 返回错误不标准")
			}
		}

		prog, err := SSAParseWithPerf(code, codeTyp, perfRecorder)

		select {
		case <-ctx.Done():
			log.Info("Static analysis cancelled after SSA compile")
			return results
		default:
		}

		if err != nil {
			log.Error("SSA 解析失败：", err)
			return results
		}

		ruleCheck := func() error {
			results = append(results, checkRules(codeTyp, prog, kind).Get()...)
			return nil
		}
		if perfRecorder != nil {
			_, _ = perfRecorder.ForKind(ssa.TrackKindStaticAnalyze).Track("Rule Check", ruleCheck)
		} else {
			_ = diagnostics.RunStepsWithoutRecording([]func() error{ruleCheck})
		}

		select {
		case <-ctx.Done():
			log.Info("Static analysis cancelled after rule check")
			return results
		default:
		}

		errs := prog.GetErrors()
		for _, err := range errs {
			severity := result.Hint
			switch err.Kind {
			case ssa.Warn:
				severity = result.Warn
			case ssa.Error:
				severity = result.Error
			}
			results = append(results, &result.StaticAnalyzeResult{
				Message:         err.Message,
				Severity:        severity,
				StartLineNumber: int64(err.Pos.GetStart().GetLine()),
				StartColumn:     int64(err.Pos.GetStart().GetColumn()),
				EndLineNumber:   int64(err.Pos.GetEnd().GetLine()),
				EndColumn:       int64(err.Pos.GetEnd().GetColumn()),
				From:            "SSA:" + string(err.Tag),
			})
		}
	case "syntaxflow":
		// 第1步：语法检测（必须通过，富集错误信息）
		var syntaxErrs []*result.StaticAnalyzeResult
		var frame *sfvm.SFFrame
		if perfRecorder != nil {
			_, _ = perfRecorder.ForKind(ssa.TrackKindStaticAnalyze).Track("SyntaxFlow Compile", func() error {
				syntaxErrs, frame = syntaxFlowCompileAndCheck(code)
				return nil
			})
		} else {
			syntaxErrs, frame = syntaxFlowCompileAndCheck(code)
		}

		if len(syntaxErrs) > 0 {
			// 富集：添加结构化错误 + 富格式提示
			results = append(results, syntaxErrs...)
			results = append(results, &result.StaticAnalyzeResult{
				Message:         "【SyntaxFlow 语法错误】\n" + FormatSyntaxFlowErrors(code, syntaxErrs),
				Severity:        result.Error,
				StartLineNumber: 0,
				StartColumn:     0,
				EndLineNumber:   0,
				EndColumn:       1,
				From:            "syntaxflow_formatted",
			})
			return results
		}

		// 第2步：规则正确性检测（内嵌 file://、UNSAFE 正反例，必须通过，富集错误信息）
		if err := sfanalyzer.EvaluateVerifyFilesystemWithFrame(frame); err != nil {
			results = append(results, &result.StaticAnalyzeResult{
				Message:         fmt.Sprintf("【规则正确性检测失败】正反例验证未通过：%v", err),
				Severity:        result.Error,
				StartLineNumber: 0,
				StartColumn:     0,
				EndLineNumber:   0,
				EndColumn:       1,
				From:            "syntaxflow_verify",
			})
			return results
		}

		// 第3步：规则打分（直接打印结果，不阻塞，供用户参考）
		_ = sfanalyzer.NewSyntaxFlowAnalyzer(code).Analyze() // Analyze 内部会 log.Info 输出评分
	default:
		log.Error("静态分析失败：未知的代码类型")
	}
	return results
}

func GetPluginSSAOpt(plugin string) []ssaconfig.Option {
	ret := plugin_type.GetPluginSSAOpt(plugin_type.PluginTypeYak)
	pluginType := plugin_type.ToPluginType(plugin)
	if pluginType != plugin_type.PluginTypeYak {
		ret = append(ret, plugin_type.GetPluginSSAOpt(pluginType)...)
	}
	return ret
}

func checkRules(plugin string, prog *ssaapi.Program, kind StaticAnalyzeKind) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults()
	switch kind {
	case Score:
		ret.Merge(plugin_type.CheckScoreRules(plugin_type.PluginTypeYak, prog))
		pluginType := plugin_type.ToPluginType(plugin)
		if pluginType != plugin_type.PluginTypeYak {
			ret.Merge(plugin_type.CheckScoreRules(pluginType, prog))
		}
		fallthrough
	default:
		ret.Merge(plugin_type.CheckRules(plugin_type.PluginTypeYak, prog))
		pluginType := plugin_type.ToPluginType(plugin)
		if pluginType != plugin_type.PluginTypeYak {
			ret.Merge(plugin_type.CheckRules(pluginType, prog))
		}
	}

	return ret
}

func SSAParse(code, scriptType string, o ...ssaconfig.Option) (*ssaapi.Program, error) {
	opt := GetPluginSSAOpt(scriptType)
	opt = append(opt, ssaapi.WithEnableCache(true))
	opt = append(opt, o...)
	return ssaapi.Parse(code, opt...)
}

// SSAParseWithPerf 带性能记录的 SSA 解析，通过显式 WithDiagnosticsRecorder 传入 recorder
func SSAParseWithPerf(code, scriptType string, perfRecorder *diagnostics.Recorder, o ...ssaconfig.Option) (*ssaapi.Program, error) {
	opt := GetPluginSSAOpt(scriptType)
	opt = append(opt, ssaapi.WithEnableCache(true))
	opt = append(opt, o...)
	if perfRecorder != nil {
		opt = append(opt, ssaapi.WithDiagnosticsRecorder(perfRecorder))
	}
	return ssaapi.Parse(code, opt...)
}
