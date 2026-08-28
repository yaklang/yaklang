package aicommon

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEmitStatusKeepsLegacyPayload(t *testing.T) {
	var captured *schema.AiOutputEvent
	emitter := NewEmitter("status-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = event
		return event, nil
	})

	_, err := emitter.EmitStatus("legacy", "working")
	require.NoError(t, err)
	require.NotNil(t, captured)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(captured.Content, &payload))
	require.Equal(t, map[string]any{"key": "legacy", "value": "working"}, payload)
}

func TestEmitStatusI18nUsesChineseLegacyValueAndStructuredMetadata(t *testing.T) {
	var captured *schema.AiOutputEvent
	emitter := NewEmitter("status-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = event
		return event, nil
	})

	_, err := emitter.EmitStatusI18n(
		"re-act-loading-status-key",
		"正在并行调用 3 个工具",
		"Running 3 tools in parallel",
		WithStatusCode("tool.batch.running"),
		WithStatusDetail("文本搜索、文件读取、语法检查", "Text Search, File Reader, Syntax Checker"),
		WithStatusProgress(1, 3, "tool"),
		WithStatusTools(StatusTool{Name: "grep", DisplayName: "文本搜索", State: StatusStateRunning}),
	)
	require.NoError(t, err)
	require.NotNil(t, captured)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(captured.Content, &payload))
	require.Equal(t, "re-act-loading-status-key", payload["key"])
	require.Equal(t, "正在并行调用 3 个工具", payload["value"])
	require.Equal(t, "tool.batch.running", payload["code"])
	require.Equal(t, "running", payload["state"])

	valueI18n := payload["value_i18n"].(map[string]any)
	require.Equal(t, "正在并行调用 3 个工具", valueI18n["zh"])
	require.Equal(t, "Running 3 tools in parallel", valueI18n["en"])

	detailI18n := payload["detail_i18n"].(map[string]any)
	require.Equal(t, "文本搜索、文件读取、语法检查", detailI18n["zh"])
	require.Equal(t, "Text Search, File Reader, Syntax Checker", detailI18n["en"])

	progress := payload["progress"].(map[string]any)
	require.Equal(t, float64(1), progress["current"])
	require.Equal(t, float64(3), progress["total"])
	require.Equal(t, "tool", progress["unit"])

	tools := payload["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "grep", tools[0].(map[string]any)["name"])
}

func TestEmitStatusI18nFallsBackToEnglishAndNormalizesProgress(t *testing.T) {
	payload := newStatusPayload(
		"status",
		"",
		"Still working",
		WithStatusProgress(9, 3, " item "),
	)
	require.Equal(t, "Still working", payload.Value)
	require.Equal(t, int64(3), payload.Progress.Current)
	require.Equal(t, int64(3), payload.Progress.Total)
	require.Equal(t, "item", payload.Progress.Unit)
}

func TestSplitLegacyStatusI18n(t *testing.T) {
	zh, en := SplitLegacyStatusI18n("正在梳理思路 / Refining the approach")
	require.Equal(t, "正在梳理思路", zh)
	require.Equal(t, "Refining the approach", en)

	zh, en = SplitLegacyStatusI18n("正在检查 GET / api")
	require.Equal(t, "正在检查 GET / api", zh)
	require.Empty(t, en)

	zh, en = SplitLegacyStatusI18n("正在检查 GET / api / Checking GET /api")
	require.Equal(t, "正在检查 GET / api", zh)
	require.Equal(t, "Checking GET /api", en)

	zh, en = SplitLegacyStatusI18n("English only / still one message")
	require.Equal(t, "English only / still one message", zh)
	require.Empty(t, en)

	zh, en = SplitLegacyStatusI18n("正在提炼关键信息 - Refining relevant information")
	require.Equal(t, "正在提炼关键信息", zh)
	require.Equal(t, "Refining relevant information", en)
}
