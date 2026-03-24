package aicommon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestOutputFileContextProvider_Basic(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.py")
	content := "print('hello')\nprint('world')\n"
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	provider := OutputFileContextProvider(filePath)
	result, err := provider(nil, nil, "test_key")
	require.NoError(t, err)

	require.Contains(t, result, "## Output File: "+filePath)
	require.Contains(t, result, "```")
	// line-numbered content
	require.Contains(t, result, "1")
	require.Contains(t, result, "print('hello')")
	require.Contains(t, result, "2")
	require.Contains(t, result, "print('world')")
}

func TestOutputFileContextProvider_NotExist(t *testing.T) {
	provider := OutputFileContextProvider("/tmp/nonexistent_abc123.txt")
	result, err := provider(nil, nil, "test_key")
	require.Error(t, err)
	require.Contains(t, result, "[Error: failed to read output file")
}

func TestOutputFileContextProvider_Truncated(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "large.py")
	largeContent := strings.Repeat("x = 1\n", int(aitool.MaxOutputFileBytes)/6+100)
	err := os.WriteFile(filePath, []byte(largeContent), 0644)
	require.NoError(t, err)

	provider := OutputFileContextProvider(filePath)
	result, err := provider(nil, nil, "test_key")
	require.NoError(t, err)

	require.Contains(t, result, "Note: file truncated")
	require.Contains(t, result, "## Output File: "+filePath)
}

func TestOutputFileContextProvider_WithRegisterTracedContent(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "script.py")

	err := os.WriteFile(filePath, []byte("version1\n"), 0644)
	require.NoError(t, err)

	cpm := NewContextProviderManager()
	cpm.RegisterTracedContent("output_file:"+filePath, OutputFileContextProvider(filePath))

	result1 := cpm.Execute(nil, nil)
	require.Contains(t, result1, "version1")

	err = os.WriteFile(filePath, []byte("version2_modified\n"), 0644)
	require.NoError(t, err)

	result2 := cpm.Execute(nil, nil)
	require.Contains(t, result2, "version2_modified")
	require.Contains(t, result2, "CHANGES_DIFF_")
}

func TestOutputFileContextProvider_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "empty.txt")
	err := os.WriteFile(filePath, []byte{}, 0644)
	require.NoError(t, err)

	provider := OutputFileContextProvider(filePath)
	result, err := provider(nil, nil, "test_key")
	require.NoError(t, err)
	require.Contains(t, result, "## Output File: "+filePath)
	require.Contains(t, result, "(0B)")
}
