package reactloops

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// extTestConfig 是 maxIterTestConfig 的变体: 重写 Feed /
// SubmitCheckpointRequest, 让 requestIterationExtension 的交互流程可被测试驱动.

// extTestInvoker 是 maxIterTestInvoker 的变体, 绑定 extTestConfig.
type extTestInvoker struct {
	*mock.MockInvoker
	cfg         *extTestConfig
	currentTask aicommon.AIStatefulTask
	mu          sync.Mutex
	timeline    []string
}

func (i *extTestInvoker) GetConfig() aicommon.AICallerConfigIf { return i.cfg }

func (i *extTestInvoker) SetCurrentTask(task aicommon.AIStatefulTask) { i.currentTask = task }

func (i *extTestInvoker) GetCurrentTask() aicommon.AIStatefulTask { return i.currentTask }

func (i *extTestInvoker) GetCurrentTaskId() string {
	if i.currentTask == nil {
		return ""
	}
	return i.currentTask.GetId()
}

func (i *extTestInvoker) AddToTimeline(entry, content string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.timeline = append(i.timeline, entry+": "+content)
}

type extTestConfig struct {
	*mock.MockedAIConfig

	mu               sync.Mutex
	feedParams       aitool.InvokeParams // 用户回复 (Feed 传入)
	fed              bool
	submitCpErr      error
	emitInteractIDs  []string
	endpointManager  *aicommon.EndpointManager
}

// GetEndpointManager 返回可控的 EndpointManager (由测试构造), 覆盖 BaseInteractiveHandler.
func (c *extTestConfig) GetEndpointManager() *aicommon.EndpointManager {
	return c.endpointManager
}

// Feed 拦截用户回复: 把预设的 feedParams 注入 endpoint 并立即释放信号.
func (c *extTestConfig) Feed(endpointId string, params aitool.InvokeParams) {
	c.mu.Lock()
	c.fed = true
	fp := c.feedParams
	c.mu.Unlock()
	ep, ok := c.endpointManager.LoadEndpoint(endpointId)
	if !ok {
		return
	}
	ep.ActiveWithParams(context.Background(), fp)
}

// DoWaitAgree 覆盖 BaseInteractiveHandler: 直接把预设的用户回复注入 endpoint 并返回,
// 模拟用户已响应 (不阻塞, 不需要外部 Feed).
func (c *extTestConfig) DoWaitAgree(ctx context.Context, endpoint *aicommon.Endpoint) {
	c.mu.Lock()
	fp := c.feedParams
	c.fed = true
	c.mu.Unlock()
	if !utils.IsNil(fp) {
		endpoint.ActiveWithParams(context.Background(), fp)
	} else {
		endpoint.Release()
	}
}

// SubmitCheckpointRequest 覆盖: 跳过 DB, 记录是否被调用, 可注入错误.
func (c *extTestConfig) SubmitCheckpointRequest(checkpoint *schema.AiCheckpoint, req any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.submitCpErr
}


func newExtTestConfig(ctx context.Context, feedParams aitool.InvokeParams) *extTestConfig {
	base, ok := mock.NewMockedAIConfig(ctx).(*mock.MockedAIConfig)
	_ = base
	_ = ok
	// 用 NewMockedAIConfig 作为底座, 但 GetEndpointManager/Feed/DoWaitAgree/SubmitCheckpointRequest
	// 都由 extTestConfig 重写, 所以 BaseInteractiveHandler 的对应方法不会被调用.
	cfg := &extTestConfig{
		MockedAIConfig:   base,
		feedParams:       feedParams,
		endpointManager:  aicommon.NewEndpointManagerContext(ctx),
	}
	// EndpointManager 在 CreateEndpoint 时会调用 config.AcquireId / GetRuntimeId / GetDB /
	// CreateReviewCheckpoint. 让它走通: 用 cfg 自身的 AcquireId/GetRuntimeId (来自 MockedAIConfig),
	// GetDB 返回 nil 时 CreateReviewCheckpoint 内部会 log.Error 但不 panic, endpoint.checkpoint 仍非 nil.
	return cfg
}

// newExtTestLoop 构造一个用于 requestIterationExtension 测试的 ReActLoop.
func newExtTestLoop(t *testing.T, cfg *extTestConfig) (*ReActLoop, aicommon.AIStatefulTask) {
	t.Helper()
	ctx := context.Background()
	baseInvoker := mock.NewMockInvoker(ctx)
	// 让 invoker 的 config 指向 extTestConfig
	baseInvoker.SetConfig(cfg)
	invoker := &extTestInvoker{
		MockInvoker: baseInvoker,
		cfg:         cfg,
	}
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("ext-test-task", "测试迭代扩充", ctx, cfg.GetEmitter(), true)
	invoker.SetCurrentTask(task)
	loop.SetCurrentTask(task)
	return loop, task
}

// TestRequestIterationExtension_UserAgree 验证用户选择 +5:
// requestIterationExtension 返回 agreed=true, delta=5, 且扩充计数 +1.
func TestRequestIterationExtension_UserAgree(t *testing.T) {
	ctx := context.Background()
	feedParams := aitool.InvokeParams{
		"suggestion": "+5",
	}
	cfg := newExtTestConfig(ctx, feedParams)
	loop, task := newExtTestLoop(t, cfg)

	agreed, delta, err := loop.requestIterationExtension(task, 11, 10)
	require.NoError(t, err)
	require.True(t, agreed)
	assert.Equal(t, 5, delta)
	assert.Equal(t, 1, loop.getIterationExtensionCount())
	assert.True(t, cfg.fed, "Feed should have been called to deliver user response")
}

// TestRequestIterationExtension_UserDecline 验证用户拒绝:
// 返回 agreed=false, 扩充计数不增加.
func TestRequestIterationExtension_UserDecline(t *testing.T) {
	ctx := context.Background()
	feedParams := aitool.InvokeParams{
		"suggestion": "停止",
	}
	// "停止" 不在固定选项 (+5/+10/翻倍) 中, 视为拒绝
	cfg := newExtTestConfig(ctx, feedParams)
	loop, task := newExtTestLoop(t, cfg)

	agreed, delta, err := loop.requestIterationExtension(task, 11, 10)
	require.NoError(t, err)
	require.False(t, agreed)
	assert.Equal(t, 0, delta)
	assert.Equal(t, 0, loop.getIterationExtensionCount(), "extension count must not increment on decline")
}

// TestRequestIterationExtension_NilParams 验证用户取消 (空响应) -> 不扩充.
func TestRequestIterationExtension_NilParams(t *testing.T) {
	ctx := context.Background()
	// 空响应: suggestion 为空 -> 不匹配任何固定选项 -> 视为拒绝.
	feedParams := aitool.InvokeParams{}
	cfg := newExtTestConfig(ctx, feedParams)
	loop, task := newExtTestLoop(t, cfg)

	agreed, delta, err := loop.requestIterationExtension(task, 11, 10)
	require.NoError(t, err)
	require.False(t, agreed)
	assert.Equal(t, 0, delta)
	assert.Equal(t, 0, loop.getIterationExtensionCount())
}

// TestRequestIterationExtension_ExtensionCapReached 验证已达扩充次数上限 -> 不再询问.
func TestRequestIterationExtension_ExtensionCapReached(t *testing.T) {
	ctx := context.Background()
	cfg := newExtTestConfig(ctx, aitool.InvokeParams{"suggestion": "+5"})
	loop, task := newExtTestLoop(t, cfg)
	// 预置已达上限
	for i := 0; i < maxIterationExtensionCount; i++ {
		loop.incrementIterationExtensionCount()
	}

	agreed, delta, err := loop.requestIterationExtension(task, 11, 10)
	require.NoError(t, err)
	require.False(t, agreed)
	assert.Equal(t, 0, delta)
	assert.False(t, cfg.fed, "Feed must not be called when extension cap reached")
}

// TestRequestIterationExtension_Plus10 验证用户选择 +10: delta=10.
func TestRequestIterationExtension_Plus10(t *testing.T) {
	ctx := context.Background()
	feedParams := aitool.InvokeParams{"suggestion": "+10"}
	cfg := newExtTestConfig(ctx, feedParams)
	loop, task := newExtTestLoop(t, cfg)

	agreed, delta, err := loop.requestIterationExtension(task, 11, 10)
	require.NoError(t, err)
	require.True(t, agreed)
	assert.Equal(t, 10, delta)
}

// TestRequestIterationExtension_Double 验证用户选择"翻倍": delta=maxIterations (10).
func TestRequestIterationExtension_Double(t *testing.T) {
	ctx := context.Background()
	feedParams := aitool.InvokeParams{"suggestion": "翻倍"}
	cfg := newExtTestConfig(ctx, feedParams)
	loop, task := newExtTestLoop(t, cfg)

	agreed, delta, err := loop.requestIterationExtension(task, 11, 10)
	require.NoError(t, err)
	require.True(t, agreed)
	assert.Equal(t, 10, delta, "翻倍 delta should equal maxIterations (10)")
}

// TestRequestIterationExtension_DoubleMinFloor 验证 maxIterations 较小时翻倍走 minDelta 下限.
func TestRequestIterationExtension_DoubleMinFloor(t *testing.T) {
	ctx := context.Background()
	feedParams := aitool.InvokeParams{"suggestion": "翻倍"}
	cfg := newExtTestConfig(ctx, feedParams)
	loop, task := newExtTestLoop(t, cfg)

	agreed, delta, err := loop.requestIterationExtension(task, 4, 3)
	require.NoError(t, err)
	require.True(t, agreed)
	assert.Equal(t, iterationExtensionMinDelta, delta, "翻倍 delta must floor to iterationExtensionMinDelta when maxIterations is small")
}

// TestRequestIterationExtension_ContextCancelled 验证任务上下文已取消时不阻塞.
func TestRequestIterationExtension_ContextCancelled(t *testing.T) {
	cfg := newExtTestConfig(context.Background(), aitool.InvokeParams{"suggestion": "+5"})
	loop, task := newExtTestLoop(t, cfg)
	// 把 task 的 ctx 替换为已取消的 ctx (newExtTestLoop 用 context.Background 构造 task)
	task.Cancel() // 取消 task 自身的 ctx

	done := make(chan struct{})
	go func() {
		agreed, _, _ := loop.requestIterationExtension(task, 11, 10)
		assert.False(t, agreed, "must not agree when task ctx cancelled")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("requestIterationExtension blocked on cancelled context")
	}
}
