package syntaxflow_scan_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// mixedLanguageFS holds one file per language. Nothing partitions these files:
// each per-language program must select its own source files through its
// builder and report only findings from them.
func mixedLanguageFS() *filesys.VirtualFS {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.go", "package main\n\nfunc report(v any) {}\n\nfunc main() { report(\"go\") }\n")
	vf.AddFile("App.java", "class App {\n    static void report(Object value) {}\n    void run() { report(\"java\"); }\n}\n")
	vf.AddFile("app.py", "def report(value):\n    pass\n\nreport(\"python\")\n")
	vf.AddFile("app.js", "function report(value) {}\nreport(\"js\");\n")
	vf.AddFile("app.ts", "function report(value: string) {}\nreport(\"ts\");\n")
	vf.AddFile("app.php", "<?php\nfunction report($value) {}\nreport(\"php\");\n")
	return vf
}

// materializeMixedFS writes the virtual tree to a temporary directory because
// a project scan compiles from a source path, not from a filesystem handle.
func materializeMixedFS(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_, err := filesys.CopyToRefLocal(mixedLanguageFS(), dir)
	require.NoError(t, err)
	return dir
}

func fileListHas(files map[string]string, name string) bool {
	for path := range files {
		if path == name || strings.HasSuffix(path, "/"+name) {
			return true
		}
	}
	return false
}

func languageProgramFiles(t *testing.T, program string) map[string]string {
	t.Helper()
	prog, err := ssaapi.FromDatabase(program)
	require.NoError(t, err)
	require.NotNil(t, prog)
	return prog.Program.FileList
}

// A file of one language must never surface in another language's program.
func TestScanProjectAllLanguagesCompilesEachLanguage(t *testing.T) {
	name := "all-" + uuid.NewString()
	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(materializeMixedFS(t)),
		ssaconfig.WithSetProgramName(name),
		syntaxflow_scan.WithAllLanguages(true),
		syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
	)
	require.NoError(t, err)
	require.True(t, result.Succeeded)

	owns := map[ssaconfig.Language]string{
		ssaconfig.GO:     "main.go",
		ssaconfig.JAVA:   "App.java",
		ssaconfig.PYTHON: "app.py",
		ssaconfig.PHP:    "app.php",
		ssaconfig.TS:     "app.ts",
	}
	require.Len(t, result.LanguagePrograms, len(owns))
	require.NotContains(t, result.LanguagePrograms, ssaconfig.Language("js"),
		"a legacy js spelling must not create a program of its own")
	require.NotContains(t, result.LanguagePrograms, ssaconfig.Language("ts"),
		"a legacy ts spelling must not create a program of its own")
	for language, file := range owns {
		program := result.LanguagePrograms[language]
		require.Equal(t, name+"-"+string(language), program)
		prog, err := ssaapi.FromDatabase(program)
		require.NoError(t, err)
		require.Equal(t, language, prog.GetLanguage())
		require.True(t, fileListHas(prog.Program.FileList, file), "%s must compile %s", language, file)
	}
	tsFiles := languageProgramFiles(t, result.LanguagePrograms[ssaconfig.TS])
	require.True(t, fileListHas(tsFiles, "app.js"), "the TypeScript program must compile JavaScript too")

	goFiles := languageProgramFiles(t, result.LanguagePrograms[ssaconfig.GO])
	require.False(t, fileListHas(goFiles, "App.java"), "a Go program must not compile Java")
	require.False(t, fileListHas(goFiles, "app.py"), "a Go program must not compile Python")
	pythonFiles := languageProgramFiles(t, result.LanguagePrograms[ssaconfig.PYTHON])
	require.False(t, fileListHas(pythonFiles, "main.go"), "a Python program must not compile Go")
}

// The scan result must name each finding's language and file, so a mixed tree
// shows "golang", "java", and "python" findings side by side.
func TestScanProjectAllLanguagesReportsEachLanguage(t *testing.T) {
	rt := ssaapi.NewScanRuntime()
	var mu sync.Mutex
	findings := map[string][]schema.SSARisk{}
	rt.RegisterRiskUpdateHandler(schema.RiskUpdateHandlerFunc(func(item schema.RiskUpdateItem) error {
		if item.Risk == nil {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		risk := *item.Risk
		findings[risk.Title] = append(findings[risk.Title], risk)
		return nil
	}))

	body := `* as $value
$value?{opcode: call} as $hit
alert $hit`
	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(materializeMixedFS(t)),
		ssaconfig.WithSetProgramName("report-"+uuid.NewString()),
		syntaxflow_scan.WithAllLanguages(true),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode, syntaxflow_scan.SSAMode),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{Content: fmt.Sprintf("desc(mode: \"ssa\", language: golang, title: \"go const\")\n%s", body)}),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{Content: fmt.Sprintf("desc(mode: \"ssa\", language: java, title: \"java const\")\n%s", body)}),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{Content: fmt.Sprintf("desc(mode: \"ssa\", language: python, title: \"python const\")\n%s", body)}),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{Content: fmt.Sprintf("desc(mode: \"ssa\", language: js, title: \"js const\")\n%s", body)}),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{Content: `desc(mode: "source", language: general, title: "source report")
${*}.pattern_regex(/report\(/) as $hit
alert $hit`}),
		syntaxflow_scan.WithScanRuntime(rt),
	)
	require.NoError(t, err)
	require.True(t, result.Succeeded)

	mu.Lock()
	defer mu.Unlock()
	expect := map[string]struct{ language, file string }{
		"go const":     {string(ssaconfig.GO), "main.go"},
		"java const":   {string(ssaconfig.JAVA), "App.java"},
		"python const": {string(ssaconfig.PYTHON), "app.py"},
	}
	for title, want := range expect {
		risks := findings[title]
		require.NotEmpty(t, risks, "rule %q produced no finding", title)
		positioned := 0
		for _, risk := range risks {
			require.Equal(t, want.language, risk.Language)
			if risk.CodeSourceUrl == "" {
				continue
			}
			positioned++
			require.True(t, strings.HasSuffix(risk.CodeSourceUrl, "/"+want.file),
				"%s ran on %s", title, risk.CodeSourceUrl)
			require.True(t, strings.HasSuffix(risk.ProgramName, "-"+want.language))
		}
		require.Positive(t, positioned, "rule %q produced no positioned finding", title)
	}

	// A js rule runs on the TypeScript program: one ECMAScript frontend and one
	// program, so the finding is attributed to the program's language.
	jsRisks := findings["js const"]
	require.NotEmpty(t, jsRisks, "a js rule must still run after the merge")
	jsOnJSFile := 0
	for _, risk := range jsRisks {
		require.Equal(t, string(ssaconfig.TS), risk.Language)
		require.True(t, strings.HasSuffix(risk.ProgramName, "-"+string(ssaconfig.TS)))
		if strings.HasSuffix(risk.CodeSourceUrl, "/app.js") {
			jsOnJSFile++
		}
	}
	require.Positive(t, jsOnJSFile, "a js rule must still report JavaScript files")

	sourceFiles := map[string]struct{}{}
	for _, risk := range findings["source report"] {
		sourceFiles[risk.CodeSourceUrl] = struct{}{}
	}
	require.GreaterOrEqual(t, len(sourceFiles), 3, "one source pass must cover every language's file")
	var names []string
	for file := range sourceFiles {
		names = append(names, file)
	}
	sort.Strings(names)
	joined := strings.Join(names, "\n")
	for _, file := range []string{"main.go", "App.java", "app.py"} {
		require.Contains(t, joined, file)
	}
}

// A broken file must not drop its language's good files. Strict compilation is
// forced off for the per-language programs even when the caller asked for it.
func TestScanProjectAllLanguagesKeepsGoodFilesAfterParseError(t *testing.T) {
	vf := mixedLanguageFS()
	vf.AddFile("broken.go", "package main\n\nfunc broken( {\n")
	dir := t.TempDir()
	_, err := filesys.CopyToRefLocal(vf, dir)
	require.NoError(t, err)

	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithSetProgramName("broken-"+uuid.NewString()),
		ssaconfig.WithCompileStrictMode(true),
		syntaxflow_scan.WithAllLanguages(true),
		syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
		ssaconfig.WithRuleInputRaw(`desc(mode: "ssa", language: golang, title: "broken sibling")
* as $value
$value?{opcode: call} as $hit
alert $hit`),
	)
	require.NoError(t, err)
	require.True(t, result.Succeeded)
	javaProgram := result.LanguagePrograms[ssaconfig.JAVA]
	require.NotEmpty(t, javaProgram)
	require.True(t, fileListHas(languageProgramFiles(t, javaProgram), "App.java"),
		"the good Java file must survive its broken sibling")
	goProgram := result.LanguagePrograms[ssaconfig.GO]
	require.NotEmpty(t, goProgram)
	require.True(t, fileListHas(languageProgramFiles(t, goProgram), "main.go"),
		"the good Go file must survive its broken sibling")
}

func TestScanProjectAllLanguagesContinuesAfterLanguageFailure(t *testing.T) {
	original := syntaxflow_scan.CompileProject
	require.NotNil(t, original)
	t.Cleanup(func() { syntaxflow_scan.CompileProject = original })
	var compiled []ssaconfig.Language
	syntaxflow_scan.CompileProject = func(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
		compiled = append(compiled, cfg.GetLanguage())
		if cfg.GetLanguage() == ssaconfig.GO {
			return nil, fmt.Errorf("injected compiler failure")
		}
		return original(ctx, cfg, extra...)
	}

	rt := ssaapi.NewScanRuntime()
	var mu sync.Mutex
	var pythonRisks int
	rt.RegisterRiskUpdateHandler(schema.RiskUpdateHandlerFunc(func(item schema.RiskUpdateItem) error {
		if item.Risk == nil || item.Risk.Language != string(ssaconfig.PYTHON) {
			return nil
		}
		mu.Lock()
		pythonRisks++
		mu.Unlock()
		return nil
	}))

	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(materializeMixedFS(t)),
		ssaconfig.WithSetProgramName("failure-"+uuid.NewString()),
		syntaxflow_scan.WithAllLanguages(true),
		syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
		ssaconfig.WithRuleInputRaw(`desc(mode: "ssa", language: python, title: "python survives")
* as $value
$value?{opcode: call} as $hit
alert $hit`),
		syntaxflow_scan.WithScanRuntime(rt),
	)
	require.NoError(t, err, "a failed language must not suppress the others")
	require.Contains(t, compiled, ssaconfig.GO)
	require.NotContains(t, result.LanguagePrograms, ssaconfig.GO)
	require.Contains(t, result.LanguagePrograms, ssaconfig.PYTHON)
	require.Contains(t, result.Error, "injected compiler failure")
	require.True(t, result.Succeeded)
	mu.Lock()
	require.Positive(t, pythonRisks, "the later Python program must still be scanned")
	mu.Unlock()
}

func TestScanProjectExplicitLanguageWinsOverDetection(t *testing.T) {
	result, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(materializeMixedFS(t)),
		ssaconfig.WithSetProgramName("explicit-"+uuid.NewString()),
		ssaconfig.WithProjectLanguage(ssaconfig.JAVA),
		syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
	)
	require.NoError(t, err)
	require.True(t, result.Succeeded)
	require.Empty(t, result.LanguagePrograms, "explicit language mode compiles one program, not a fan-out")
	files := languageProgramFiles(t, result.ProgramName)
	require.True(t, fileListHas(files, "App.java"))
	require.False(t, fileListHas(files, "main.go"), "explicit java must not compile Go")
	require.False(t, fileListHas(files, "app.py"), "explicit java must not compile Python")
}

func TestAllLanguagesRejectsExplicitLanguageAndExistingProgram(t *testing.T) {
	_, err := syntaxflow_scan.ScanProject(context.Background(), syntaxflow_scan.WithAllLanguages(true), ssaconfig.WithProjectLanguage(ssaconfig.JAVA))
	require.ErrorContains(t, err, "mutually exclusive")
	_, err = syntaxflow_scan.ScanProject(context.Background(), syntaxflow_scan.WithAllLanguages(true), ssaconfig.WithSetProgramName("existing"))
	require.ErrorContains(t, err, "source target")
}
