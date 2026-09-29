package syntaxflow_scan_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func writeEvalPythonProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("def run(x):\n    return eval(x)\n"), 0o644))
	return dir
}

func productPipelineRuleOptions() []ssaconfig.Option {
	return []ssaconfig.Option{
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: python, title: "pipeline source eval")
${*.py}.pattern_regex(/eval\s*\(/) as $hit
alert $hit`,
			Language: "python",
		}),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "struct", language: "python", title: "pipeline struct eval")
eval(* as $arg) as $call
alert $call`,
			Language: "python",
		}),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: python, title: "pipeline ssa eval")
eval(* as $arg) as $call
alert $call`,
			Language: "python",
		}),
	}
}

func wrapCompileTimeline(t *testing.T, timeline *[]string, onStart func(cfg *ssaconfig.Config, extra []ssaconfig.Option)) {
	t.Helper()
	orig := syntaxflow_scan.CompileProject
	t.Cleanup(func() { syntaxflow_scan.CompileProject = orig })
	syntaxflow_scan.CompileProject = func(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
		*timeline = append(*timeline, "compile:start")
		if onStart != nil {
			onStart(cfg, extra)
		}
		prog, err := orig(ctx, cfg, extra...)
		*timeline = append(*timeline, "compile:end")
		return prog, err
	}
}

func appendStageTimeline(timeline *[]string) syntaxflow_scan.StageCallback {
	return func(stage syntaxflow_scan.ProductStage, overall, progress float64, info *syntaxflow_scan.RuleProcessInfoList) {
		if progress == 0 {
			*timeline = append(*timeline, string(stage)+":start")
		}
		if progress == 1 {
			*timeline = append(*timeline, string(stage)+":end")
		}
	}
}

func mustIndex(t *testing.T, events []string, name string) int {
	t.Helper()
	for i, event := range events {
		if event == name {
			return i
		}
	}
	t.Fatalf("event %q not in timeline: %#v", name, events)
	return -1
}

func compileExtrasHaveStructRule(t *testing.T, extra []ssaconfig.Option) bool {
	t.Helper()
	dummy, err := ssaconfig.New(ssaconfig.ModeAll)
	require.NoError(t, err)
	for _, opt := range extra {
		if opt == nil {
			continue
		}
		require.NoError(t, opt(dummy))
	}
	_, ok := dummy.GetExtraInfo("ssa_compile/struct_rule_raw")
	return ok
}

func requireInspectThenCompileReviewThenAnalyze(t *testing.T, result syntaxflow_scan.ProjectResult, timeline []string, alerts map[string]int) {
	t.Helper()
	require.True(t, result.Succeeded)
	require.Equal(t, []string{
		string(syntaxflow_scan.StageCollect),
		string(syntaxflow_scan.StageInspect),
		string(syntaxflow_scan.StageReview),
		string(syntaxflow_scan.StageAnalyze),
	}, stageNames(result), "product stages must be 收集 → 源码分析 → 语义检测 → 深度扫描")
	require.NotContains(t, stageNames(result), string(syntaxflow_scan.StageCompile),
		"review owns compile; a standalone 编译源码 stage must not appear")

	inspectEnd := mustIndex(t, timeline, "inspect:end")
	compileStart := mustIndex(t, timeline, "compile:start")
	compileEnd := mustIndex(t, timeline, "compile:end")
	reviewStart := mustIndex(t, timeline, "review:start")
	analyzeStart := mustIndex(t, timeline, "analyze:start")
	require.Less(t, inspectEnd, compileStart, "源码分析 must finish before compile starts")
	require.Less(t, reviewStart, compileEnd, "语义检测 must run during compile")
	require.Less(t, compileEnd, analyzeStart, "深度扫描 must start after compile finishes")

	require.Greater(t, alerts[string(schema.SFR_MODE_SOURCE)], 0, "source analysis must emit hits")
	require.Greater(t, alerts[string(schema.SFR_MODE_SSA)], 0, "deep scan must emit hits")
	var review syntaxflow_scan.StageOutcome
	for _, outcome := range result.Stages {
		if outcome.Stage == syntaxflow_scan.StageReview {
			review = outcome
		}
	}
	require.True(t, review.Succeeded())
	require.Greater(t, review.RuleCount+review.RiskCount+int64(alerts[string(schema.SFR_MODE_STRUCT)]), int64(0),
		"semantic analysis during compile must run struct rules")
}

func scanProductPipeline(t *testing.T, sourceOpts []ssaconfig.Option, timeline *[]string, onCompile func(cfg *ssaconfig.Config, extra []ssaconfig.Option)) (syntaxflow_scan.ProjectResult, map[string]int) {
	t.Helper()
	wrapCompileTimeline(t, timeline, onCompile)
	alerts := map[string]int{}
	opts := []ssaconfig.Option{
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithSetProgramName(fmt.Sprintf("%s-%s", t.Name(), uuid.NewString())),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode, syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
		syntaxflow_scan.WithStageCallback(appendStageTimeline(timeline)),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r == nil || r.Result == nil || r.Result.GetRule() == nil {
				return
			}
			if len(r.Result.GetAlertVariables()) == 0 && r.Result.RiskCount() == 0 {
				return
			}
			alerts[string(r.Result.GetRule().Mode)]++
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	}
	opts = append(opts, sourceOpts...)
	opts = append(opts, productPipelineRuleOptions()...)
	result, err := syntaxflow_scan.ScanProject(context.Background(), opts...)
	require.NoError(t, err)
	return result, alerts
}

// Local source: 源码分析 on files, 语义检测 during compile, then 深度扫描 on IR.
func TestScanProject_LocalProductPipelineInspectThenCompileReviewThenAnalyze(t *testing.T) {
	dir := writeEvalPythonProject(t)
	var timeline []string
	var compileSawStruct bool
	result, alerts := scanProductPipeline(t,
		[]ssaconfig.Option{
			ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
			ssaconfig.WithCodeSourceLocalFile(dir),
		},
		&timeline,
		func(cfg *ssaconfig.Config, extra []ssaconfig.Option) {
			compileSawStruct = compileExtrasHaveStructRule(t, extra)
		},
	)
	require.True(t, compileSawStruct, "compile must receive struct rules for 语义检测")
	requireInspectThenCompileReviewThenAnalyze(t, result, timeline, alerts)
}

// Git/zip collect first, inspect the tree, then compile that same directory
// with struct rules, then SSA. Compile must not run before source analysis.
func TestScanProject_GitProductPipelineInspectThenCompileReviewThenAnalyze(t *testing.T) {
	dir := writeEvalPythonProject(t)
	origCollect := syntaxflow_scan.CollectCodeSourceDir
	t.Cleanup(func() { syntaxflow_scan.CollectCodeSourceDir = origCollect })
	syntaxflow_scan.CollectCodeSourceDir = func(ctx context.Context, cfg *ssaconfig.Config) (string, error) {
		return dir, nil
	}

	var timeline []string
	var compileKind ssaconfig.CodeSourceKind
	var compileLocal string
	var compileSawStruct bool
	result, alerts := scanProductPipeline(t,
		[]ssaconfig.Option{
			ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceGit),
			ssaconfig.WithCodeSourceURL("https://example.invalid/repo.git"),
		},
		&timeline,
		func(cfg *ssaconfig.Config, extra []ssaconfig.Option) {
			compileKind = cfg.GetCodeSourceKind()
			compileLocal = cfg.GetCodeSourceLocalFile()
			compileSawStruct = compileExtrasHaveStructRule(t, extra)
		},
	)
	require.Equal(t, ssaconfig.CodeSourceLocal, compileKind, "compile must reuse the collected tree, not clone again")
	require.Equal(t, dir, compileLocal)
	require.True(t, compileSawStruct, "compile must receive struct rules for 语义检测")
	requireInspectThenCompileReviewThenAnalyze(t, result, timeline, alerts)
}

func TestScanProject_ZipProductPipelineInspectThenCompileReviewThenAnalyze(t *testing.T) {
	zipPath := writeTestZip(t, map[string]string{
		"app.py": "def run(x):\n    return eval(x)\n",
	})
	var timeline []string
	var compileSawStruct bool
	result, alerts := scanProductPipeline(t,
		[]ssaconfig.Option{
			ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
			ssaconfig.WithCodeSourceLocalFile(zipPath),
		},
		&timeline,
		func(cfg *ssaconfig.Config, extra []ssaconfig.Option) {
			compileSawStruct = compileExtrasHaveStructRule(t, extra)
			require.Equal(t, ssaconfig.CodeSourceLocal, cfg.GetCodeSourceKind(), "zip inspect must extract to a directory before compile")
			info, err := os.Stat(cfg.GetCodeSourceLocalFile())
			require.NoError(t, err)
			require.True(t, info.IsDir())
		},
	)
	require.True(t, compileSawStruct, "compile must receive struct rules for 语义检测")
	requireInspectThenCompileReviewThenAnalyze(t, result, timeline, alerts)
}

// Review-only still compiles, but that compile is 语义检测, not a standalone
// compile stage, and source analysis must not run.
func TestScanProject_ReviewOnlyRunsStructDuringCompile(t *testing.T) {
	dir := writeEvalPythonProject(t)
	var timeline []string
	var compileSawStruct bool
	wrapCompileTimeline(t, &timeline, func(cfg *ssaconfig.Config, extra []ssaconfig.Option) {
		compileSawStruct = compileExtrasHaveStructRule(t, extra)
	})

	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithSetProgramName(fmt.Sprintf("%s-%s", t.Name(), uuid.NewString())),
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "struct", language: "python", title: "review only eval")
eval(* as $arg) as $call
alert $call`,
			Language: "python",
		}),
		syntaxflow_scan.WithStageCallback(appendStageTimeline(&timeline)),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.True(t, result.Succeeded)
	require.True(t, compileSawStruct)
	require.Equal(t, []string{
		string(syntaxflow_scan.StageCollect),
		string(syntaxflow_scan.StageReview),
	}, stageNames(result))
	require.NotContains(t, stageNames(result), string(syntaxflow_scan.StageInspect))
	require.NotContains(t, stageNames(result), string(syntaxflow_scan.StageAnalyze))
	require.NotContains(t, stageNames(result), string(syntaxflow_scan.StageCompile))
	require.Less(t, mustIndex(t, timeline, "review:start"), mustIndex(t, timeline, "compile:end"))
	require.Less(t, mustIndex(t, timeline, "compile:start"), mustIndex(t, timeline, "review:end"))
}
