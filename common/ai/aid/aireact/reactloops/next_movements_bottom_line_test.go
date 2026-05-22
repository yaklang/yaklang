package reactloops

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/schema"
)

// nextMovementsTrackableConfig 包装 mock.MockedAIConfig, 把 verification TODO
// 相关方法的调用次数 / 入参单独跟踪下来, 供兜底逻辑测试断言.
//
// 关键词: next_movements 兜底测试 config wrapper, ApplyVerificationTodoOps 跟踪
type nextMovementsTrackableConfig struct {
	*mock.MockedAIConfig

	mu              sync.Mutex
	applyCalls      int
	lastSatisfied   bool
	lastMovements   []aicommon.VerifyNextMovement
	markdownAsked   int
	markdownLastOps []aicommon.VerifyNextMovement
	markdownReturn  string
	snapshotItems   []aicommon.VerificationTodoItem
	snapshotStats   aicommon.VerificationTodoStats
}

func (c *nextMovementsTrackableConfig) ApplyVerificationTodoOps(satisfied bool, movements []aicommon.VerifyNextMovement) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.applyCalls++
	c.lastSatisfied = satisfied
	c.lastMovements = append([]aicommon.VerifyNextMovement(nil), movements...)
}

func (c *nextMovementsTrackableConfig) GetVerificationTodoMarkdownDelta(satisfied bool, movements []aicommon.VerifyNextMovement) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.markdownAsked++
	c.markdownLastOps = append([]aicommon.VerifyNextMovement(nil), movements...)
	return c.markdownReturn
}

func (c *nextMovementsTrackableConfig) SnapshotVerificationTodoItems() []aicommon.VerificationTodoItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]aicommon.VerificationTodoItem(nil), c.snapshotItems...)
}

func (c *nextMovementsTrackableConfig) GetVerificationTodoStats() aicommon.VerificationTodoStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotStats
}

// nextMovementsTrackableInvoker 包装 mock.MockInvoker, 把 GetConfig 重定向到
// 跟踪型 config, 并自行收集 AddToTimeline 调用 (mock 默认是空实现, 收不到
// timeline 信号).
//
// 关键词: next_movements 兜底测试 invoker wrapper, AddToTimeline 捕获
type nextMovementsTrackableInvoker struct {
	*mock.MockInvoker
	cfg *nextMovementsTrackableConfig

	mu              sync.Mutex
	timelineEntries []string
}

func (i *nextMovementsTrackableInvoker) GetConfig() aicommon.AICallerConfigIf {
	if i.cfg != nil {
		return i.cfg
	}
	return i.MockInvoker.GetConfig()
}

func (i *nextMovementsTrackableInvoker) AddToTimeline(entry, content string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.timelineEntries = append(i.timelineEntries, entry+": "+content)
}

func (i *nextMovementsTrackableInvoker) timelineString() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return strings.Join(i.timelineEntries, "\n")
}

// newNextMovementsTrackableSetup 构造测试所需的 invoker + config + 事件捕获
// 三件套. captureFn 不为 nil 时, 用它替换底层 emitter, 这样测试可以收集
// EmitTextMarkdownStreamEvent / EmitTodoListUpdate 等事件; 不传则用 mock 默认.
func newNextMovementsTrackableSetup(t *testing.T, ctx context.Context, captureFn func(*schema.AiOutputEvent)) (*nextMovementsTrackableInvoker, *nextMovementsTrackableConfig) {
	t.Helper()
	baseInvoker := mock.NewMockInvoker(ctx)
	mockCfg, ok := baseInvoker.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok, "expected base config to be *mock.MockedAIConfig")

	if captureFn != nil {
		mockCfg.Emitter = aicommon.NewEmitter("next-movements-bottom-line-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			captureFn(e)
			return e, nil
		})
	}

	cfg := &nextMovementsTrackableConfig{
		MockedAIConfig: mockCfg,
		snapshotItems: []aicommon.VerificationTodoItem{
			{ID: "test_employee_idor", Content: "test workspace IDOR", Status: "doing"},
		},
		snapshotStats: aicommon.VerificationTodoStats{
			Doing: 1,
		},
		markdownReturn: "- [+]: [id: test_employee_idor]: test workspace IDOR\n- [+]: [id: sqli_union_extract_data]: extract DB",
	}

	invoker := &nextMovementsTrackableInvoker{
		MockInvoker: baseInvoker,
		cfg:         cfg,
	}
	return invoker, cfg
}

func buildBottomLineAction(t *testing.T, payload, actionName string) *aicommon.Action {
	t.Helper()
	action, err := aicommon.ExtractAction(payload, actionName)
	require.NoError(t, err)
	return action
}

// TestApplyNextMovementsBottomLine_FiresForNonAdjustTodolistAction 验证: 当
// AI 选择 directly_call_tool 但 JSON 里"自作主张"携带 next_movements 字段时,
// 兜底逻辑必须立即 apply 到 TODO store 并广播完整事件三联 (snapshot /
// todo_list_update / timeline breadcrumb), 修复"待办事项 stream 已显示但
// store 永远不更新"的孤儿待办 bug.
//
// 关键词: 主 loop 兜底正向用例, 孤儿待办修复, directly_call_tool 携带
//
//	next_movements
func TestApplyNextMovementsBottomLine_FiresForNonAdjustTodolistAction(t *testing.T) {
	ctx := context.Background()
	var (
		mu       sync.Mutex
		captured []*schema.AiOutputEvent
	)
	invoker, cfg := newNextMovementsTrackableSetup(t, ctx, func(e *schema.AiOutputEvent) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, e)
	})

	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)

	// AI 输出 directly_call_tool 但 JSON 顺手列了两条 next_movements 增量.
	// 旧实现下: stream handler 会把它显示成"待办事项" (adjust_todolist 节点),
	// 但 ActionHandler 走 directly_call_tool 分支, 不会触发 apply + emit.
	action := buildBottomLineAction(t, `{
		"@action": "directly_call_tool",
		"directly_call_tool_name": "do_http_request",
		"next_movements": [
			{"op": "add", "id": "test_employee_idor", "content": "test workspace IDOR"},
			{"op": "add", "id": "sqli_union_extract_data", "content": "extract DB"}
		]
	}`, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)

	// 触发兜底
	applyNextMovementsBottomLine(loop, nil, 7, action)
	cfg.MockedAIConfig.Emitter.WaitForStream()

	// 1. store 必须被 apply 一次, satisfied=false (主循环兜底永远不抢
	//    verification 收口), movements 字节级保留
	cfg.mu.Lock()
	applyCalls := cfg.applyCalls
	lastSatisfied := cfg.lastSatisfied
	lastMovements := append([]aicommon.VerifyNextMovement(nil), cfg.lastMovements...)
	markdownAsked := cfg.markdownAsked
	cfg.mu.Unlock()

	require.Equal(t, 1, applyCalls,
		"bottom-line MUST apply movements exactly once when a non-adjust_todolist action carries next_movements")
	assert.False(t, lastSatisfied,
		"bottom-line satisfied flag must be false; main loop never claims verification outcome")
	require.Equal(t, 1, markdownAsked,
		"bottom-line must ask for markdown delta exactly once (apply 前算)")
	require.Len(t, lastMovements, 2)
	assert.Equal(t, "add", lastMovements[0].Op)
	assert.Equal(t, "test_employee_idor", lastMovements[0].ID)
	assert.Equal(t, "add", lastMovements[1].Op)
	assert.Equal(t, "sqli_union_extract_data", lastMovements[1].ID)

	// 2. EVENT_TYPE_TODO_LIST_UPDATE 必须 emit, 携带 movements
	mu.Lock()
	defer mu.Unlock()
	var (
		todoUpdateEvt   *schema.AiOutputEvent
		markdownEvtSeen bool
	)
	for _, e := range captured {
		if e.Type == schema.EVENT_TYPE_TODO_LIST_UPDATE {
			todoUpdateEvt = e
		}
		if e.NodeId == "next_movements_snapshot" && e.Type == schema.EVENT_TYPE_STREAM {
			markdownEvtSeen = true
		}
	}
	require.NotNil(t, todoUpdateEvt,
		"bottom-line MUST emit EVENT_TYPE_TODO_LIST_UPDATE so the frontend TODO panel refreshes (this is the missing piece in the orphan-todo bug)")
	assert.Contains(t, string(todoUpdateEvt.Content), "test_employee_idor",
		"todo_list_update payload should carry the snapshot items containing the new id")
	assert.True(t, markdownEvtSeen,
		"bottom-line MUST emit next_movements_snapshot markdown stream so the frontend '待办' panel renders the delta")

	// 3. NEXT_MOVEMENTS timeline breadcrumb 必须写入, 与 verification 路径
	//    使用同一个 timeline 类别
	tl := invoker.timelineString()
	assert.Contains(t, tl, "NEXT_MOVEMENTS")
	assert.Contains(t, tl, "ADD[test_employee_idor]: test workspace IDOR")
	assert.Contains(t, tl, "ADD[sqli_union_extract_data]: extract DB")
}

// TestApplyNextMovementsBottomLine_SkipsForAdjustTodolistAction 验证: 当
// actionType == adjust_todolist 时, 兜底必须**跳过**, 因为 adjust_todolist
// 自己的 ActionHandler 会调 applyAdjustTodolistMovements; 否则会发生双重
// apply / emit, 重复写入 store / 重复广播事件.
//
// 关键词: 兜底反向用例, adjust_todolist 避免双重 apply, 单一来源原则
func TestApplyNextMovementsBottomLine_SkipsForAdjustTodolistAction(t *testing.T) {
	ctx := context.Background()
	var (
		mu       sync.Mutex
		captured []*schema.AiOutputEvent
	)
	invoker, cfg := newNextMovementsTrackableSetup(t, ctx, func(e *schema.AiOutputEvent) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, e)
	})

	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)

	// adjust_todolist 自己的 schema 就有 next_movements 字段; 兜底必须识别
	// actionType 并跳过, 把 apply 让给 adjust_todolist 的 ActionHandler.
	action := buildBottomLineAction(t, `{
		"@action": "adjust_todolist",
		"next_movements": [
			{"op": "add", "id": "create_file", "content": "create A.md"}
		]
	}`, schema.AI_REACT_LOOP_ACTION_ADJUST_TODOLIST)

	applyNextMovementsBottomLine(loop, nil, 1, action)
	cfg.MockedAIConfig.Emitter.WaitForStream()

	cfg.mu.Lock()
	applyCalls := cfg.applyCalls
	markdownAsked := cfg.markdownAsked
	cfg.mu.Unlock()

	assert.Equal(t, 0, applyCalls,
		"bottom-line MUST skip adjust_todolist action; adjust_todolist's own ActionHandler is responsible for apply")
	assert.Equal(t, 0, markdownAsked,
		"bottom-line MUST not ask for markdown delta when skipping; otherwise the snapshot would be computed twice")

	mu.Lock()
	defer mu.Unlock()
	for _, e := range captured {
		assert.NotEqual(t, schema.EVENT_TYPE_TODO_LIST_UPDATE, e.Type,
			"bottom-line MUST not emit todo_list_update for adjust_todolist; that's the ActionHandler's job")
		assert.NotEqual(t, "next_movements_snapshot", e.NodeId,
			"bottom-line MUST not emit next_movements_snapshot for adjust_todolist")
	}
	assert.Empty(t, invoker.timelineString(),
		"bottom-line MUST not write NEXT_MOVEMENTS timeline for adjust_todolist")
}

// TestApplyNextMovementsBottomLine_NoopWhenNoMovements 验证: action 不含
// next_movements (或 movements 解析后为空) 时, 兜底必须是 no-op, 不能误触发
// 任何 apply / emit / timeline 写入.
//
// 关键词: 兜底反向用例, 无 movements 不触发, 不污染事件流
func TestApplyNextMovementsBottomLine_NoopWhenNoMovements(t *testing.T) {
	ctx := context.Background()
	var (
		mu       sync.Mutex
		captured []*schema.AiOutputEvent
	)
	invoker, cfg := newNextMovementsTrackableSetup(t, ctx, func(e *schema.AiOutputEvent) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, e)
	})

	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)

	// directly_call_tool 不携带 next_movements; 兜底应该是完全 no-op.
	action := buildBottomLineAction(t, `{
		"@action": "directly_call_tool",
		"directly_call_tool_name": "do_http_request"
	}`, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)

	applyNextMovementsBottomLine(loop, nil, 3, action)
	cfg.MockedAIConfig.Emitter.WaitForStream()

	cfg.mu.Lock()
	applyCalls := cfg.applyCalls
	markdownAsked := cfg.markdownAsked
	cfg.mu.Unlock()

	assert.Equal(t, 0, applyCalls,
		"bottom-line MUST be no-op when action carries no next_movements")
	assert.Equal(t, 0, markdownAsked,
		"bottom-line MUST not ask for markdown delta when there is nothing to apply")

	mu.Lock()
	defer mu.Unlock()
	for _, e := range captured {
		assert.NotEqual(t, schema.EVENT_TYPE_TODO_LIST_UPDATE, e.Type,
			"bottom-line MUST not emit todo_list_update when there are no movements")
		assert.NotEqual(t, "next_movements_snapshot", e.NodeId,
			"bottom-line MUST not emit next_movements_snapshot when there are no movements")
	}
	assert.Empty(t, invoker.timelineString(),
		"bottom-line MUST not write timeline when there are no movements")
}
