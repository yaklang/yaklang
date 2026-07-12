package aitool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutputFileInfo_IsSafeSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		expected bool
	}{
		{"zero", 0, true},
		{"small", 1024, true},
		{"exactly_limit", MaxOutputFileTokens, true},
		{"one_over", MaxOutputFileTokens + 1, false},
		{"large", 100 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &OutputFileInfo{Size: tt.size}
			require.Equal(t, tt.expected, info.IsSafeSize())
		})
	}
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