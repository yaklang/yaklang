package aid

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// makeTestCoordinatorAndTaskTree 构造一个最小化的 Coordinator + (root + 2 个
// 子任务) 树, 用来跑 GetUserInputSplitForCache 等纯纯逻辑测试.
//
// 关键词: 测试夹具, PE-TASK 子任务树, frozen user context
func makeTestCoordinatorAndTaskTree(t *testing.T, rootGoal, sub1Goal, sub2Goal, userInput string) (*Coordinator, *AiTask, *AiTask, *AiTask) {
	t.Helper()
	mem := GetDefaultContextProvider()
	cod := &Coordinator{
		Config:          &aicommon.Config{Ctx: context.Background()},
		ContextProvider: mem,
		userInput:       userInput,
	}
	root := cod.generateAITaskWithName("Root", rootGoal)
	root.Index = "1"
	sub1 := cod.generateAITaskWithName("Sub1", sub1Goal)
	sub1.Index = "1-1"
	sub1.ParentTask = root
	sub2 := cod.generateAITaskWithName("Sub2", sub2Goal)
	sub2.Index = "1-2"
	sub2.ParentTask = root
	root.Subtasks = []*AiTask{sub1, sub2}
	return cod, root, sub1, sub2
}

// TestGetUserInputSplitForCache_RootRawQueryOnly: root 任务 (ParentTask 为 nil)
// rawQuery 返回 root user input, frozenUserContext 为空。这是普通 ReAct loop /
// non-PE-TASK 路径的语义, 行为不能破坏。
//
// 关键词: GetUserInputSplitForCache, root task, 老路径不破坏
func TestGetUserInputSplitForCache_RootRawQueryOnly(t *testing.T) {
	mem := GetDefaultContextProvider()
	cod := &Coordinator{
		Config:          &aicommon.Config{Ctx: context.Background()},
		ContextProvider: mem,
		userInput:       "原始用户输入",
	}
	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"

	rawQuery, frozenCtx := root.GetUserInputSplitForCache()
	require.NotEmpty(t, rawQuery, "root task should expose its user input as rawQuery")
	require.Empty(t, frozenCtx, "root task should not expose frozenUserContext")
}

// TestGetUserInputSplitForCache_PETaskFreezesAll: 子任务 (ParentTask != nil)
// rawQuery 必须为空, frozenUserContext 包含完整的 PARENT_TASK / CURRENT_TASK /
// INSTRUCTION 三联块。
//
// 关键词: GetUserInputSplitForCache, PE-TASK 子任务, frozen user context
func TestGetUserInputSplitForCache_PETaskFreezesAll(t *testing.T) {
	_, _, sub, _ := makeTestCoordinatorAndTaskTree(
		t, "root goal", "sub1 goal", "sub2 goal", "渗透测试，http://127.0.0.1:18080",
	)

	rawQuery, frozenCtx := sub.GetUserInputSplitForCache()
	require.Empty(t, rawQuery, "PE-TASK subtask rawQuery should be empty (all moved to frozen)")
	require.NotEmpty(t, frozenCtx, "PE-TASK subtask should expose frozenUserContext")
	require.Contains(t, frozenCtx, "PARENT_TASK_", "frozenUserContext must contain PARENT_TASK block")
	require.Contains(t, frozenCtx, "CURRENT_TASK_", "frozenUserContext must contain CURRENT_TASK block")
	require.Contains(t, frozenCtx, "INSTRUCTION_", "frozenUserContext must contain INSTRUCTION block")
	require.Contains(t, frozenCtx, "渗透测试，http://127.0.0.1:18080", "frozenUserContext must include raw user input as prefix")
}

// TestGetUserInputSplitForCache_NonceStableAcrossCalls: 同一个子任务对象多次
// 调用 GetUserInputSplitForCache, frozenUserContext 字节必须完全一致 (反
// RandStringBytes 反模式回归).
//
// 关键词: GetUserInputSplitForCache, 跨调用稳定, prefix cache, 反 RandStringBytes
func TestGetUserInputSplitForCache_NonceStableAcrossCalls(t *testing.T) {
	_, _, sub, _ := makeTestCoordinatorAndTaskTree(
		t, "root goal", "sub1 goal", "sub2 goal", "user input",
	)

	_, frozenCtx1 := sub.GetUserInputSplitForCache()
	_, frozenCtx2 := sub.GetUserInputSplitForCache()
	_, frozenCtx3 := sub.GetUserInputSplitForCache()
	require.Equal(t, frozenCtx1, frozenCtx2, "frozenUserContext should be byte-stable across calls (call 1 vs 2)")
	require.Equal(t, frozenCtx2, frozenCtx3, "frozenUserContext should be byte-stable across calls (call 2 vs 3)")
}

// TestGetUserInputSplitForCache_PrefixSharedAcrossSubtasks: 同一个 root 下
// 不同子任务的 frozenUserContext "前缀" (RawUserInput + PARENT_TASK 块) 必须
// 字节相同, 仅在 CURRENT_TASK 段开始时分叉。这是 prefix cache 命中的关键。
//
// 关键词: GetUserInputSplitForCache, 跨子任务前缀稳定, PARENT_TASK 段, prefix cache
func TestGetUserInputSplitForCache_PrefixSharedAcrossSubtasks(t *testing.T) {
	_, _, sub1, sub2 := makeTestCoordinatorAndTaskTree(
		t, "root goal", "sub1 goal", "sub2 goal", "渗透测试，http://127.0.0.1:18080",
	)

	_, frozen1 := sub1.GetUserInputSplitForCache()
	_, frozen2 := sub2.GetUserInputSplitForCache()
	require.NotEqual(t, frozen1, frozen2, "different subtasks must produce different full frozenUserContext (CURRENT_TASK differs)")

	// 两个子任务从 0 到 CURRENT_TASK 开始处的字节必须完全相同.
	currentMarkerSub1 := strings.Index(frozen1, "<|CURRENT_TASK_")
	currentMarkerSub2 := strings.Index(frozen2, "<|CURRENT_TASK_")
	require.Greaterf(t, currentMarkerSub1, 0, "sub1 frozenUserContext must contain CURRENT_TASK marker")
	require.Equalf(t, currentMarkerSub1, currentMarkerSub2, "CURRENT_TASK marker offset must match for prefix sharing")

	prefixSub1 := frozen1[:currentMarkerSub1]
	prefixSub2 := frozen2[:currentMarkerSub2]
	require.Equal(t, prefixSub1, prefixSub2,
		"frozenUserContext prefix (RawUserInput + PARENT_TASK block) must be byte-identical across subtasks of the same root")
}

// TestGetUserInputSplitForCache_DiffRootDiffNonce: 不同 Coordinator (不同 plan
// 周期 / 不同 root user input) 派生的 nonce 不同。
//
// 关键词: GetUserInputSplitForCache, plan epoch 隔离
func TestGetUserInputSplitForCache_DiffRootDiffNonce(t *testing.T) {
	_, _, sub1A, _ := makeTestCoordinatorAndTaskTree(
		t, "root goal A", "sub1 goal", "sub2 goal", "user input A",
	)
	_, _, sub1B, _ := makeTestCoordinatorAndTaskTree(
		t, "root goal B", "sub1 goal", "sub2 goal", "user input B",
	)

	_, frozenA := sub1A.GetUserInputSplitForCache()
	_, frozenB := sub1B.GetUserInputSplitForCache()
	require.NotEqual(t, frozenA, frozenB,
		"different plan epoch (different root user input) must produce different frozenUserContext")
}

// TestGetUserInput_BackwardCompatible: 老 GetUserInput 调用点必须保持原语义,
// 新接口 GetUserInputSplitForCache 不能破坏老路径。整体字符串 = rawQuery + frozenCtx。
//
// 关键词: GetUserInput 老语义兼容
func TestGetUserInput_BackwardCompatible(t *testing.T) {
	_, _, sub, _ := makeTestCoordinatorAndTaskTree(
		t, "root goal", "sub1 goal", "sub2 goal", "user input",
	)

	full := sub.GetUserInput()
	rawQuery, frozenCtx := sub.GetUserInputSplitForCache()
	require.Equal(t, full, rawQuery+frozenCtx,
		"GetUserInput should equal rawQuery + frozenUserContext to preserve old semantics")
}
