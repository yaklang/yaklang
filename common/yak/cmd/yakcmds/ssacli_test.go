package yakcmds_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/cmd/yakcmds"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

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

	// recompile
	err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
	require.Error(t, err)
	require.Contains(t, err.Error(), "please use `re-compile` flag to re-compile or change program name")
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
