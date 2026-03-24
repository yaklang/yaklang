package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifySatisfactionNextMovementsArrayExtraction(t *testing.T) {
	raw := `{
		"@action": "verify-satisfaction",
		"user_satisfied": false,
		"reasoning": "任务未完成",
		"completed_task_index": "",
		"next_movements": [
			{"op": "add", "id": "create_file", "content": "创建一个 A.md 文件"},
			{"op": "done", "id": "remove_temp_name"}
		]
	}`
	action, err := ExtractActionFromStream(context.Background(), strings.NewReader(raw), "verify-satisfaction")
	require.NoError(t, err)

	movements := action.GetInvokeParamsArray("next_movements")
	require.Len(t, movements, 2)
	require.Equal(t, "add", movements[0].GetString("op"))
	require.Equal(t, "create_file", movements[0].GetString("id"))
	require.Equal(t, "创建一个 A.md 文件", movements[0].GetString("content"))
	require.Equal(t, "done", movements[1].GetString("op"))
	require.Equal(t, "remove_temp_name", movements[1].GetString("id"))
	require.Equal(t, "", movements[1].GetString("content"))
}

func TestFormatVerifyNextMovementsSummary(t *testing.T) {
	summary := FormatVerifyNextMovementsSummary([]VerifyNextMovement{
		{Op: "add", ID: "create_file", Content: "创建一个 A.md 文件"},
		{Op: "done", ID: "remove_temp_name"},
	})
	require.Equal(t, "ADD[create_file]: 创建一个 A.md 文件; DONE[remove_temp_name]", summary)
}

func TestFormatVerifyNextMovementsSummary_Empty(t *testing.T) {
	require.Equal(t, "", FormatVerifyNextMovementsSummary(nil))
}
