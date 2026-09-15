package syntaxflow_scan_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	_ "github.com/yaklang/yaklang/common/yak/ssa_compile"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestScanProject_CompilesAndRunsSourceFromSnapshot(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.yak"), []byte("key = \"AKIAIOSFODNN7EXAMPLE\"\n"), 0o644))

	var alerts int
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("yak"),
		ssaconfig.WithSetProgramName(t.Name()),
		// Explicit mode: an empty mode list is compile-only.
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: general, title: "scan-project source")
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			Language: string(ssaconfig.General),
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0)
}

// Compile-only runs persist IR and report the compile stage without executing
// any rule set, so a platform can compile once and scan later.
func TestScanProject_CompileOnlyReportsProgramName(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("def run(x):\n    return eval(x)\n"), 0o644))

	// Unique name per run: a reused worktree-local IR DB must not make a stale
	// program satisfy this test.
	programName := fmt.Sprintf("%s-%s", t.Name(), uuid.NewString())
	var result *syntaxflow_scan.ProjectResult
	var stages []string
	var alerts int
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithSetProgramName(programName),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: python, title: "must not run")
eval(* as $arg) as $call
alert $call`,
			Language: "python",
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		syntaxflow_scan.WithStageCallback(func(stage syntaxflow_scan.ProductStage, overall, progress float64, info *syntaxflow_scan.RuleProcessInfoList) {
			if progress == 0 || progress == 1 {
				stages = append(stages, string(stage))
			}
		}),
		syntaxflow_scan.WithProjectResultCallback(func(r syntaxflow_scan.ProjectResult) {
			copied := r
			result = &copied
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Succeeded, "compile-only run must succeed")
	require.Equal(t, programName, result.ProgramName)
	require.Empty(t, alerts, "compile-only must not execute rule sets")
	require.Contains(t, stages, string(syntaxflow_scan.StageCompile))
	require.NotContains(t, stages, string(syntaxflow_scan.StageAnalyze))

	// The persisted IR must be reloadable by name: that is the contract a
	// platform relies on to reuse a compile-only run for a later scan.
	reloaded, reloadErr := ssaapi.FromDatabase(result.ProgramName)
	require.NoError(t, reloadErr)
	require.NotNil(t, reloaded, "compile-only must persist a reloadable program")
	require.True(t, reloaded.IsFromDatabase(), "reloaded program must come from the IR DB")
	require.Positive(t, reloaded.TotalLines(), "persisted IR must carry compiled source lines")

	var reported []syntaxflow_scan.StageOutcome
	for _, outcome := range result.Stages {
		if outcome.Succeeded() {
			reported = append(reported, outcome)
		}
	}
	require.Len(t, reported, 2, "collect + compile are the only successful stages")
	require.Equal(t, syntaxflow_scan.StageCollect, reported[0].Stage)
	require.Equal(t, syntaxflow_scan.StageCompile, reported[1].Stage)
}

func TestScanProject_EmitsProductStages(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("eval(user)\n"), 0o644))

	var stages []string
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithSetProgramName(t.Name()),
		// Explicit mode: an empty mode list is compile-only.
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: python, title: "stage source")
${*.py}.pattern_regex(/eval\s*\(/) as $hit
alert $hit`,
			Language: "python",
		}),
		syntaxflow_scan.WithStageCallback(func(stage syntaxflow_scan.ProductStage, overall, progress float64, info *syntaxflow_scan.RuleProcessInfoList) {
			if progress == 0 || progress == 1 {
				stages = append(stages, string(stage))
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Contains(t, stages, string(syntaxflow_scan.StageCollect))
	require.Contains(t, stages, string(syntaxflow_scan.StageInspect))
	require.Equal(t, "收集代码", syntaxflow_scan.StageCollect.DisplayName())
	require.Equal(t, "代码检测", syntaxflow_scan.StageInspect.DisplayName())
	require.Equal(t, "语义检测", syntaxflow_scan.StageReview.DisplayName())
	require.Equal(t, "深度分析", syntaxflow_scan.StageAnalyze.DisplayName())
}

func TestScanProject_ExternalStructRule(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("def run(x):\n    return eval(x)\n"), 0o644))

	var alerts int
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithSetProgramName(t.Name()),
		// Explicit mode: an empty mode list is compile-only.
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(
	mode: "struct"
	language: "python"
	title: "external struct eval"
)
eval(* as $arg) as $call
alert $call`,
			Language: "python",
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0)
}

func TestScanProjectFromJSON_UsesConfigBlob(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.yak"), []byte("key = \"AKIAIOSFODNN7EXAMPLE\"\n"), 0o644))

	raw := fmt.Sprintf(`{
		"Mode": 127,
		"CodeSource": {"kind": "local", "local_file": %q},
		"BaseInfo": {"language": "yak", "program_names": [%q]}
	}`, dir, t.Name())

	var alerts int
	err := syntaxflow_scan.ScanProjectFromJSON(context.Background(), raw,
		// Explicit mode: an empty mode list is compile-only.
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: general, title: "json source")
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0)
}

func TestScanProject_WithModeSourceOnly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.yak"), []byte("key = \"AKIAIOSFODNN7EXAMPLE\"\n"), 0o644))

	var alerts int
	var stages []string
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("yak"),
		ssaconfig.WithSetProgramName(t.Name()),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: general, title: "source-only")
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			Language: string(ssaconfig.General),
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		syntaxflow_scan.WithStageCallback(func(stage syntaxflow_scan.ProductStage, overall, progress float64, info *syntaxflow_scan.RuleProcessInfoList) {
			if progress == 0 || progress == 1 {
				stages = append(stages, string(stage))
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0)
	require.Contains(t, stages, string(syntaxflow_scan.StageInspect))
	require.NotContains(t, stages, string(syntaxflow_scan.StageReview))
	require.NotContains(t, stages, string(syntaxflow_scan.StageAnalyze))
}

func TestScanProject_WithModeStackedSourceAndStruct(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("def run(x):\n    return eval(x)\n"), 0o644))

	var stages []string
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithSetProgramName(t.Name()),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(
	mode: "struct"
	language: "python"
	title: "stacked struct eval"
)
eval(* as $arg) as $call
alert $call`,
			Language: "python",
		}),
		syntaxflow_scan.WithStageCallback(func(stage syntaxflow_scan.ProductStage, overall, progress float64, info *syntaxflow_scan.RuleProcessInfoList) {
			if progress == 0 || progress == 1 {
				stages = append(stages, string(stage))
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Contains(t, stages, string(syntaxflow_scan.StageInspect))
	require.Contains(t, stages, string(syntaxflow_scan.StageReview))
	require.NotContains(t, stages, string(syntaxflow_scan.StageAnalyze))
}

func TestScanProject_ProgramPathSourceFromIrSource(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.yak", "key = \"AKIAIOSFODNN7EXAMPLE\"\n")
	progs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithProgramName(t.Name()))
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	var alerts int
	var stages []string
	err = syntaxflow_scan.ScanProject(context.Background(),
		syntaxflow_scan.WithPrograms(progs[0]),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: general, title: "program irsource")
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			Language: string(ssaconfig.General),
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		syntaxflow_scan.WithStageCallback(func(stage syntaxflow_scan.ProductStage, overall, progress float64, info *syntaxflow_scan.RuleProcessInfoList) {
			if progress == 0 || progress == 1 {
				stages = append(stages, string(stage))
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0)
	require.Contains(t, stages, string(syntaxflow_scan.StageInspect))
	require.NotContains(t, stages, string(syntaxflow_scan.StageReview))
	require.NotContains(t, stages, string(syntaxflow_scan.StageAnalyze))
}

func TestScanProject_TargetReloadsThenRunsSSA(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "A.java"), []byte(`class A {
	public static void main(String[] args) {
		Runtime.getRuntime().exec(args[0]);
	}
}
`), 0o644))

	var alerts int
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("java"),
		ssaconfig.WithSetProgramName(t.Name()),
		// Explicit mode: an empty mode list is compile-only.
		syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: java, title: "target reload ssa")
Runtime.getRuntime().exec(* as $cmd)
alert $cmd`,
			Language: "java",
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0, "SSA after -t must run on the reloaded DB program")
}

func TestScanProject_ProgramPathRunsStructWithoutCompile(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("app.py", "def run(x):\n    return eval(x)\n")
	progs, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.PYTHON),
		ssaapi.WithProgramName(t.Name()),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	orig := syntaxflow_scan.CompileProject
	t.Cleanup(func() { syntaxflow_scan.CompileProject = orig })
	syntaxflow_scan.CompileProject = func(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
		t.Fatal("code-scan -p must not compile")
		return nil, fmt.Errorf("must not compile")
	}

	var alerts int
	var stages []string
	err = syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithProgramNames(t.Name()),
		// Explicit mode: an empty mode list is compile-only.
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode, syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(
	mode: "struct"
	language: "python"
	title: "program struct eval"
)
eval(* as $arg) as $call
alert $call`,
			Language: "python",
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		syntaxflow_scan.WithStageCallback(func(stage syntaxflow_scan.ProductStage, overall, progress float64, info *syntaxflow_scan.RuleProcessInfoList) {
			if progress == 0 || progress == 1 {
				stages = append(stages, string(stage))
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0, "code-scan -p must struct-scan the loaded program")
	require.Contains(t, stages, string(syntaxflow_scan.StageReview))
	require.Contains(t, stages, string(syntaxflow_scan.StageInspect))
	require.Contains(t, stages, string(syntaxflow_scan.StageAnalyze))
}

func TestScanProject_NamedProgramFromDatabase(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("A.java", `class A {
	public static void main(String[] args) {
		Runtime.getRuntime().exec(args[0]);
	}
}
`)
	progs, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(t.Name()),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	var alerts int
	err = syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithProgramNames(t.Name()),
		syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: java, title: "named program exec")
Runtime.getRuntime().exec(* as $cmd)
alert $cmd`,
			Language: "java",
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0)
}
