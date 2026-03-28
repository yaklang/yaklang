package aireact

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestNormalizeVerifyNextMovements_LegacyStringFallback(t *testing.T) {
	action := aicommon.NewSimpleAction("verify-satisfaction", aitool.InvokeParams{
		"next_movements": "继续读取目标文件并确认内容是否正确",
	})

	movements := normalizeVerifyNextMovements(action)
	require.Len(t, movements, 1)
	require.Equal(t, "add", movements[0].Op)
	require.Equal(t, "legacy_next_movements", movements[0].ID)
	require.Equal(t, "继续读取目标文件并确认内容是否正确", movements[0].Content)
}

func TestNormalizeVerifyNextMovements_PrefersStructuredArray(t *testing.T) {
	action, err := aicommon.ExtractActionFromStream(context.Background(), strings.NewReader(`{
		"@action": "verify-satisfaction",
		"next_movements": [
			{"op": "add", "id": "create_file", "content": "创建 A.md"},
			{"op": "done", "id": "cleanup_temp"},
			{"op": "delete", "id": "stale_todo"}
		]
	}`), "verify-satisfaction")
	require.NoError(t, err)

	movements := normalizeVerifyNextMovements(action)
	require.Len(t, movements, 3)
	require.Equal(t, "create_file", movements[0].ID)
	require.Equal(t, "cleanup_temp", movements[1].ID)
	require.Equal(t, "delete", movements[2].Op)
	require.Equal(t, "stale_todo", movements[2].ID)
}

func TestNormalizeVerifyNextMovements_NormalizesPendingToDoing(t *testing.T) {
	action, err := aicommon.ExtractActionFromStream(context.Background(), strings.NewReader(`{
		"@action": "verify-satisfaction",
		"next_movements": [
			{"op": "pending", "id": "create_file"}
		]
	}`), "verify-satisfaction")
	require.NoError(t, err)

	movements := normalizeVerifyNextMovements(action)
	require.Len(t, movements, 1)
	require.Equal(t, "doing", movements[0].Op)
	require.Equal(t, "create_file", movements[0].ID)
}

func TestFormatNextMovementDisplayLine(t *testing.T) {
	require.Equal(t,
		"- [+]: [id: create_file]: 创建一个 A.md 文件",
		formatNextMovementDisplayLine(aicommon.VerifyNextMovement{Op: "add", ID: "create_file", Content: "创建一个 A.md 文件"}),
	)
	require.Equal(t,
		"- [x]: [id: remove_temp_name]",
		formatNextMovementDisplayLine(aicommon.VerifyNextMovement{Op: "done", ID: "remove_temp_name"}),
	)
	require.Equal(t,
		"- [DELETED]: [id: stale_todo]",
		formatNextMovementDisplayLine(aicommon.VerifyNextMovement{Op: "delete", ID: "stale_todo"}),
	)
	require.Equal(t,
		"- [DOING]: [id: create_file]",
		formatNextMovementDisplayLine(aicommon.VerifyNextMovement{Op: "doing", ID: "create_file"}),
	)
}

func TestWriteNextMovementsDisplayStream(t *testing.T) {
	var out bytes.Buffer
	err := writeNextMovementsDisplayStream(strings.NewReader(`[
		{"op":"add","id":"create_file","content":"创建一个 A.md 文件"},
		{"op":"done","id":"remove_temp_name"},
		{"op":"delete","id":"stale_todo"}
	]`), &out)
	require.NoError(t, err)
	require.Equal(t, "- [+]: [id: create_file]: 创建一个 A.md 文件\n- [x]: [id: remove_temp_name]\n- [DELETED]: [id: stale_todo]", out.String())
}

func TestWriteNextMovementsDisplayStream_InvalidJSON(t *testing.T) {
	var out bytes.Buffer
	err := writeNextMovementsDisplayStream(strings.NewReader(`继续读取目标文件并确认内容是否正确`), &out)
	require.Error(t, err)
}
