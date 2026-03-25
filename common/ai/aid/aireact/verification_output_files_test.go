package aireact

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestVerificationSchema_OutputFilesField(t *testing.T) {
	schemaPath := "prompts/verification/verification.json"
	data, err := os.ReadFile(schemaPath)
	require.NoError(t, err)

	var schema map[string]interface{}
	err = json.Unmarshal(data, &schema)
	require.NoError(t, err)

	props, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok, "schema should have properties")

	outputFiles, ok := props["output_files"].(map[string]interface{})
	require.True(t, ok, "schema should have output_files property")

	require.Equal(t, "array", outputFiles["type"])

	items, ok := outputFiles["items"].(map[string]interface{})
	require.True(t, ok, "output_files should have items")
	require.Equal(t, "string", items["type"])
}

func TestVerifySatisfactionResult_OutputFiles(t *testing.T) {
	jsonStr := `{
		"satisfied": false,
		"reasoning": "task not complete",
		"completed_task_index": "",
		"next_movements": [],
		"output_files": ["/tmp/test.py", "/tmp/output.txt"]
	}`

	var result aicommon.VerifySatisfactionResult
	err := json.Unmarshal([]byte(jsonStr), &result)
	require.NoError(t, err)
	require.False(t, result.Satisfied)
	require.Equal(t, []string{"/tmp/test.py", "/tmp/output.txt"}, result.OutputFiles)
}

func TestVerifySatisfactionResult_OutputFiles_Empty(t *testing.T) {
	jsonStr := `{
		"satisfied": true,
		"reasoning": "task complete",
		"completed_task_index": "1-1",
		"next_movements": [],
		"output_files": []
	}`

	var result aicommon.VerifySatisfactionResult
	err := json.Unmarshal([]byte(jsonStr), &result)
	require.NoError(t, err)
	require.True(t, result.Satisfied)
	require.Empty(t, result.OutputFiles)
}

func TestVerifySatisfactionResult_OutputFiles_Missing(t *testing.T) {
	jsonStr := `{
		"satisfied": true,
		"reasoning": "task complete",
		"completed_task_index": "1-1",
		"next_movements": []
	}`

	var result aicommon.VerifySatisfactionResult
	err := json.Unmarshal([]byte(jsonStr), &result)
	require.NoError(t, err)
	require.Nil(t, result.OutputFiles)
}

func TestRenderVerificationOutputFilesMarkdown(t *testing.T) {
	markdown := renderVerificationOutputFilesMarkdown([]string{
		" /tmp/result.md ",
		"/tmp/result.md",
		"/tmp/log.txt",
		"/tmp/ai_bash_script_123.sh",
	})
	require.Equal(t, "## 交付文件\n\n- /tmp/result.md\n- /tmp/log.txt", markdown)
}

func TestRenderVerificationOutputFilesMarkdown_Empty(t *testing.T) {
	require.Empty(t, renderVerificationOutputFilesMarkdown(nil))
	require.Empty(t, renderVerificationOutputFilesMarkdown([]string{"", "   ", "/tmp/ai_bash_script_1.sh"}))
}
