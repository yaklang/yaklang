package aicommon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutputFilePin_FullFlow(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "deploy.sh")
	v1 := "#!/bin/bash\necho 'hello'\nexit 0\n"
	err := os.WriteFile(scriptPath, []byte(v1), 0755)
	require.NoError(t, err)

	cpm := NewContextProviderManager()

	verifyResult := &VerifySatisfactionResult{
		Satisfied:          false,
		Reasoning:          "script created, not executed yet",
		CompletedTaskIndex: "",
		OutputFiles:        []string{scriptPath},
	}

	for _, filePath := range verifyResult.OutputFiles {
		providerName := "output_file:" + filePath
		cpm.RegisterTracedContent(providerName, OutputFileContextProvider(filePath))
	}

	result1 := cpm.Execute(nil, nil)
	require.Contains(t, result1, "## Output File: "+scriptPath)
	require.Contains(t, result1, "echo 'hello'")
	require.Contains(t, result1, "1")
	require.Contains(t, result1, "exit 0")

	v2 := "#!/bin/bash\necho 'hello world'\nset -e\nexit 0\n"
	err = os.WriteFile(scriptPath, []byte(v2), 0755)
	require.NoError(t, err)

	result2 := cpm.Execute(nil, nil)
	require.Contains(t, result2, "echo 'hello world'")
	require.Contains(t, result2, "set -e")
	require.Contains(t, result2, "CHANGES_DIFF_")
}

func TestOutputFilePin_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "main.py")
	configPath := filepath.Join(dir, "config.yaml")

	err := os.WriteFile(scriptPath, []byte("import os\nprint(os.getcwd())\n"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(configPath, []byte("key: value\nport: 8080\n"), 0644)
	require.NoError(t, err)

	cpm := NewContextProviderManager()

	verifyResult := &VerifySatisfactionResult{
		Satisfied:   false,
		OutputFiles: []string{scriptPath, configPath},
	}

	for _, filePath := range verifyResult.OutputFiles {
		providerName := "output_file:" + filePath
		cpm.RegisterTracedContent(providerName, OutputFileContextProvider(filePath))
	}

	result := cpm.Execute(nil, nil)
	require.Contains(t, result, "main.py")
	require.Contains(t, result, "import os")
	require.Contains(t, result, "config.yaml")
	require.Contains(t, result, "key: value")
}

func TestOutputFilePin_FileDeletedBetweenRounds(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "temp.txt")

	err := os.WriteFile(filePath, []byte("temporary content\n"), 0644)
	require.NoError(t, err)

	cpm := NewContextProviderManager()
	cpm.RegisterTracedContent("output_file:"+filePath, OutputFileContextProvider(filePath))

	result1 := cpm.Execute(nil, nil)
	require.Contains(t, result1, "temporary content")

	os.Remove(filePath)

	result2 := cpm.Execute(nil, nil)
	require.Contains(t, result2, "Error")
}

func TestOutputFilePin_DuplicateRegistration(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "script.sh")
	err := os.WriteFile(filePath, []byte("echo test\n"), 0644)
	require.NoError(t, err)

	cpm := NewContextProviderManager()

	providerName := "output_file:" + filePath
	cpm.RegisterTracedContent(providerName, OutputFileContextProvider(filePath))
	cpm.RegisterTracedContent(providerName, OutputFileContextProvider(filePath))

	result := cpm.Execute(nil, nil)

	count := strings.Count(result, "## Output File: "+filePath)
	require.Equal(t, 1, count, "duplicate registration should be ignored")
}

func TestOutputFilePin_ModifyReExecuteCycle(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "task.py")

	v1 := "import sys\nprint('v1')\nsys.exit(1)\n"
	err := os.WriteFile(scriptPath, []byte(v1), 0644)
	require.NoError(t, err)

	cpm := NewContextProviderManager()
	cpm.RegisterTracedContent("output_file:"+scriptPath, OutputFileContextProvider(scriptPath))

	round1 := cpm.Execute(nil, nil)
	require.Contains(t, round1, "sys.exit(1)")
	require.Contains(t, round1, "1")
	require.Contains(t, round1, "print('v1')")

	v2 := "import sys\nprint('v2 fixed')\nsys.exit(0)\n"
	err = os.WriteFile(scriptPath, []byte(v2), 0644)
	require.NoError(t, err)

	round2 := cpm.Execute(nil, nil)
	require.Contains(t, round2, "print('v2 fixed')")
	require.Contains(t, round2, "sys.exit(0)")
	require.Contains(t, round2, "CHANGES_DIFF_")
	require.Contains(t, round2, "-2 | print('v1')")
	require.Contains(t, round2, "+2 | print('v2 fixed')")

	v3 := "import sys\nimport os\nprint('v3 final')\nsys.exit(0)\n"
	err = os.WriteFile(scriptPath, []byte(v3), 0644)
	require.NoError(t, err)

	round3 := cpm.Execute(nil, nil)
	require.Contains(t, round3, "import os")
	require.Contains(t, round3, "print('v3 final')")
	require.Contains(t, round3, "CHANGES_DIFF_")
}
