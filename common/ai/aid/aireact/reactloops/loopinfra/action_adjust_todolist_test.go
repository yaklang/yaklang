package loopinfra

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

// adjustTodolistTestConfig 包装 mock.MockedAIConfig, 把对 TODO store 的读写
// 重定向到本地跟踪字段, 用于断言 adjust_todolist 的 handler 是否真把
// movements 应用到了 store 接口上, 以及发出 EVENT_TYPE_TODO_LIST_UPDATE
// 时使用的 snapshot / stats 是否来自 store 接口.
//
// 关键词: adjust_todolist 测试 config, ApplyVerificationTodoOps 跟踪,
//
//	SnapshotVerificationTodoItems 注入
type adjustTodolistTestConfig struct {
	*mock.MockedAIConfig
	mu              sync.Mutex
	applyCalls      int
	lastScope       aicommon.VerificationTodoScope
	lastSatisfied   bool
	lastMovements   []aicommon.VerifyNextMovement
	snapshotItems   []aicommon.VerificationTodoItem
	snapshotStats   aicommon.VerificationTodoStats
	markdownReturn  string
	markdownAsked   int
	markdownScope   aicommon.VerificationTodoScope
	markdownLastOps []aicommon.VerifyNextMovement
}

func (c *adjustTodolistTestConfig) ApplyVerificationTodoOps(scope aicommon.VerificationTodoScope, satisfied bool, movements []aicommon.VerifyNextMovement) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.applyCalls++
	c.lastScope = scope
	c.lastSatisfied = satisfied
	c.lastMovements = append([]aicommon.VerifyNextMovement(nil), movements...)
}

func (c *adjustTodolistTestConfig) SnapshotVerificationTodoItems() []aicommon.VerificationTodoItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]aicommon.VerificationTodoItem(nil), c.snapshotItems...)
}

func (c *adjustTodolistTestConfig) GetVerificationTodoStats() aicommon.VerificationTodoStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotStats
}

func (c *adjustTodolistTestConfig) SnapshotVerificationTodoItemsByScope(scope aicommon.VerificationTodoScope) []aicommon.VerificationTodoItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]aicommon.VerificationTodoItem(nil), c.snapshotItems...)
}

func (c *adjustTodolistTestConfig) GetVerificationTodoStatsByScope(scope aicommon.VerificationTodoScope) aicommon.VerificationTodoStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotStats
}

func (c *adjustTodolistTestConfig) HasActiveVerificationTodosByScope(scope aicommon.VerificationTodoScope) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotStats.Pending+c.snapshotStats.Doing > 0
}

func (c *adjustTodolistTestConfig) ActiveVerificationTodoItemsByScope(scope aicommon.VerificationTodoScope) []aicommon.VerificationTodoItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []aicommon.VerificationTodoItem
	for _, item := range c.snapshotItems {
		if item.Status == aicommon.VerificationTodoStatusPending || item.Status == aicommon.VerificationTodoStatusDoing {
			out = append(out, item)
		}
	}
	return out
}

func (c *adjustTodolistTestConfig) GetVerificationTodoMarkdownDelta(scope aicommon.VerificationTodoScope, satisfied bool, movements []aicommon.VerifyNextMovement) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.markdownAsked++
	c.markdownScope = scope
	c.markdownLastOps = append([]aicommon.VerifyNextMovement(nil), movements...)
	return c.markdownReturn
}

type adjustTodolistTestInvoker struct {
	*testInvoker
	cfg *adjustTodolistTestConfig
}

func (i *adjustTodolistTestInvoker) GetConfig() aicommon.AICallerConfigIf {
	if i.cfg != nil {
		return i.cfg
	}
	return i.testInvoker.GetConfig()
}

// newAdjustTodolistInvoker 构造一个带跟踪的 test invoker + test config 组合.
// captureFn 不为 nil 时, 用它替换底层 emitter, 这样测试可以捕获
// EVENT_TYPE_TODO_LIST_UPDATE 等结构化事件; 不传则保留 mock 默认 emitter.
//
// 关键词: adjust_todolist 测试工厂, emitter 注入
func newAdjustTodolistInvoker(t *testing.T, ctx context.Context, captureFn func(*schema.AiOutputEvent)) (*adjustTodolistTestInvoker, *adjustTodolistTestConfig) {
	t.Helper()
	base := newTestInvoker(ctx)
	mockCfg, ok := base.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok, "expected base config to be *mock.MockedAIConfig")

	if captureFn != nil {
		mockCfg.Emitter = aicommon.NewEmitter("adjust-todolist-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			captureFn(e)
			return e, nil
		})
	}

	wrapper := &adjustTodolistTestConfig{
		MockedAIConfig: mockCfg,
		snapshotItems: []aicommon.VerificationTodoItem{
			{ID: "create_file", Content: "create A.md", Status: "doing"},
		},
		snapshotStats:  aicommon.VerificationTodoStats{Doing: 1},
		markdownReturn: "- markdown delta",
	}
	return &adjustTodolistTestInvoker{
		testInvoker: base,
		cfg:         wrapper,
	}, wrapper
}

func buildAdjustTodolistAction(t *testing.T, payload string) *aicommon.Action {
	t.Helper()
	action, err := aicommon.ExtractAction(payload, schema.AI_REACT_LOOP_ACTION_ADJUST_TODOLIST)
	require.NoError(t, err)
	return action
}

// TestAdjustTodolist_Verifier_RejectsEmptyMovements 验证: verifier 在
// movements 为空 / 缺字段的情况下应直接返回错误, 不允许进入 handler 空转.
// 关键词: adjust_todolist verifier 非空校验
func TestAdjustTodolist_Verifier_RejectsEmptyMovements(t *testing.T) {
	ctx := context.Background()
	invoker, _ := newAdjustTodolistInvoker(t, ctx, nil)
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)

	cases := []struct {
		name    string
		payload string
	}{
		{"completely empty", `{"@action":"adjust_todolist"}`},
		{"empty array", `{"@action":"adjust_todolist","next_movements":[]}`},
		{"missing id", `{"@action":"adjust_todolist","next_movements":[{"op":"add","content":"x"}]}`},
		{"missing op", `{"@action":"adjust_todolist","next_movements":[{"id":"a","content":"x"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action := buildAdjustTodolistAction(t, tc.payload)
			err := loopAction_AdjustTodolist.ActionVerifier(loop, action)
			assert.Error(t, err, "verifier must reject empty / malformed movements")
		})
	}
}

// TestAdjustTodolist_Verifier_NormalizesPendingToDoing 验证: verifier 复用
// aicommon.NormalizeVerifyNextMovements, 把历史 op=pending 统一为 doing,
// 并把归一化结果缓存到 loop 变量供 handler 复用.
// 关键词: adjust_todolist verifier 归一化, pending->doing 缓存
func TestAdjustTodolist_Verifier_NormalizesPendingToDoing(t *testing.T) {
	ctx := context.Background()
	invoker, _ := newAdjustTodolistInvoker(t, ctx, nil)
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)

	action := buildAdjustTodolistAction(t, `{
		"@action": "adjust_todolist",
		"next_movements": [
			{"op": "pending", "id": "create_file"}
		]
	}`)
	require.NoError(t, loopAction_AdjustTodolist.ActionVerifier(loop, action))

	cached, ok := loop.GetVariable("adjust_todolist_movements").([]aicommon.VerifyNextMovement)
	require.True(t, ok, "verifier should cache normalized movements into loop variables")
	require.Len(t, cached, 1)
	assert.Equal(t, "doing", cached[0].Op)
	assert.Equal(t, "create_file", cached[0].ID)
}

// TestAdjustTodolist_Handler_AppliesAddOpAndEmitsTodoListUpdate 验证: handler
// 把 add op 透传给 ApplyVerificationTodoOps(satisfied=false, ...) 并以
// EVENT_TYPE_TODO_LIST_UPDATE 形式把 store snapshot 广播出去, 同时把
// breadcrumb 写进 timeline 的 NEXT_MOVEMENTS 键, 与 verification 路径对齐.
// 关键词: adjust_todolist handler add op, EVENT_TYPE_TODO_LIST_UPDATE,
//
//	NEXT_MOVEMENTS timeline 对齐
func TestAdjustTodolist_Handler_AppliesAddOpAndEmitsTodoListUpdate(t *testing.T) {
	ctx := context.Background()
	captured := make([]*schema.AiOutputEvent, 0, 4)
	mu := new(sync.Mutex)
	captureFn := func(e *schema.AiOutputEvent) {
		mu.Lock()
		captured = append(captured, e)
		mu.Unlock()
	}
	invoker, cfg := newAdjustTodolistInvoker(t, ctx, captureFn)

	task := newTestTask(ctx)
	invoker.testInvoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.SetCurrentTask(task)

	action := buildAdjustTodolistAction(t, `{
		"@action": "adjust_todolist",
		"next_movements": [
			{"op": "add", "id": "create_file", "content": "create A.md"},
			{"op": "done", "id": "cleanup_temp"}
		]
	}`)

	require.NoError(t, loopAction_AdjustTodolist.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	loopAction_AdjustTodolist.ActionHandler(loop, action, op)

	cfg.mu.Lock()
	applyCalls := cfg.applyCalls
	lastScope := cfg.lastScope
	lastSatisfied := cfg.lastSatisfied
	lastMovements := append([]aicommon.VerifyNextMovement(nil), cfg.lastMovements...)
	cfg.mu.Unlock()

	require.Equal(t, 1, applyCalls, "handler must invoke ApplyVerificationTodoOps exactly once")
	assert.Equal(t, task.GetId(), lastScope.TaskID)
	assert.Equal(t, task.GetIndex(), lastScope.TaskIndex)
	assert.False(t, lastSatisfied, "satisfied must be false: 主循环增量不抢 verification 收口")
	require.Len(t, lastMovements, 2)
	assert.Equal(t, "add", lastMovements[0].Op)
	assert.Equal(t, "create_file", lastMovements[0].ID)
	assert.Equal(t, "done", lastMovements[1].Op)
	assert.Equal(t, "cleanup_temp", lastMovements[1].ID)

	mu.Lock()
	defer mu.Unlock()
	var todoEvent *schema.AiOutputEvent
	for _, e := range captured {
		if e.Type == schema.EVENT_TYPE_TODO_LIST_UPDATE {
			todoEvent = e
			break
		}
	}
	require.NotNil(t, todoEvent, "expected an EVENT_TYPE_TODO_LIST_UPDATE event")
	require.True(t, todoEvent.IsJson, "todo_list_update payload should be JSON")
	bodyStr := string(todoEvent.Content)
	assert.Contains(t, bodyStr, "create_file")
	assert.Contains(t, bodyStr, `"satisfied":false`)

	assert.True(t, op.IsContinued(), "handler should call operator.Continue() after applying delta")
	feedback := op.GetFeedback().String()
	assert.True(t, strings.Contains(feedback, "TODO list adjusted"),
		"feedback should announce the TODO adjustment, got %q", feedback)

	// timeline breadcrumb should have one entry under NEXT_MOVEMENTS, matching
	// the verification path's key (so consumers see a unified chronology).
	tlString := invoker.testInvoker.getTimelineString()
	assert.Contains(t, tlString, "NEXT_MOVEMENTS")
	assert.Contains(t, tlString, "ADD[create_file]: create A.md")
	assert.Contains(t, tlString, "DONE[cleanup_temp]")
}

// TestAdjustTodolist_Handler_DoesNotEmitNextMovementsSnapshotStream 验证:
// handler 只广播 todo_list_update, 不再 emit next_movements_snapshot 聊天卡片.
// 关键词: adjust_todolist 不发待办流, todo_list_update 单通道
func TestAdjustTodolist_Handler_DoesNotEmitNextMovementsSnapshotStream(t *testing.T) {
	ctx := context.Background()
	captured := make([]*schema.AiOutputEvent, 0, 4)
	mu := new(sync.Mutex)
	captureFn := func(e *schema.AiOutputEvent) {
		mu.Lock()
		captured = append(captured, e)
		mu.Unlock()
	}
	invoker, cfg := newAdjustTodolistInvoker(t, ctx, captureFn)

	task := newTestTask(ctx)
	invoker.testInvoker.currentTask = task
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.SetCurrentTask(task)

	action := buildAdjustTodolistAction(t, `{
		"@action": "adjust_todolist",
		"next_movements": [{"op": "add", "id": "x", "content": "x"}]
	}`)
	require.NoError(t, loopAction_AdjustTodolist.ActionVerifier(loop, action))
	loopAction_AdjustTodolist.ActionHandler(
		loop, action, reactloops.NewActionHandlerOperator(task),
	)
	cfg.MockedAIConfig.Emitter.WaitForStream()

	mu.Lock()
	defer mu.Unlock()
	for _, e := range captured {
		assert.NotEqual(t, "next_movements_snapshot", e.NodeId,
			"adjust_todolist must not emit next_movements_snapshot stream")
	}
}

// TestAdjustTodolistNextMovementsStreamHandler_TranslatesJSONToDisplayLines
// 验证: stream handler 把 AI 流出来的 next_movements JSON 数组**实时**翻译
// 成 verification 路径同字节的 display 行 (`- [+]: [id: x]: y`), 前端在
// "adjust_todolist" 节点 (UI 文案 "待办事项") 上看到的不再是裸 JSON.
//
// 关键词: adjust_todolist next_movements StreamHandler, JSON→display 行,
//
//	verification 字节级对齐, 不裸 JSON
func TestAdjustTodolistNextMovementsStreamHandler_TranslatesJSONToDisplayLines(t *testing.T) {
	input := `[
		{"op":"add","id":"recon_dns","content":"DNS 信息收集: dig id.redhaze.top"},
		{"op":"done","id":"old_step"},
		{"op":"doing","id":"step2","content":"扫描"},
		{"op":"delete","id":"dropped"},
		{"op":"skip","id":"skipped_branch"}
	]`

	var out bytes.Buffer
	adjustTodolistNextMovementsStreamHandler(strings.NewReader(input), &out)
	rendered := out.String()

	// 与 verification 路径完全一致的 marker 集合
	assert.Contains(t, rendered, "- [+]: [id: recon_dns]: DNS 信息收集: dig id.redhaze.top")
	assert.Contains(t, rendered, "- [x]: [id: old_step]")
	assert.Contains(t, rendered, "- [DOING]: [id: step2]: 扫描")
	assert.Contains(t, rendered, "- [DELETED]: [id: dropped]")
	assert.Contains(t, rendered, "- [SKIPPED]: [id: skipped_branch]")

	// display 行之间用 "\n" 分隔, 没有裸 `{"op":`
	assert.NotContains(t, rendered, `{"op":`,
		"display stream must not leak raw JSON tokens to the frontend")
	assert.Equal(t, 5, strings.Count(rendered, "\n- ")+1,
		"expected one display line per movement (got %q)", rendered)
}

// TestAdjustTodolistNextMovementsStreamHandler_DrainsOnNonArrayInput 验证:
// 当 AI 误把 next_movements 写成非数组形态 (例如裸字符串 / 半截 JSON) 时,
// handler 必须把 fieldReader 全部排空, 否则上游 producer 会卡在 pipe 上
// 阻塞整个 react loop.
//
// 关键词: stream handler 容错, drain pipe 防卡死, 非数组兜底
func TestAdjustTodolistNextMovementsStreamHandler_DrainsOnNonArrayInput(t *testing.T) {
	garbage := strings.Repeat("not-an-array ", 64)
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_, _ = io.WriteString(pw, garbage)
	}()

	done := make(chan struct{})
	var out bytes.Buffer
	go func() {
		defer close(done)
		adjustTodolistNextMovementsStreamHandler(pr, &out)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler must not block on malformed (non-array) input")
	}
	// 既然原始输入不是合法 JSON 数组, display stream 应该几乎不输出有效内容,
	// 但绝不应该把裸 garbage 透传到前端.
	assert.NotContains(t, out.String(), "not-an-array",
		"non-array garbage must never reach the display stream emitter")
}

// TestAdjustTodolist_Handler_RecoversWithoutVerifierCache 验证: 即使 handler
// 在脱离 verifier 缓存 (例如直接被调用) 的场景下, 仍能从 action 上重新解析
// movements 并完成 apply / emit, 保证幂等.
// 关键词: adjust_todolist handler 兜底, 脱离 verifier 缓存
func TestAdjustTodolist_Handler_RecoversWithoutVerifierCache(t *testing.T) {
	ctx := context.Background()
	invoker, cfg := newAdjustTodolistInvoker(t, ctx, nil)
	task := newTestTask(ctx)
	invoker.testInvoker.currentTask = task
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.SetCurrentTask(task)

	action := buildAdjustTodolistAction(t, `{
		"@action": "adjust_todolist",
		"next_movements": [
			{"op": "skip", "id": "abandoned_branch"}
		]
	}`)

	// 故意跳过 verifier, 直接调 handler
	op := reactloops.NewActionHandlerOperator(task)
	loopAction_AdjustTodolist.ActionHandler(loop, action, op)

	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	require.Equal(t, 1, cfg.applyCalls, "handler must still apply when verifier cache is absent")
	require.Len(t, cfg.lastMovements, 1)
	assert.Equal(t, "skip", cfg.lastMovements[0].Op)
	assert.Equal(t, "abandoned_branch", cfg.lastMovements[0].ID)
}
