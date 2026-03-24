package aitool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutputFileInfo_LineNumberedContent(t *testing.T) {
	info := &OutputFileInfo{
		Path:    "/tmp/test.py",
		Size:    42,
		Content: "line one\nline two\nline three",
	}

	result := info.LineNumberedContent()
	require.Contains(t, result, "1")
	require.Contains(t, result, "line one")
	require.Contains(t, result, "2")
	require.Contains(t, result, "line two")
	require.Contains(t, result, "3")
	require.Contains(t, result, "line three")
}

func TestOutputFileInfo_LineNumberedContent_Empty(t *testing.T) {
	info := &OutputFileInfo{
		Path:    "/tmp/empty.txt",
		Size:    0,
		Content: "",
	}

	result := info.LineNumberedContent()
	require.Equal(t, "", result)
}

func TestOutputFileInfo_IsSafeSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		expected bool
	}{
		{"zero", 0, true},
		{"small", 1024, true},
		{"exactly_limit", MaxOutputFileBytes, true},
		{"one_over", MaxOutputFileBytes + 1, false},
		{"large", 100 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &OutputFileInfo{Size: tt.size}
			require.Equal(t, tt.expected, info.IsSafeSize())
		})
	}
}

func TestReadOutputFileFromPath(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test_script.py")
	content := "#!/usr/bin/env python3\nprint('hello world')\n"
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	info, err := ReadOutputFileFromPath(filePath)
	require.NoError(t, err)
	require.Equal(t, filePath, info.Path)
	require.Equal(t, int64(len(content)), info.Size)
	require.Equal(t, content, info.Content)
}

func TestReadOutputFileFromPath_NotExist(t *testing.T) {
	_, err := ReadOutputFileFromPath("/tmp/nonexistent_file_abc123.txt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "stat output file")
}

func TestReadOutputFileFromPath_Directory(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadOutputFileFromPath(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is a directory")
}

func TestReadOutputFileFromPath_Truncation(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "large_file.txt")

	largeContent := strings.Repeat("x", int(MaxOutputFileBytes)+1024)
	err := os.WriteFile(filePath, []byte(largeContent), 0644)
	require.NoError(t, err)

	info, err := ReadOutputFileFromPath(filePath)
	require.NoError(t, err)
	require.Equal(t, int64(len(largeContent)), info.Size)
	require.Equal(t, int(MaxOutputFileBytes), len(info.Content))
	require.False(t, info.IsSafeSize())
}

func TestReadOutputFileFromPath_ExactlyMaxSize(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "exact_file.txt")

	exactContent := strings.Repeat("y", int(MaxOutputFileBytes))
	err := os.WriteFile(filePath, []byte(exactContent), 0644)
	require.NoError(t, err)

	info, err := ReadOutputFileFromPath(filePath)
	require.NoError(t, err)
	require.Equal(t, int64(MaxOutputFileBytes), info.Size)
	require.Equal(t, exactContent, info.Content)
	require.True(t, info.IsSafeSize())
}

func TestReadOutputFileFromPath_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "empty.txt")
	err := os.WriteFile(filePath, []byte{}, 0644)
	require.NoError(t, err)

	info, err := ReadOutputFileFromPath(filePath)
	require.NoError(t, err)
	require.Equal(t, int64(0), info.Size)
	require.Equal(t, "", info.Content)
	require.True(t, info.IsSafeSize())
}

func TestToolResult_OutputFiles_String(t *testing.T) {
	result := &ToolResult{
		Name:    "bash",
		Success: true,
		Data:    &ToolExecutionResult{Stdout: "ok"},
		OutputFiles: []*OutputFileInfo{
			{Path: "/tmp/script.py", Size: 1024},
			{Path: "/tmp/output.txt", Size: 512},
		},
	}

	str := result.String()
	require.Contains(t, str, "output_files:")
	require.Contains(t, str, "/tmp/script.py (1024 bytes)")
	require.Contains(t, str, "/tmp/output.txt (512 bytes)")
}

func TestToolResult_OutputFiles_Empty(t *testing.T) {
	result := &ToolResult{
		Name:    "bash",
		Success: true,
		Data:    &ToolExecutionResult{Stdout: "ok"},
	}

	str := result.String()
	require.NotContains(t, str, "output_files:")
}
