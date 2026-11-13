//go:build !irify_exclude

package yakcmds_test

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/cmd/yakcmds"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// writeCodeScanZip builds the same kind of archive CI passes via `-t ./fs.zip`.
func writeCodeScanZip(t *testing.T, files map[string]string) string {
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

// code-scan -t <zip> must accept an archive target instead of failing with
// "root path is not a directory: .". This is the exact CI invocation.
func TestCodeScanAcceptsZipTarget(t *testing.T) {
	zipPath := writeCodeScanZip(t, map[string]string{
		"main.go": "package main\n\nvar key = \"AKIAIOSFODNN7EXAMPLE\"\n",
	})
	outputPath := filepath.Join(t.TempDir(), "risk")

	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)
	err := app.Run([]string{
		"yak", "code-scan",
		"-t", zipPath,
		"-l", "golang",
		"--format", "irify-full",
		"-o", outputPath,
		"--memory",
		"--rule-keyword", "golang",
	})
	require.NoError(t, err)
	require.FileExists(t, outputPath+".json")
}

// code-scan without --language must still detect the project language. An
// empty language reaches the compiler as "": no builder is chosen, extra file
// analysis is disabled, and every language-typed rule is skipped as a
// language mismatch.
func TestCodeScanDetectsLanguageWhenOmitted(t *testing.T) {
	// A mixed tree is where the language decision is visible: the .ts file
	// sorts first, so the old per-file "try every builder" fallback picked
	// TypeScript for a Java project. Detection scores the whole tree instead.
	dir := t.TempDir()
	writeFile := func(name, content string) {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	writeFile("app/main.ts", "const x: number = 1;\nexport {};\n")
	writeFile("pom.xml", "<project><modelVersion>4.0.0</modelVersion></project>\n")
	for _, name := range []string{"A", "B", "C"} {
		writeFile("zsrc/"+name+".java", "public class "+name+" {}\n")
	}
	writeFile("zsrc/Exec.java", `public class Exec {
    public void run(String cmd) throws Exception {
        Runtime.getRuntime().exec(cmd);
    }
}
`)
	outputPath := filepath.Join(t.TempDir(), "report")
	programName := uuid.NewString()

	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)
	err := app.Run([]string{
		"yak", "code-scan",
		"-t", dir,
		"-p", programName,
		"--format", "irify",
		"-o", outputPath,
		"--mode", "ssa",
		"--rule-keyword", "java",
		"--syntaxflow", `desc(mode: "ssa", language: java, title: "language detection e2e")
Runtime.getRuntime().exec(* as $cmd) as $call;
alert $call`,
	})
	require.NoError(t, err)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	raw, err := os.ReadFile(outputPath + ".json")
	require.NoError(t, err)
	var report struct {
		RiskNums int `json:"RiskNums"`
	}
	require.NoError(t, json.Unmarshal(raw, &report))
	ir, err := ssadb.GetProgram(programName, ssa.Application)
	require.NoError(t, err)
	require.NotNil(t, ir)
	require.Equal(t, ssaconfig.Language("java"), ir.Language,
		"code-scan without --language must record the language the whole tree implies")
	require.NotEmpty(t, ir.FileList,
		"the detected language must compile the project files")
	require.Greater(t, report.RiskNums, 0,
		"a language-typed rule must run instead of being skipped as a language mismatch")
}

func addCommands(app *cli.App, cmds ...*cli.Command) {
	for _, i := range cmds {
		app.Commands = append(app.Commands, *i)
	}
}

func TestRecompileWarn(t *testing.T) {
	tmpDir := t.TempDir()
	log.Infof("tmpDir: %s", tmpDir)

	// create a test yak file
	file, err := os.Create(tmpDir + "/test.yak")
	require.NoError(t, err)
	file.WriteString(`
	a = 1
	`)
	file.Close()

	programName := uuid.NewString()

	// compile the yak file
	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)
	// compile with program name
	err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
	require.NoError(t, err)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	prog, err := ssadb.GetProgram(programName, ssa.Application)
	require.NoError(t, err)
	require.NotNil(t, prog, "compiled program should use explicit -p name")

	// recompile
	err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
	require.Error(t, err)
	require.Contains(t, err.Error(), "please use `re-compile` flag to re-compile or change program name")
}

func TestCompileDefaultProgramNameNoTimestamp(t *testing.T) {
	root := t.TempDir()
	programName := uuid.NewString()
	targetDir := filepath.Join(root, programName)
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "test.yak"), []byte("a = 1\n"), 0o644))

	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)

	err := app.Run([]string{"yak", "ssa-compile", "-t", targetDir})
	require.NoError(t, err)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	prog, err := ssadb.GetProgram(programName, ssa.Application)
	require.NoError(t, err)
	require.NotNil(t, prog, "compiled program should default to target directory name when -p is not set")
}

func TestSyntaxFlowEvaluate(t *testing.T) {
	tmpDir := t.TempDir()
	log.Infof("tmpDir: %s", tmpDir)

	// 创建一个好的测试规则文件
	goodRuleFile := filepath.Join(tmpDir, "good_rule.sf")
	goodRuleContent := `desc(
	title: 'Test Rule',
	type: vuln,
	description: 'This is a test rule with proper description',
	solution: 'Fix the vulnerability by proper input validation'
)

// This is a test rule
someVar as $sink
alert $sink`

	err := os.WriteFile(goodRuleFile, []byte(goodRuleContent), 0644)
	require.NoError(t, err)

	// 创建一个有语法错误的规则文件
	badRuleFile := filepath.Join(tmpDir, "bad_rule.sf")
	badRuleContent := `desc(
	title: 'Bad Rule'
)

// This rule has syntax error
invalid syntax here $$$`

	err = os.WriteFile(badRuleFile, []byte(badRuleContent), 0644)
	require.NoError(t, err)

	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)

	// 测试评估好的规则
	t.Run("evaluate good rule", func(t *testing.T) {
		err := app.Run([]string{"yak", "syntaxflow-evaluate", "-v", "-t", goodRuleFile})
		require.NoError(t, err)
	})

	// 测试评估坏的规则
	t.Run("evaluate bad rule", func(t *testing.T) {
		err := app.Run([]string{"yak", "syntaxflow-evaluate", "-v", "-t", badRuleFile})
		require.NoError(t, err) // 命令执行成功，但规则质量差
	})

	// 测试评估目录
	t.Run("evaluate directory", func(t *testing.T) {
		err := app.Run([]string{"yak", "syntaxflow-evaluate", "-t", tmpDir})
		require.NoError(t, err)
	})

	// 测试详细输出
	t.Run("evaluate with verbose", func(t *testing.T) {
		err := app.Run([]string{"yak", "syntaxflow-evaluate", "-t", badRuleFile, "-v"})
		require.NoError(t, err)
	})

	// 测试JSON输出
	t.Run("evaluate with json output", func(t *testing.T) {
		err := app.Run([]string{"yak", "syntaxflow-evaluate", "-t", goodRuleFile, "--json"})
		require.NoError(t, err)
	})

	// 测试输出到文件
	t.Run("evaluate with output file", func(t *testing.T) {
		outputFile := filepath.Join(tmpDir, "result.json")
		err := app.Run([]string{"yak", "syntaxflow-evaluate", "-t", goodRuleFile, "-o", outputFile})
		require.NoError(t, err)

		// 检查输出文件是否存在
		_, err = os.Stat(outputFile)
		require.NoError(t, err)
	})

	// 测试不存在的文件
	t.Run("evaluate non-existent file", func(t *testing.T) {
		err := app.Run([]string{"yak", "syntaxflow-evaluate", "-t", "/non/existent/file.sf"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	// 测试没有提供目标文件
	t.Run("evaluate without target", func(t *testing.T) {
		err := app.Run([]string{"yak", "syntaxflow-evaluate"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "target file or directory is required")
	})

	// 测试非.sf文件
	t.Run("evaluate non-sf file", func(t *testing.T) {
		nonSfFile := filepath.Join(tmpDir, "test.txt")
		err := os.WriteFile(nonSfFile, []byte("not a syntaxflow rule"), 0644)
		require.NoError(t, err)

		err = app.Run([]string{"yak", "syntaxflow-evaluate", "-t", nonSfFile})
		require.Error(t, err)
		require.Contains(t, err.Error(), "must be a .sf file")
	})
}

func TestSyntaxFlowEvaluateAliases(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建一个测试规则文件
	ruleFile := filepath.Join(tmpDir, "test_rule.sf")
	ruleContent := `desc(
	title: 'Test Rule',
	type: audit
)

someVar as $result
alert $result`

	err := os.WriteFile(ruleFile, []byte(ruleContent), 0644)
	require.NoError(t, err)

	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)

	// 测试各种别名
	aliases := []string{"sf-evaluate"}
	for _, alias := range aliases {
		t.Run("test alias "+alias, func(t *testing.T) {
			err := app.Run([]string{"yak", alias, "-t", ruleFile})
			require.NoError(t, err)
		})
	}
}

// The dry-parse contract for the memory mode: `ssa-compile --memory --debug
// <dir> --pprof` must compile the target and leave diagnostics only -- never a
// database, neither beside the diagnostics nor in the yak home.
func TestSSACompileMemoryModeWritesNoDatabase(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "Hello.java"),
		[]byte("public class Hello {\n  public static int add(int a, int b) { return a + b; }\n}\n"),
		0o644,
	))
	homeDir := t.TempDir()
	t.Setenv("YAKIT_HOME", homeDir)
	debugDir := filepath.Join(t.TempDir(), "debug")

	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)
	err := app.Run([]string{
		"yak", "ssa-compile",
		"-t", srcDir,
		"--memory",
		"--debug", debugDir,
		"--pprof",
	})
	require.NoError(t, err)

	require.NoFileExists(t, filepath.Join(debugDir, "ssadb.db"))
	require.FileExists(t, filepath.Join(debugDir, "log"))
	require.DirExists(t, filepath.Join(debugDir, "cpu-pprof"))
	require.DirExists(t, filepath.Join(debugDir, "memory-pprof"))

	// Memory mode must not create a database in the yak home either.
	entries, err := os.ReadDir(homeDir)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotContains(t, entry.Name(), ".db", "memory mode created %s", entry.Name())
	}
}

// --pprof without --debug has nowhere to write snapshots; fail loudly instead
// of silently dropping the profiling request.
func TestSSACompilePprofRequiresDebug(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.yak"), []byte("a = 1\n"), 0o644))

	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)
	err := app.Run([]string{"yak", "ssa-compile", "-t", srcDir, "--memory", "--pprof"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--pprof requires --debug")
}

// --ast-only parses source/prepared ASTs and stops before prehandler and SSA
// body build. This allows fast verification of jar/jsp decompiler output and
// parser correctness without paying for full SSA construction.
func TestSSACompileAstOnlySucceedsOnValidJava(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "Valid.java"),
		[]byte("package com.test;\npublic class Valid {\n  public int sum(int a, int b) { return a + b; }\n}\n"),
		0o644,
	))

	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)
	err := app.Run([]string{
		"yak", "ssa-compile",
		"-t", srcDir,
		"--memory",
		"--ast-only",
	})
	require.NoError(t, err)
}

// In ast-only mode, any syntax error rejected by the language parser must cause
// the command to fail and report the parser error.
func TestSSACompileAstOnlyFailsOnSyntaxError(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "Broken.java"),
		[]byte("public class Broken { this is not valid java code !!! }"),
		0o644,
	))

	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)
	err := app.Run([]string{
		"yak", "ssa-compile",
		"-t", srcDir,
		"--memory",
		"--ast-only",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse")
}
