package aicommon

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestSanitizeIdentifier tests the sanitizeIdentifier function
func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "simple lowercase",
			input:    "query_large_file",
			expected: "query_large_file",
		},
		{
			name:     "uppercase to lowercase",
			input:    "QUERY_LARGE_FILE",
			expected: "query_large_file",
		},
		{
			name:     "mixed case",
			input:    "Query_Large_File",
			expected: "query_large_file",
		},
		{
			name:     "spaces to underscores",
			input:    "query large file",
			expected: "query_large_file",
		},
		{
			name:     "hyphens to underscores",
			input:    "query-large-file",
			expected: "query_large_file",
		},
		{
			name:     "special characters removed",
			input:    "query@large#file!",
			expected: "querylargefile",
		},
		{
			name:     "numbers preserved",
			input:    "query_file_123",
			expected: "query_file_123",
		},
		{
			name:     "long string truncated to 30 chars",
			input:    "this_is_a_very_long_identifier_that_exceeds_thirty_characters",
			expected: "this_is_a_very_long_identifier",
		},
		{
			name:     "unicode characters removed",
			input:    "query_文件_file",
			expected: "query__file",
		},
		{
			name:     "mixed special characters",
			input:    "find/process (pid=123)",
			expected: "findprocess_pid123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeIdentifier(tt.input)
			require.Equal(t, tt.expected, result, "sanitizeIdentifier(%q) should return %q, got %q", tt.input, tt.expected, result)
		})
	}
}

// TestGenerateParamsResult tests the GenerateParamsResult structure
func TestGenerateParamsResult(t *testing.T) {
	t.Run("with identifier", func(t *testing.T) {
		result := &GenerateParamsResult{
			Params: aitool.InvokeParams{
				"param1": "value1",
				"param2": 123,
			},
			Identifier: "query_large_file",
		}

		require.NotNil(t, result.Params)
		require.Equal(t, "value1", result.Params.GetString("param1"))
		require.Equal(t, int64(123), result.Params.GetInt("param2"))
		require.Equal(t, "query_large_file", result.Identifier)
	})

	t.Run("without identifier", func(t *testing.T) {
		result := &GenerateParamsResult{
			Params: aitool.InvokeParams{
				"message": "hello",
			},
			Identifier: "",
		}

		require.NotNil(t, result.Params)
		require.Equal(t, "hello", result.Params.GetString("message"))
		require.Empty(t, result.Identifier)
	})
}

// TestToolParamsPromptMetaWithIdentifier tests the ToolParamsPromptMeta structure with Identifier field
func TestToolParamsPromptMetaWithIdentifier(t *testing.T) {
	meta := &ToolParamsPromptMeta{
		Prompt:     "test prompt",
		Nonce:      "abc123",
		ParamNames: []string{"param1", "param2"},
		Identifier: "find_process",
	}

	require.Equal(t, "test prompt", meta.Prompt)
	require.Equal(t, "abc123", meta.Nonce)
	require.Len(t, meta.ParamNames, 2)
	require.Equal(t, "find_process", meta.Identifier)
}

// TestExtractIdentifierFromAction tests extracting identifier from action response
func TestExtractIdentifierFromAction(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		expectedIdentifier string
	}{
		{
			name: "with identifier field",
			input: `{
				"@action": "call-tool",
				"tool": "grep",
				"identifier": "search_log_error",
				"params": {
					"pattern": "ERROR"
				}
			}`,
			expectedIdentifier: "search_log_error",
		},
		{
			name: "without identifier field",
			input: `{
				"@action": "call-tool",
				"tool": "grep",
				"params": {
					"pattern": "ERROR"
				}
			}`,
			expectedIdentifier: "",
		},
		{
			name: "identifier with special characters",
			input: `{
				"@action": "call-tool",
				"tool": "bash",
				"identifier": "Query-Large-File!",
				"params": {}
			}`,
			expectedIdentifier: "query_large_file",
		},
		{
			name: "identifier too long",
			input: `{
				"@action": "call-tool",
				"tool": "find",
				"identifier": "this_is_a_very_long_identifier_that_should_be_truncated",
				"params": {}
			}`,
			expectedIdentifier: "this_is_a_very_long_identifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			action, err := ExtractValidActionFromStream(ctx, strings.NewReader(tt.input), "call-tool")
			require.NoError(t, err, "failed to extract action")

			// Extract and sanitize identifier
			rawIdentifier := action.GetString("identifier")
			identifier := sanitizeIdentifier(rawIdentifier)

			require.Equal(t, tt.expectedIdentifier, identifier, "identifier mismatch")
		})
	}
}

// TestSanitizeFilename tests the sanitizeFilename function for directory names
func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "bash",
			expected: "bash",
		},
		{
			name:     "name with underscores",
			input:    "test_tool",
			expected: "test_tool",
		},
		{
			name:     "name with hyphens",
			input:    "test-tool",
			expected: "test-tool",
		},
		{
			name:     "name with special characters",
			input:    "test@tool#name",
			expected: "test_tool_name",
		},
		{
			name:     "empty name",
			input:    "",
			expected: "unknown",
		},
		{
			name:     "name with spaces",
			input:    "test tool",
			expected: "test_tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			require.Equal(t, tt.expected, result, "sanitizeFilename(%q) should return %q, got %q", tt.input, tt.expected, result)
		})
	}
}

// TestToolCallDirectoryNaming tests the directory naming logic for tool calls
func TestToolCallDirectoryNaming(t *testing.T) {
	tests := []struct {
		name                  string
		toolName              string
		toolCallNumber        int
		destinationIdentifier string
		expectedDirName       string
	}{
		{
			name:                  "with identifier",
			toolName:              "grep",
			toolCallNumber:        1,
			destinationIdentifier: "query_large_file",
			expectedDirName:       "1_grep_query_large_file",
		},
		{
			name:                  "without identifier",
			toolName:              "bash",
			toolCallNumber:        2,
			destinationIdentifier: "",
			expectedDirName:       "2_bash",
		},
		{
			name:                  "with special tool name",
			toolName:              "test@tool",
			toolCallNumber:        3,
			destinationIdentifier: "find_process",
			expectedDirName:       "3_test_tool_find_process",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolNameSanitized := sanitizeFilename(tt.toolName)

			var dirName string
			if tt.destinationIdentifier != "" {
				dirName = fmt.Sprintf("%d_%s_%s", tt.toolCallNumber, toolNameSanitized, tt.destinationIdentifier)
			} else {
				dirName = fmt.Sprintf("%d_%s", tt.toolCallNumber, toolNameSanitized)
			}

			require.Equal(t, tt.expectedDirName, dirName, "directory name mismatch")
		})
	}
}
