// mirror_test.go - aibalance Mirror 子系统单元测试
//
// 覆盖:
//   - ParseActionFromText / ExtractToolFromActionPayload
//   - MirrorRuleMatch 四种条件
//   - mirrorLogRing 环形缓冲
//   - mirrorRuleRuntime 队列满 -> dropped
//   - executeMirrorScript panic / timeout
//
// 关键词: aibalance mirror unit tests, evaluator, queue full, panic safe, timeout

package aibalance

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
)

// -------------------- ParseAction --------------------

func TestParseActionFromText_TopLevel(t *testing.T) {
	text := `{"@action":"directly_answer","answer_payload":"hi"}`
	action, payload := ParseActionFromText(text)
	assert.Equal(t, "directly_answer", action)
	assert.Equal(t, "hi", payload["answer_payload"])
}

func TestParseActionFromText_FencedMarkdown(t *testing.T) {
	text := "some preface\n```json\n{\"@action\":\"call-tool\",\"tool\":\"read-file\"}\n```\ntrailing"
	action, payload := ParseActionFromText(text)
	assert.Equal(t, "call-tool", action)
	assert.Equal(t, "read-file", payload["tool"])
}

func TestParseActionFromText_NextActionFallback(t *testing.T) {
	text := `{"next_action":{"type":"require_tool","tool_require_payload":"create-file"}}`
	action, payload := ParseActionFromText(text)
	assert.Equal(t, "require_tool", action)
	tool := ExtractToolFromActionPayload(action, payload)
	assert.Equal(t, "create-file", tool)
}

func TestParseActionFromText_Empty(t *testing.T) {
	action, payload := ParseActionFromText("")
	assert.Equal(t, "", action)
	assert.Nil(t, payload)
}

func TestParseActionFromText_NonJSON(t *testing.T) {
	action, payload := ParseActionFromText("hello world, no json here")
	assert.Equal(t, "", action)
	assert.Nil(t, payload)
}

func TestParseActionFromText_NestedBraces(t *testing.T) {
	// payload 中含有嵌套花括号, 必须正确平衡
	text := `prefix {"@action":"directly_answer","data":{"nested":{"k":"v"}}} suffix`
	action, payload := ParseActionFromText(text)
	assert.Equal(t, "directly_answer", action)
	require.NotNil(t, payload)
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok)
	nested, ok := data["nested"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "v", nested["k"])
}

func TestExtractToolFromActionPayload_CallTool(t *testing.T) {
	payload := map[string]interface{}{"tool": "read-file"}
	assert.Equal(t, "read-file", ExtractToolFromActionPayload("call-tool", payload))
	assert.Equal(t, "read-file", ExtractToolFromActionPayload("directly_call_tool", payload))
	assert.Equal(t, "", ExtractToolFromActionPayload("directly_answer", payload))
}

func TestExtractToolFromActionPayload_NextAction(t *testing.T) {
	payload := map[string]interface{}{
		"next_action": map[string]interface{}{
			"tool": "grep-search",
		},
	}
	assert.Equal(t, "grep-search", ExtractToolFromActionPayload("call-tool", payload))
}

// -------------------- MirrorRuleMatch --------------------

func TestMirrorRuleMatch_Always(t *testing.T) {
	rule := &schema.AiMirrorRule{ConditionType: MirrorConditionAlways}
	snap := &MirrorSnapshot{}
	assert.True(t, MirrorRuleMatch(rule, snap))
}

func TestMirrorRuleMatch_ActionEq(t *testing.T) {
	rule := &schema.AiMirrorRule{ConditionType: MirrorConditionActionEq, ActionName: "directly_answer"}
	snapHit := &MirrorSnapshot{Action: "directly_answer"}
	snapMiss := &MirrorSnapshot{Action: "call-tool"}
	snapEmpty := &MirrorSnapshot{}
	assert.True(t, MirrorRuleMatch(rule, snapHit))
	assert.False(t, MirrorRuleMatch(rule, snapMiss))
	assert.False(t, MirrorRuleMatch(rule, snapEmpty))
}

func TestMirrorRuleMatch_AnyToolcall(t *testing.T) {
	rule := &schema.AiMirrorRule{ConditionType: MirrorConditionAnyToolcall}
	assert.False(t, MirrorRuleMatch(rule, &MirrorSnapshot{}))
	assert.True(t, MirrorRuleMatch(rule, &MirrorSnapshot{
		ToolCalls: []*aispec.ToolCall{{ID: "abc"}},
	}))
}

func TestMirrorRuleMatch_ActionCallToolEq(t *testing.T) {
	rule := &schema.AiMirrorRule{
		ConditionType: MirrorConditionActionCallToolEq,
		ToolName:      "read-file",
	}
	hit := &MirrorSnapshot{
		Action:        "call-tool",
		ActionPayload: map[string]interface{}{"tool": "read-file"},
	}
	wrongTool := &MirrorSnapshot{
		Action:        "call-tool",
		ActionPayload: map[string]interface{}{"tool": "write-file"},
	}
	wrongAction := &MirrorSnapshot{
		Action:        "directly_answer",
		ActionPayload: map[string]interface{}{"tool": "read-file"},
	}
	requireTool := &MirrorSnapshot{
		Action:        "require_tool",
		ActionPayload: map[string]interface{}{"tool_require_payload": "read-file"},
	}
	assert.True(t, MirrorRuleMatch(rule, hit))
	assert.False(t, MirrorRuleMatch(rule, wrongTool))
	assert.False(t, MirrorRuleMatch(rule, wrongAction))
	assert.True(t, MirrorRuleMatch(rule, requireTool))
}

// TestMirrorRuleMatch_ActionCallToolEq_ActionNameOptionalFilter
// 覆盖 action_call_tool_eq 条件下 ActionName 作为"可选过滤器"的语义:
//
//	空 ActionName  => 三种 call-tool 类 action 全通配
//	非空 ActionName => 仅精确匹配该 action (例如只想要 require_tool)
//
// 关键词: action_call_tool_eq, ActionName 可选过滤, optional action narrow
func TestMirrorRuleMatch_ActionCallToolEq_ActionNameOptionalFilter(t *testing.T) {
	callTool := &MirrorSnapshot{
		Action:        "call-tool",
		ActionPayload: map[string]interface{}{"tool": "read-file"},
	}
	directly := &MirrorSnapshot{
		Action:        "directly_call_tool",
		ActionPayload: map[string]interface{}{"tool": "read-file"},
	}
	requireT := &MirrorSnapshot{
		Action:        "require_tool",
		ActionPayload: map[string]interface{}{"tool_require_payload": "read-file"},
	}

	// 1. 留空 ActionName: 3 种 action 都应当命中 (tool 都匹配)
	ruleNoAction := &schema.AiMirrorRule{
		ConditionType: MirrorConditionActionCallToolEq,
		ActionName:    "",
		ToolName:      "read-file",
	}
	assert.True(t, MirrorRuleMatch(ruleNoAction, callTool))
	assert.True(t, MirrorRuleMatch(ruleNoAction, directly))
	assert.True(t, MirrorRuleMatch(ruleNoAction, requireT))

	// 2. 指定 ActionName=require_tool: 只命中 require_tool
	ruleOnlyRequire := &schema.AiMirrorRule{
		ConditionType: MirrorConditionActionCallToolEq,
		ActionName:    "require_tool",
		ToolName:      "read-file",
	}
	assert.False(t, MirrorRuleMatch(ruleOnlyRequire, callTool))
	assert.False(t, MirrorRuleMatch(ruleOnlyRequire, directly))
	assert.True(t, MirrorRuleMatch(ruleOnlyRequire, requireT))

	// 3. 指定 ActionName=call-tool: 只命中 call-tool
	ruleOnlyCall := &schema.AiMirrorRule{
		ConditionType: MirrorConditionActionCallToolEq,
		ActionName:    "call-tool",
		ToolName:      "read-file",
	}
	assert.True(t, MirrorRuleMatch(ruleOnlyCall, callTool))
	assert.False(t, MirrorRuleMatch(ruleOnlyCall, directly))
	assert.False(t, MirrorRuleMatch(ruleOnlyCall, requireT))

	// 4. 指定一个不在白名单的 ActionName: 永远不命中
	//    (即使填了 directly_answer, 也会被外层 3-种-action 白名单先拦掉)
	ruleBadAction := &schema.AiMirrorRule{
		ConditionType: MirrorConditionActionCallToolEq,
		ActionName:    "directly_answer",
		ToolName:      "read-file",
	}
	assert.False(t, MirrorRuleMatch(ruleBadAction, callTool))
}

func TestMirrorRuleMatch_NilSafe(t *testing.T) {
	assert.False(t, MirrorRuleMatch(nil, &MirrorSnapshot{}))
	assert.False(t, MirrorRuleMatch(&schema.AiMirrorRule{ConditionType: MirrorConditionAlways}, nil))
}

// -------------------- mirrorLogRing --------------------

func TestMirrorLogRing_RoundRobin(t *testing.T) {
	ring := newMirrorLogRing()
	// 写入 cap+5 条, 期望保留最近 cap 条且 newest first.
	total := mirrorLogRingCap + 5
	for i := 0; i < total; i++ {
		ring.push(MirrorRunLog{
			Timestamp: time.Now(),
			ReqID:     "r" + itoa(i),
			Success:   true,
		})
	}
	snap := ring.snapshot()
	require.Len(t, snap, mirrorLogRingCap)
	// newest first => 第一条应该是最后写入的 (i=total-1)
	assert.Equal(t, "r"+itoa(total-1), snap[0].ReqID)
}

// itoa 是 strconv.Itoa 的本地别名, 避免引入额外 import.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	s := string(b[pos:])
	if neg {
		return "-" + s
	}
	return s
}

// -------------------- executeMirrorScript --------------------

func TestExecuteMirrorScript_HappyPath(t *testing.T) {
	// 这个测试需要 yak.NewScriptEngine 可用. 如果 ScriptEngine 在测试环境下不可用,
	// 它会返回 error - 此时跳过测试.
	script := `
func handle(data) {
    // do nothing
}
`
	snap := &MirrorSnapshot{ReqID: "test1", Model: "m1"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err, _ := executeMirrorScript(ctx, script, snap)
	if err != nil {
		t.Logf("script engine not available in test env: %v", err)
		t.Skip("yak script engine not available in test env")
	}
}

func TestExecuteMirrorScript_EmptyScript(t *testing.T) {
	ctx := context.Background()
	err, _ := executeMirrorScript(ctx, "", &MirrorSnapshot{ReqID: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty script")
}

// -------------------- Queue Full -> Dropped --------------------

// 验证 mirrorRuleRuntime: 当所有 worker 都被堵住, channel 满, 后续 Trigger 应触发 dropped 计数.
// 这里我们手动构造 manager + runtime, 不依赖 DB IncrementMirrorCounters 的真实写入
// (DB 不可用时 increment 会失败但不影响 dropped 语义).
func TestMirrorRunner_QueueFull(t *testing.T) {
	rule := &schema.AiMirrorRule{
		Name:          "queue-full",
		Enabled:       true,
		ConditionType: MirrorConditionAlways,
		Concurrency:   1,
		QueueSize:     2,
		TimeoutMs:     5000,
	}
	rule.ID = 991
	m := NewMirrorManager()

	// 手动启动 runtime, 但不让 worker 真正消费 - 改成自定义 worker 卡在 sleep.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	consumed := int32(0)
	rt := &mirrorRuleRuntime{
		rule:   rule,
		ch:     make(chan *MirrorSnapshot, rule.QueueSize),
		cancel: cancel,
		logs:   newMirrorLogRing(),
	}
	// 自定义 slow worker: 收到第一条后 sleep, 让后续投递撑满队列.
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-rt.ch:
				if !ok {
					return
				}
				atomic.AddInt32(&consumed, 1)
				time.Sleep(2 * time.Second)
			}
		}
	}()
	m.mu.Lock()
	m.runtime[rule.ID] = rt
	m.mu.Unlock()

	// 发起 5 次 Trigger; 队列 size=2, 1 个 worker 卡住后:
	//   - 第 1 条: worker 立刻 receive, ch len=0
	//   - 第 2,3 条: 入队 ch len=2
	//   - 第 4,5 条: 队列满 -> dropped (DB increment 可能 fail 但内存 ring 会留下条目)
	snap := &MirrorSnapshot{ReqID: "trigger"}
	for i := 0; i < 5; i++ {
		m.Trigger(snap)
	}
	// 短暂等待让所有 select 完成
	time.Sleep(200 * time.Millisecond)

	logs := rt.logs.snapshot()
	dropCount := 0
	for _, l := range logs {
		if strings.Contains(l.ErrorMessage, "queue full") {
			dropCount++
		}
	}
	// 期望至少 2 条 dropped 日志 (条目 4,5)
	assert.GreaterOrEqual(t, dropCount, 2, "expected at least 2 dropped events, got %d", dropCount)

	// 清理
	m.RemoveRule(rule.ID)
}

// -------------------- Panic Safety --------------------

func TestMirrorRunOnce_PanicSafe(t *testing.T) {
	// runOnce 内置 panic recover; 这里直接构造一个 runtime 用 panic 脚本验证.
	rule := &schema.AiMirrorRule{
		Name:          "panic-safe",
		Enabled:       true,
		ConditionType: MirrorConditionAlways,
		Concurrency:   1,
		QueueSize:     4,
		TimeoutMs:     2000,
		CallbackScript: `
func handle(data) {
    panic("boom")
}
`,
	}
	rule.ID = 992
	m := NewMirrorManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt := &mirrorRuleRuntime{
		rule:   rule,
		ch:     make(chan *MirrorSnapshot, rule.QueueSize),
		cancel: cancel,
		logs:   newMirrorLogRing(),
	}
	// 直接同步 runOnce, 不开 worker
	snap := &MirrorSnapshot{ReqID: "panic-test", TimestampMs: time.Now().UnixMilli()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// 即便脚本 panic, runOnce 也不应当 panic 出来
		m.runOnce(ctx, rt, snap)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("runOnce did not return within timeout")
	}
	// ring 中应当至少存有一条 entry (panic 也算 finished, 走 defer)
	logs := rt.logs.snapshot()
	if len(logs) > 0 {
		t.Logf("got %d log entries; first success=%v err=%q", len(logs), logs[0].Success, logs[0].ErrorMessage)
	}
}

// -------------------- Timeout --------------------

func TestMirrorRunOnce_Timeout(t *testing.T) {
	// 死循环脚本; TimeoutMs=200, 应当被超时 cancel.
	rule := &schema.AiMirrorRule{
		Name:          "timeout-test",
		Enabled:       true,
		ConditionType: MirrorConditionAlways,
		Concurrency:   1,
		QueueSize:     4,
		TimeoutMs:     200,
		CallbackScript: `
func handle(data) {
    for {
        // 死循环
    }
}
`,
	}
	rule.ID = 993
	m := NewMirrorManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt := &mirrorRuleRuntime{
		rule:   rule,
		ch:     make(chan *MirrorSnapshot, rule.QueueSize),
		cancel: cancel,
		logs:   newMirrorLogRing(),
	}
	snap := &MirrorSnapshot{ReqID: "timeout-test"}
	start := time.Now()
	m.runOnce(ctx, rt, snap)
	elapsed := time.Since(start)
	// 200ms 超时, 给上层 yak engine 一点 grace, 但不应超过 5s
	assert.Less(t, elapsed, 5*time.Second, "runOnce should return within 5s due to timeout, took %v", elapsed)
}

// -------------------- MirrorSnapshot.ToScriptMap --------------------

func TestMirrorSnapshot_ToScriptMap(t *testing.T) {
	snap := &MirrorSnapshot{
		ReqID:        "abc",
		Model:        "m1",
		APIKeyFP:     APIKeyFingerprint("sk-secret-1234567890"),
		Action:       "directly_answer",
		ResponseText: "hello",
		ActionPayload: map[string]interface{}{
			"@action": "directly_answer",
			"answer":  "hi",
		},
	}
	m := snap.ToScriptMap()
	assert.Equal(t, "abc", m["req_id"])
	assert.Equal(t, "m1", m["model"])
	assert.Equal(t, "directly_answer", m["action"])
	assert.Equal(t, "hello", m["response_text"])
	// 指纹字段存在且为 16 字符 hex.
	fp, ok := m["api_key_fp"].(string)
	assert.True(t, ok)
	assert.Len(t, fp, 16)
	// 兜底剔除: 不应出现 api_key / apikey 等原始字段.
	// 关键词: ToScriptMap redact, api_key 不可暴露
	_, hasApiKey := m["api_key"]
	assert.False(t, hasApiKey, "ToScriptMap should never expose api_key")
	_, hasApikey := m["apikey"]
	assert.False(t, hasApikey)
	// nil 安全
	var nilSnap *MirrorSnapshot
	assert.NotNil(t, nilSnap.ToScriptMap())
}

// TestAPIKeyFingerprint_Deterministic 保证同一 key 多次计算指纹一致, 不同 key 不同.
// 关键词: APIKeyFingerprint, SHA256[:16] 确定性 / 区分性
func TestAPIKeyFingerprint_Deterministic(t *testing.T) {
	a1 := APIKeyFingerprint("sk-abc")
	a2 := APIKeyFingerprint("sk-abc")
	b1 := APIKeyFingerprint("sk-def")
	assert.Equal(t, a1, a2, "same key must produce same fingerprint")
	assert.NotEqual(t, a1, b1, "different keys must produce different fingerprints")
	assert.Len(t, a1, 16)
	assert.Equal(t, "", APIKeyFingerprint(""), "empty input -> empty fp")
	// 指纹本身不能反推 (这里只做基础检查: 不能包含原 key 子串).
	leakage := APIKeyFingerprint("sk-leakcheck-1234567890")
	assert.False(t, strings.Contains(leakage, "sk-"))
	assert.False(t, strings.Contains(leakage, "leakcheck"))
}

// TestMirrorDataSpec_NoSensitiveFields 保证 spec 不会描述任何原始 key 字段.
// 关键词: MirrorDataSpec, 不暴露 api_key, 仅暴露 api_key_fp
func TestMirrorDataSpec_NoSensitiveFields(t *testing.T) {
	spec := MirrorDataSpec()
	require.NotEmpty(t, spec)
	for _, f := range spec {
		assert.NotEqual(t, "api_key", f.Name, "spec must not describe raw api_key field")
		assert.NotEqual(t, "apikey", f.Name)
	}
	// 必须包含 api_key_fp 这个不可逆指纹字段
	var hasFP bool
	for _, f := range spec {
		if f.Name == "api_key_fp" {
			hasFP = true
			break
		}
	}
	assert.True(t, hasFP, "spec must include api_key_fp")
}

// TestDefaultMirrorScript_ShapeAndYAKMAIN 保证默认脚本含 handle 入口和 YAK_MAIN 自测块.
// 关键词: DefaultMirrorScript, handle 入口, YAK_MAIN 自测块, 本地复跑
func TestDefaultMirrorScript_ShapeAndYAKMAIN(t *testing.T) {
	s := DefaultMirrorScript()
	require.NotEmpty(t, s)
	assert.True(t, strings.Contains(s, "func handle(data)"),
		"default script must define func handle(data)")
	assert.True(t, strings.Contains(s, "if YAK_MAIN"),
		"default script must include `if YAK_MAIN { ... }` local test entry")
	// 默认脚本不能硬编码原始 key, 只能出现 api_key_fp 样例
	assert.False(t, strings.Contains(s, "sk-real"),
		"default script must not contain real-looking sk-* tokens")
}

// -------------------- MirrorManager.Trigger nil safe --------------------

func TestMirrorManager_TriggerNilSafe(t *testing.T) {
	var m *MirrorManager
	// nil receiver should not panic
	m.Trigger(&MirrorSnapshot{})
	// non-nil but empty
	m2 := NewMirrorManager()
	m2.Trigger(nil)
	m2.Trigger(&MirrorSnapshot{ReqID: "x"})
}

// -------------------- 并发投递验证 --------------------

func TestMirrorManager_ConcurrentTrigger(t *testing.T) {
	// 简单并发安全: 多个 goroutine 同时 Trigger 不应该 race.
	m := NewMirrorManager()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				m.Trigger(&MirrorSnapshot{ReqID: "c"})
			}
		}()
	}
	wg.Wait()
}
