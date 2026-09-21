package syntaxflow_scan_test

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// writeTestZip builds a tiny zip archive on disk, mirroring CI's `-t ./fs.zip`.
func writeTestZip(t *testing.T, files map[string]string) string {
	t.Helper()
	zipPath := filepath.Join(t.TempDir(), "fs.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	for name, content := range files {
		entry, err := w.Create(name)
		require.NoError(t, err)
		_, err = entry.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
	return zipPath
}

// A local archive target must compile through the zip code source instead of
// being walked as a directory: `-t ./fs.zip` used to fail with
// "root path is not a directory: ." and abort the whole scan.
func TestScanProject_LocalZipTargetCompilesInsteadOfWalkingDirectory(t *testing.T) {
	zipPath := writeTestZip(t, map[string]string{
		"main.go": "package main\n\nvar key = \"AKIAIOSFODNN7EXAMPLE\"\n",
	})

	var alerts int
	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(zipPath),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName(fmt.Sprintf("%s-%s", t.Name(), uuid.NewString())),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: general, title: "zip source")
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
	require.True(t, result.Succeeded)
	require.Greater(t, alerts, 0, "source rules must run against the zip snapshot")
}

// A local jar target must compile through the jar code source (java archive),
// again without treating the file as a project directory.
func TestScanProject_LocalJarTargetCompilesAsJar(t *testing.T) {
	jarPath, err := ssatest.GetJarFile()
	require.NoError(t, err)

	programName := fmt.Sprintf("%s-%s", t.Name(), uuid.NewString())
	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(jarPath),
		ssaconfig.WithProjectRawLanguage("java"),
		ssaconfig.WithSetProgramName(programName),
	)
	require.NoError(t, err)
	require.True(t, result.Succeeded)
	require.Equal(t, programName, result.ProgramName)
}

// Local directories must keep the live-source path: classification changes the
// kind, not the pipeline.
func TestScanProject_LocalDirectoryStaysLiveSource(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("eval(user)\n"), 0o644))

	var stages []string
	_, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithSetProgramName(fmt.Sprintf("%s-%s", t.Name(), uuid.NewString())),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: python, title: "dir source")
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
}

func TestScanProject_CompilesAndRunsSourceFromSnapshot(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.yak"), []byte("key = \"AKIAIOSFODNN7EXAMPLE\"\n"), 0o644))

	var alerts int
	_, err := syntaxflow_scan.ScanProject(context.Background(),
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
	var stages []string
	var alerts int
	result, err := syntaxflow_scan.ScanProject(context.Background(),
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
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.True(t, result.Succeeded, "compile-only run must succeed")
	require.Equal(t, programName, result.ProgramName)

	// The returned stage list is the authoritative "what ran" answer: no
	// inspect/analyze outcome exists, so a platform renders successful stages
	// directly instead of inferring them.
	require.Len(t, result.Stages, 2)
	require.Equal(t, syntaxflow_scan.StageCollect, result.Stages[0].Stage)
	require.Equal(t, syntaxflow_scan.StageCompile, result.Stages[1].Stage)
	require.True(t, result.Stages[1].Succeeded())
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
	projectResult, err := syntaxflow_scan.ScanProject(context.Background(),
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

	// The returned stages must carry per-stage evidence, not just status: that
	// is what a platform renders instead of re-deriving stage state.
	var inspectOutcome *syntaxflow_scan.StageOutcome
	for i := range projectResult.Stages {
		if projectResult.Stages[i].Stage == syntaxflow_scan.StageInspect {
			inspectOutcome = &projectResult.Stages[i]
		}
	}
	require.NotNil(t, inspectOutcome, "inspect stage must be reported")
	require.True(t, inspectOutcome.Succeeded())
	require.NotEmpty(t, inspectOutcome.RuleCount, "inspect must report how many rules ran")
	require.Equal(t, "收集代码", syntaxflow_scan.StageCollect.DisplayName())
	require.Equal(t, "代码检测", syntaxflow_scan.StageInspect.DisplayName())
	require.Equal(t, "语义检测", syntaxflow_scan.StageReview.DisplayName())
	require.Equal(t, "深度分析", syntaxflow_scan.StageAnalyze.DisplayName())
}

func TestScanProject_ExternalStructRule(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("def run(x):\n    return eval(x)\n"), 0o644))

	var alerts int
	_, err := syntaxflow_scan.ScanProject(context.Background(),
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
	_, err := syntaxflow_scan.ScanProjectFromJSON(context.Background(), raw,
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
	_, err := syntaxflow_scan.ScanProject(context.Background(),
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

// A run without struct mode still compiles for SSA, but that compile must not
// be reported as a successful 语义检测 stage: the product result lists what
// actually ran, so a phantom review stage would misinform the operator.
func TestScanProject_SSAOnlyDoesNotReportReview(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("def run(x):\n    return eval(x)\n"), 0o644))

	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithSetProgramName(t.Name()),
		syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: python, title: "ssa only")
eval(* as $arg) as $call
alert $call`,
			Language: "python",
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.True(t, result.Succeeded)

	stageKeys := make([]string, 0, len(result.Stages))
	for _, outcome := range result.Stages {
		stageKeys = append(stageKeys, string(outcome.Stage))
	}
	require.Contains(t, stageKeys, string(syntaxflow_scan.StageCollect))
	require.Contains(t, stageKeys, string(syntaxflow_scan.StageAnalyze))
	require.NotContains(t, stageKeys, string(syntaxflow_scan.StageReview),
		"compile for SSA-only must not claim 语义检测 ran")
	require.NotContains(t, stageKeys, string(syntaxflow_scan.StageInspect))
}

func TestScanProject_WithModeStackedSourceAndStruct(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("def run(x):\n    return eval(x)\n"), 0o644))

	var stages []string
	_, err := syntaxflow_scan.ScanProject(context.Background(),
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
	_, err = syntaxflow_scan.ScanProject(context.Background(),
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
	_, err := syntaxflow_scan.ScanProject(context.Background(),
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

func TestScanProject_MemoryCompileKeepsSSAAlerts(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("def run(x):\n    return eval(x)\n"), 0o644))

	var alerts int
	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithCompileMemoryCompile(true),
		syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: python, title: "memory ssa")
eval(* as $arg) as $call
alert $call`,
			Language: "python",
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += r.Result.RiskCount()
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.True(t, result.Succeeded)
	require.NotEmpty(t, result.ProgramName, "ScanProject must stamp a program name so SSA risks can be created")
	require.Greater(t, alerts, 0, "memory compile must keep SSA alerts without saving results to DB")
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
	result, err := syntaxflow_scan.ScanProject(context.Background(),
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
	require.Contains(t, stages, string(syntaxflow_scan.StageAnalyze))
	// Inspect still runs after review, but live callbacks stay in product
	// order so the cursor does not jump backward.
	var sawInspect, sawReview, sawAnalyze bool
	for _, outcome := range result.Stages {
		switch outcome.Stage {
		case syntaxflow_scan.StageInspect:
			sawInspect = true
		case syntaxflow_scan.StageReview:
			sawReview = true
		case syntaxflow_scan.StageAnalyze:
			sawAnalyze = true
		}
	}
	require.True(t, sawInspect, "inspect must still run after a loaded-program review")
	require.True(t, sawReview)
	require.True(t, sawAnalyze)
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
	_, err = syntaxflow_scan.ScanProject(context.Background(),
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

// A remote clone failure is collect failing: the project tree never arrived,
// so the run must not claim 语义检测 ran.
func TestScanProject_RemoteCloneFailureReportsCollectFailed(t *testing.T) {
	orig := syntaxflow_scan.CompileProject
	t.Cleanup(func() { syntaxflow_scan.CompileProject = orig })
	syntaxflow_scan.CompileProject = func(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
		return nil, fmt.Errorf("SSA Git clone failed: workspace=%q: git clone: connection refused", t.TempDir())
	}

	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceGit),
		ssaconfig.WithCodeSourceURL("https://127.0.0.1:1/no-such-repo.git"),
		ssaconfig.WithProjectRawLanguage("php"),
		ssaconfig.WithSetProgramName(t.Name()),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode),
		syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
	)
	require.Error(t, err)
	require.False(t, result.Succeeded)
	require.Len(t, result.Stages, 1)
	require.Equal(t, syntaxflow_scan.StageCollect, result.Stages[0].Stage)
	require.False(t, result.Stages[0].Succeeded())
	require.Contains(t, result.Stages[0].Error, "SSA Git clone failed")
}

// A local tree that fails to compile already finished collect. The compile
// error belongs to the detection stage that asked for compile.
func TestScanProject_LocalCompileFailureKeepsCollect(t *testing.T) {
	orig := syntaxflow_scan.CompileProject
	t.Cleanup(func() { syntaxflow_scan.CompileProject = orig })
	syntaxflow_scan.CompileProject = func(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
		return nil, fmt.Errorf("php parse failed")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.php"), []byte("<?php echo 1;\n"), 0o644))
	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("php"),
		ssaconfig.WithSetProgramName(t.Name()),
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode),
	)
	require.Error(t, err)
	require.False(t, result.Succeeded)
	require.Len(t, result.Stages, 2)
	require.Equal(t, syntaxflow_scan.StageCollect, result.Stages[0].Stage)
	require.True(t, result.Stages[0].Succeeded())
	require.Equal(t, syntaxflow_scan.StageReview, result.Stages[1].Stage)
	require.False(t, result.Stages[1].Succeeded())
	require.Contains(t, result.Stages[1].Error, "php parse failed")
}

// Nested StartScan must keep the dispatch snapshot. Without that copy the
// product pipeline queries the empty node sfdb and reports Total Rules = 0.
func TestScanProject_ForwardsTaskLocalRulesToStartScan(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "leak.env"), []byte("AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n"), 0o644))

	const ruleName = "task-local-source.sf"
	payload, err := json.Marshal(ssaconfig.TaskLocalRuleInputFile{
		Version: ssaconfig.TaskLocalRuleInputFileVersionV1,
		Rules: []*ypb.SyntaxFlowRuleInput{{
			RuleName: ruleName,
			Content: `desc(mode: "source", language: general, title: "task-local source")
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			Language: string(ssaconfig.General),
		}},
		Metadata: map[string]ssaconfig.TaskLocalRuleMetadata{
			ruleName: {AssetID: "asset-source"},
		},
	})
	require.NoError(t, err)
	inputPath := filepath.Join(t.TempDir(), "task-local-rules.json")
	require.NoError(t, os.WriteFile(inputPath, payload, 0o600))
	sum := sha256.Sum256(payload)
	configJSON, err := json.Marshal(map[string]any{
		"Mode": int(ssaconfig.ModeAll),
		"SyntaxFlowRule": map[string]any{
			"task_local":              true,
			"task_local_input_file":   inputPath,
			"task_local_input_sha256": hex.EncodeToString(sum[:]),
			"task_local_input_count":  1,
		},
	})
	require.NoError(t, err)

	var alerts int
	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithJsonRawConfig(configJSON),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("yak"),
		ssaconfig.WithSetProgramName(t.Name()),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.True(t, result.Succeeded)
	require.Greater(t, alerts, 0)
}
