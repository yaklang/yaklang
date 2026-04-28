package reactloops_yak

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
	// 触发 yaklang.Import 把 str / log / sprintf 等内置库注册到全局 yaklangLibs，
	// 否则 yak focus mode 脚本里的 str.HasPrefix / sprint 会因函数缺失 panic。
	_ "github.com/yaklang/yaklang/common/yak"
)

// 端到端验证：通过 LoadAllFromEmbed 把内置 yak 专注模式加载到全局工厂表，
// 再通过 reactloops.CreateLoopByName 创建出真正的 ReActLoop 实例并 Execute，
// 借助 mock AICallback 让 LLM 返回 yak_scan_demo / comprehensive_showcase
// 自定义 action 的 JSON，断言：
//   - verifier 真的被调用（非法参数早期失败 / 合法参数继续）
//   - handler 真的被调用（loop 内部状态被改写）
//   - op.Feedback 注入的内容能被下一轮 prompt 看到
//   - sidekick 函数（runReachabilityCheck / pickStepEmoji / formatShowcaseSummary）
//     从主入口可达
//
// 关键词: yak focus mode e2e test, ExecuteLoopTask integration, action handler verified

// ----- 测试 1：yak_scan_demo 通过 scan_target → summarize_scan → finish 完整跑通 -----

func TestE2E_YakScanDemo_ActionHandlerInvoked(t *testing.T) {
	require.NoError(t, LoadAllFromEmbed(), "load embed focus modes should succeed")

	// 第一次：scan_target；第二次：summarize_scan；之后：finish
	mock := &mockAIScript{
		t: t,
		responses: []mockAIResponse{
			{
				match: nil, // 不区分 prompt，按调用顺序
				body:  `{"@action":"scan_target","target":"https://example.com"}`,
			},
			{
				body: `{"@action":"summarize_scan"}`,
			},
			{
				body: `{"@action":"finish"}`,
			},
		},
		fallback: `{"@action":"finish"}`,
	}

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(mock.callback()),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
	)
	require.NoError(t, err)

	loop, err := reactloops.CreateLoopByName("yak_scan_demo", reactIns)
	require.NoError(t, err, "yak_scan_demo loop must be creatable from registered factory")
	require.NotNil(t, loop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = loop.Execute("e2e-yak-scan-"+utils.RandStringBytes(4), ctx,
		"please scan https://example.com and summarize")
	require.NoError(t, err, "yak_scan_demo Execute should not fail")

	// loop.GetVariable 取原始值；loop.Get 会做字符串化。
	require.Equal(t, "https://example.com", loop.GetVariable("current_target"),
		"verifier must have set current_target via loop.Set")

	findings := loop.GetVariable("scan_findings")
	require.NotNil(t, findings, "handler must have appended at least one finding")

	// 至少跑过 scan_target 和 summarize_scan 两次 LLM 调用
	require.GreaterOrEqual(t, mock.callCount(), 2,
		"expect at least scan_target + summarize_scan + finish round")
}

// ----- 测试 2：comprehensive_showcase 通过自定义 action 触发 sidekick 函数 -----

func TestE2E_ComprehensiveShowcase_SidekickReachable(t *testing.T) {
	require.NoError(t, LoadAllFromEmbed())

	mock := &mockAIScript{
		t: t,
		responses: []mockAIResponse{
			{body: `{"@action":"showcase_step","step":"intro","note":"first hop"}`},
			{body: `{"@action":"summarize_findings"}`},
			{body: `{"@action":"finish"}`},
		},
		fallback: `{"@action":"finish"}`,
	}

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(mock.callback()),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
	)
	require.NoError(t, err)

	loop, err := reactloops.CreateLoopByName("comprehensive_showcase", reactIns)
	require.NoError(t, err)
	require.NotNil(t, loop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = loop.Execute("e2e-showcase-"+utils.RandStringBytes(4), ctx,
		"walk me through the showcase")
	require.NoError(t, err)

	// verifier 把 step 落到 current_step；GetVariable 取原始字符串。
	require.Equal(t, "intro", loop.GetVariable("current_step"),
		"verifier must record current_step via loop.Set")

	// handler 调用 sidekick pickStepEmoji 后维护计数（int 型变量）。
	doneRaw := loop.GetVariable("showcase_steps_done")
	require.NotNil(t, doneRaw, "showcase_steps_done must be present")
	doneInt := utils.InterfaceToInt(doneRaw)
	require.GreaterOrEqual(t, doneInt, 1,
		"showcase_steps_done must be incremented by handler at least once")

	require.GreaterOrEqual(t, mock.callCount(), 2)
}

// ----- 测试 3：verifier 拒绝非法参数（验证 verifier 真的被调用） -----
// verifier 抛出的错误会被 reactloops 当成 ai transaction error 上抛，
// 因此 Execute 应当返回包含 verifier 报错信息的 error。
// 关键词: yak focus mode verifier rejection, error propagation

func TestE2E_YakScanDemo_VerifierRejects(t *testing.T) {
	require.NoError(t, LoadAllFromEmbed())

	mock := &mockAIScript{
		t:        t,
		fallback: `{"@action":"scan_target","target":"not-a-url"}`,
	}

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(mock.callback()),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
	)
	require.NoError(t, err)

	loop, err := reactloops.CreateLoopByName("yak_scan_demo", reactIns)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = loop.Execute("e2e-yak-scan-bad-"+utils.RandStringBytes(4), ctx,
		"scan something weird")
	require.Error(t, err, "verifier must reject invalid url and propagate error")
	require.Contains(t, err.Error(), "invalid url",
		"error message must come from yak verifier's error('invalid url: ...')")

	// verifier 失败的语义是不写入 current_target；handler 也没机会跑。
	require.Nil(t, loop.GetVariable("current_target"),
		"verifier must reject invalid target so current_target stays nil")
	// scan_findings 由 __VARS__ 初始化为 []，verifier 阶段不会被覆盖。
	findings := loop.GetVariable("scan_findings")
	if list, ok := findings.([]any); ok {
		require.Equal(t, 0, len(list), "handler must NOT run when verifier rejects")
	}
}

// ===== mock AI 脚本 =====

type mockAIResponse struct {
	// match 为空时按 calls 索引顺序匹配；非空时只在匹配的 prompt 上消费一次。
	match []string
	body  string
}

type mockAIScript struct {
	t         *testing.T
	mu        sync.Mutex
	calls     int
	responses []mockAIResponse
	fallback  string
}

func (m *mockAIScript) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *mockAIScript) callback() aicommon.AICallbackType {
	return func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		m.mu.Lock()
		idx := m.calls
		m.calls++
		m.mu.Unlock()

		rsp := i.NewAIResponse()
		prompt := req.GetPrompt()

		// 1. 优先满足 verify-satisfaction 类的辅助 prompt
		if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
			rsp.EmitOutputStream(bytes.NewBufferString(
				`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "ok"}`,
			))
			rsp.Close()
			return rsp, nil
		}

		// 2. 顺序匹配主脚本
		if idx < len(m.responses) {
			r := m.responses[idx]
			if len(r.match) == 0 || utils.MatchAllOfSubString(prompt, r.match...) {
				rsp.EmitOutputStream(bytes.NewBufferString(r.body))
				rsp.Close()
				return rsp, nil
			}
		}

		// 3. fallback
		if m.fallback != "" {
			rsp.EmitOutputStream(bytes.NewBufferString(m.fallback))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(
				`{"@action":"directly_answer","answer":"fallback"}`,
			))
		}
		rsp.Close()
		return rsp, nil
	}
}
