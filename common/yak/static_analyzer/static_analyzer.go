package static_analyzer

import (
	"context"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/log"
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

func StaticAnalyze(code, codeTyp string, kind StaticAnalyzeKind) []*result.StaticAnalyzeResult {
	return StaticAnalyzeWithContext(context.Background(), code, codeTyp, kind)
}

func init() {
	ssaapi.RegisterExport("YaklangScriptChecking", YaklangScriptChecking)
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

	perfRecorder := diagnostics.NewRecorder()
	defer func() {
		snapshots := perfRecorder.Snapshot()
		if len(snapshots) > 0 {
			table := diagnostics.FormatPerformanceTable("Static Analysis Performance", snapshots)
			fmt.Println(table)
		}
	}()

	// compiler
	switch codeTyp {
	case "yak", "mitm", "port-scan", "codec":
		// Yaklang 编译
		yaklangStart := time.Now()
		newEngine := yaklang.New()
		newEngine.SetStrictMode(false)
		_, err := newEngine.Compile(code)
		yaklangDuration := time.Since(yaklangStart)
		perfRecorder.RecordDuration("Yaklang StaticAnalyze", yaklangDuration)

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

		ruleStart := time.Now()
		results = append(results, checkRules(codeTyp, prog, kind).Get()...)
		ruleDuration := time.Since(ruleStart)
		perfRecorder.RecordDuration("Rule Check", ruleDuration)

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
		sfStart := time.Now()
		vm := sfvm.NewSyntaxFlowVirtualMachine()
		vm.Compile(code)
		sfDuration := time.Since(sfStart)
		perfRecorder.RecordDuration("SyntaxFlow Compile", sfDuration)

		errs := vm.GetErrors()
		if errs != nil {
			addSourceCodeError(errs)
		}
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

// SSAParseWithPerf 带性能记录的 SSA 解析
func SSAParseWithPerf(code, scriptType string, perfRecorder *diagnostics.Recorder, o ...ssaconfig.Option) (*ssaapi.Program, error) {
	opt := GetPluginSSAOpt(scriptType)
	opt = append(opt, ssaapi.WithEnableCache(true))
	opt = append(opt, o...)

	// 如果提供了性能记录器，启用 diagnostics 并设置记录器
	if perfRecorder != nil {
		opt = append(opt, ssaapi.WithDiagnostics(true))
		// 创建一个选项来设置性能记录器，使用 ssaconfig.SetOption 创建接收 *Config 的选项
		setPerfRecorderOpt := ssaconfig.SetOption("ssa_compile/perf_recorder", func(c *ssaapi.Config, rec *diagnostics.Recorder) {
			c.SetDiagnosticsRecorder(rec)
		})
		opt = append(opt, setPerfRecorderOpt(perfRecorder))
	}

	// 使用公开的 Parse 方法
	return ssaapi.Parse(code, opt...)
}
