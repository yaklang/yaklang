package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestNormalizeVerifyNextMovements_LegacyStringFallback 验证: 没有结构化数组,
// 仅有顶层 next_movements 字符串时, 应被兼容成一条 add/legacy 项, 用于回收
// 老 prompt 输出.
//
// 关键词: NormalizeVerifyNextMovements legacy string 兼容
func TestNormalizeVerifyNextMovements_LegacyStringFallback(t *testing.T) {
	action := NewSimpleAction("verify-satisfaction", aitool.InvokeParams{
		"next_movements": "继续读取目标文件并确认内容是否正确",
	})

	movements := NormalizeVerifyNextMovements(action)
	require.Len(t, movements, 1)
	require.Equal(t, "add", movements[0].Op)
	require.Equal(t, "legacy_next_movements", movements[0].ID)
	require.Equal(t, "继续读取目标文件并确认内容是否正确", movements[0].Content)
}

// TestNormalizeVerifyNextMovements_PrefersStructuredArray 验证: 结构化数组
// 存在时优先采用, 不会再 fallback 到 legacy 字符串.
//
// 关键词: NormalizeVerifyNextMovements 结构化优先
func TestNormalizeVerifyNextMovements_PrefersStructuredArray(t *testing.T) {
	action, err := ExtractActionFromStream(context.Background(), strings.NewReader(`{
		"@action": "verify-satisfaction",
		"next_movements": [
			{"op": "add", "id": "create_file", "content": "创建 A.md"},
			{"op": "done", "id": "cleanup_temp"},
			{"op": "delete", "id": "stale_todo"}
		]
	}`), "verify-satisfaction")
	require.NoError(t, err)

	movements := NormalizeVerifyNextMovements(action)
	require.Len(t, movements, 3)
	require.Equal(t, "create_file", movements[0].ID)
	require.Equal(t, "cleanup_temp", movements[1].ID)
	require.Equal(t, "delete", movements[2].Op)
	require.Equal(t, "stale_todo", movements[2].ID)
}

// TestNormalizeVerifyNextMovements_NormalizesPendingToDoing 验证: 历史
// op=pending 被统一收敛为 doing, 下游 store 只需识别一种 in-progress 状态.
//
// 关键词: NormalizeVerifyNextMovements pending->doing 归一
func TestNormalizeVerifyNextMovements_NormalizesPendingToDoing(t *testing.T) {
	action, err := ExtractActionFromStream(context.Background(), strings.NewReader(`{
		"@action": "verify-satisfaction",
		"next_movements": [
			{"op": "pending", "id": "create_file"}
		]
	}`), "verify-satisfaction")
	require.NoError(t, err)

	movements := NormalizeVerifyNextMovements(action)
	require.Len(t, movements, 1)
	require.Equal(t, "doing", movements[0].Op)
	require.Equal(t, "create_file", movements[0].ID)
}

// TestNormalizeVerifyNextMovements_DropsEmptyOpOrId 验证: 缺少 op 或 id 的项
// 会被静默丢弃, 不会污染 store; 只剩有效项时不会再 fallback 到 legacy.
//
// 关键词: NormalizeVerifyNextMovements 丢弃缺字段项
func TestNormalizeVerifyNextMovements_DropsEmptyOpOrId(t *testing.T) {
	action, err := ExtractActionFromStream(context.Background(), strings.NewReader(`{
		"@action": "verify-satisfaction",
		"next_movements": [
			{"op": "", "id": "missing_op"},
			{"op": "add", "id": ""},
			{"op": "add", "id": "valid_one", "content": "ok"}
		]
	}`), "verify-satisfaction")
	require.NoError(t, err)

	movements := NormalizeVerifyNextMovements(action)
	require.Len(t, movements, 1)
	require.Equal(t, "valid_one", movements[0].ID)
	require.Equal(t, "add", movements[0].Op)
}

// TestNormalizeVerifyNextMovements_AllFiveOpsPreserved 验证: 五个合法 op
// (add / doing / done / delete / skip) 都保留, 不会被规则误杀.
//
// 关键词: NormalizeVerifyNextMovements 五种 op 完整支持
func TestNormalizeVerifyNextMovements_AllFiveOpsPreserved(t *testing.T) {
	action, err := ExtractActionFromStream(context.Background(), strings.NewReader(`{
		"@action": "verify-satisfaction",
		"next_movements": [
			{"op": "add", "id": "a", "content": "x"},
			{"op": "doing", "id": "b"},
			{"op": "done", "id": "c"},
			{"op": "delete", "id": "d"},
			{"op": "skip", "id": "e"}
		]
	}`), "verify-satisfaction")
	require.NoError(t, err)

	movements := NormalizeVerifyNextMovements(action)
	require.Len(t, movements, 5)
	opSet := map[string]struct{}{}
	for _, m := range movements {
		opSet[m.Op] = struct{}{}
	}
	require.Equal(t, map[string]struct{}{
		"add": {}, "doing": {}, "done": {}, "delete": {}, "skip": {},
	}, opSet)
}

// TestNormalizeVerifyNextMovements_NilAction 验证: 空指针保护, 不应 panic,
// 返回 nil.
//
// 关键词: NormalizeVerifyNextMovements nil action 保护
func TestNormalizeVerifyNextMovements_NilAction(t *testing.T) {
	require.Nil(t, NormalizeVerifyNextMovements(nil))
}

// TestFormatNextMovementsBreadcrumb_RendersAllShapes 验证: breadcrumb 行
// 在 id-only / content-only / 双有 三种形状下都能渲染, 与 verification 的
// timeline 写入格式 1:1 对齐.
//
// 关键词: FormatNextMovementsBreadcrumb 三形态渲染
func TestFormatNextMovementsBreadcrumb_RendersAllShapes(t *testing.T) {
	got := FormatNextMovementsBreadcrumb([]VerifyNextMovement{
		{Op: "add", ID: "create_file", Content: "创建 A.md"},
		{Op: "done", ID: "cleanup_temp"},
		{Op: "", ID: "", Content: "freeform"},
	})
	require.Equal(t,
		"ADD[create_file]: 创建 A.md\nDONE[cleanup_temp]\nADD: freeform",
		got,
	)
}

// TestFormatNextMovementsBreadcrumb_EmptyInput 验证: 输入为 nil / 空时,
// breadcrumb 返回空字符串, 调用方据此可跳过 timeline 写入.
//
// 关键词: FormatNextMovementsBreadcrumb 空输入兜底
func TestFormatNextMovementsBreadcrumb_EmptyInput(t *testing.T) {
	require.Empty(t, FormatNextMovementsBreadcrumb(nil))
	require.Empty(t, FormatNextMovementsBreadcrumb([]VerifyNextMovement{}))
	require.Empty(t, FormatNextMovementsBreadcrumb([]VerifyNextMovement{{Op: "", ID: "", Content: ""}}))
}
