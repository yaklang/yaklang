package reactloops

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

type perceptionMidtermSchedulerTestInvoker struct {
	*mockcfg.MockInvoker
	scheduledSummary  string
	scheduledTopics   []string
	scheduledKeywords []string
}

func (i *perceptionMidtermSchedulerTestInvoker) InvokeSpeedPriorityLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	_ = ctx
	_ = actionName
	_ = prompt
	_ = outputs
	_ = opts
	return aicommon.ExtractAction(`{
		"@action": "perception",
		"summary": "focused summary from perception",
		"topics": ["http fuzzing"],
		"keywords": ["header", "malformed"],
		"changed": true,
		"confidence": 0.92
	}`, "perception")
}

func (i *perceptionMidtermSchedulerTestInvoker) ScheduleMidtermTimelineRecallFromPerception(summary string, topics []string, keywords []string) {
	i.scheduledSummary = summary
	i.scheduledTopics = append([]string{}, topics...)
	i.scheduledKeywords = append([]string{}, keywords...)
}

type perceptionCapabilitySearchTestInvoker struct {
	*mockcfg.MockInvoker
	cfg aicommon.AICallerConfigIf
}

func (i *perceptionCapabilitySearchTestInvoker) GetConfig() aicommon.AICallerConfigIf {
	return i.cfg
}

func (i *perceptionCapabilitySearchTestInvoker) InvokeSpeedPriorityLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	_ = ctx
	_ = actionName
	_ = prompt
	_ = outputs
	_ = opts
	return aicommon.ExtractAction(`{
		"@action": "perception",
		"summary": "focused summary from perception",
		"topics": ["http fuzzing"],
		"keywords": ["header", "malformed"],
		"changed": true,
		"confidence": 0.92
	}`, "perception")
}

type perceptionKnowledgeSearchTestInvoker struct {
	*mockcfg.MockInvoker
	cfg                        aicommon.AICallerConfigIf
	selectedKnowledgeBase      []string
	quickSearchResult          string
	compressedResult           string
	lastQuickSearchQuery       string
	lastQuickSearchKeywords    []string
	lastQuickSearchCollections []string
	lastCompressTarget         int64
	selectCalls                int
}

func (i *perceptionKnowledgeSearchTestInvoker) GetConfig() aicommon.AICallerConfigIf {
	return i.cfg
}

func (i *perceptionKnowledgeSearchTestInvoker) InvokeSpeedPriorityLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	_ = ctx
	_ = actionName
	_ = prompt
	_ = outputs
	_ = opts
	return aicommon.ExtractAction(`{
		"@action": "perception",
		"summary": "focused summary from perception",
		"topics": ["http fuzzing"],
		"keywords": ["header", "malformed"],
		"changed": true,
		"confidence": 0.92
	}`, "perception")
}

func (i *perceptionKnowledgeSearchTestInvoker) SelectKnowledgeBase(ctx context.Context, originQuery string) (*aicommon.SelectedKnowledgeBaseResult, error) {
	_ = ctx
	_ = originQuery
	i.selectCalls++
	return aicommon.NewSelectedKnowledgeBaseResult("test selection", append([]string{}, i.selectedKnowledgeBase...)), nil
}

func (i *perceptionKnowledgeSearchTestInvoker) QuickKnowledgeSearch(ctx context.Context, userQuery string, keywords []string, collections ...string) (string, error) {
	_ = ctx
	i.lastQuickSearchQuery = userQuery
	i.lastQuickSearchKeywords = append([]string{}, keywords...)
	i.lastQuickSearchCollections = append([]string{}, collections...)
	return i.quickSearchResult, nil
}

func (i *perceptionKnowledgeSearchTestInvoker) CompressLongTextWithDestination(ctx context.Context, input any, destination string, targetByteSize int64) (string, error) {
	_ = ctx
	_ = input
	_ = destination
	i.lastCompressTarget = targetByteSize
	return i.compressedResult, nil
}

func TestTriggerPerception_SchedulesMidtermRecallSummary(t *testing.T) {
	invoker := &perceptionMidtermSchedulerTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}

	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.loopName = "perception-midterm-test"
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.maxIterations = 100
	loop.actionHistory = make([]*ActionRecord, 0)
	loop.actionHistoryMutex = new(sync.Mutex)

	state := loop.TriggerPerception(PerceptionTriggerForced, true)
	require.NotNil(t, state)
	require.Equal(t, "focused summary from perception", state.OneLinerSummary)
	require.Equal(t, "focused summary from perception", invoker.scheduledSummary)
	require.Equal(t, []string{"http fuzzing"}, invoker.scheduledTopics)
	require.Equal(t, []string{"header", "malformed"}, invoker.scheduledKeywords)
}

func TestTriggerPerception_AppliesCapabilitySearchResultsToLoop(t *testing.T) {
	tool, err := aitool.New(
		"perception_tool",
		aitool.WithDescription("tool discovered from perception"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "ok", nil
		}),
	)
	require.NoError(t, err)

	toolManager := buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return []*aitool.Tool{tool} },
		buildinaitools.WithExtendTools([]*aitool.Tool{tool}, true),
	)

	cfg := &aicommon.Config{
		Ctx:                    context.Background(),
		ContextProviderManager: aicommon.NewContextProviderManager(),
		AiToolManager:          toolManager,
	}
	invoker := &perceptionCapabilitySearchTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
		cfg:         cfg,
	}

	loop := NewMinimalReActLoop(cfg, invoker)
	loop.loopName = "perception-capability-search-test"
	loop.perception = newPerceptionController(perceptionDefaultIterationInterval)
	loop.extraCapabilities = NewExtraCapabilitiesManager()
	loop.maxIterations = 100
	loop.actionHistory = make([]*ActionRecord, 0)
	loop.actionHistoryMutex = new(sync.Mutex)

	originalSearcher := perceptionCapabilitySearcher
	perceptionCapabilitySearcher = func(r aicommon.AIInvokeRuntime, loop *ReActLoop, input CapabilitySearchInput) (*CapabilitySearchResult, error) {
		require.Equal(t, "focused summary from perception", input.Query)
		require.Contains(t, input.Queries, "http fuzzing")
		require.Contains(t, input.Queries, "header")
		require.Contains(t, input.Queries, "malformed")
		return &CapabilitySearchResult{
			SearchResultsMarkdown:   "### Matched Tools\n- perception_tool\n",
			ContextEnrichment:       "### Recommended Capabilities\n- perception_tool\n",
			MatchedToolNames:        []string{"perception_tool"},
			RecommendedCapabilities: []string{"perception_tool"},
		}, nil
	}
	defer func() {
		perceptionCapabilitySearcher = originalSearcher
	}()

	state := loop.TriggerPerception(PerceptionTriggerForced, true)
	require.NotNil(t, state)
	require.Equal(t, "perception_tool", loop.Get("perception_matched_tool_names"))
	require.Equal(t, "perception_tool", loop.Get("perception_recommended_capabilities"))
	require.Contains(t, loop.Get("perception_capability_context_enrichment"), "perception_tool")
	require.True(t, toolManager.IsRecentlyUsedTool("perception_tool"))

	rendered := loop.extraCapabilities.Render("nonce")
	require.Contains(t, rendered, "`perception_tool`")
}

func TestTriggerPerception_AppliesKnowledgeSearchResultsToLoop(t *testing.T) {
	cfg := &aicommon.Config{
		Ctx:                    context.Background(),
		ContextProviderManager: aicommon.NewContextProviderManager(),
	}
	invoker := &perceptionKnowledgeSearchTestInvoker{
		MockInvoker:           mockcfg.NewMockInvoker(context.Background()),
		cfg:                   cfg,
		selectedKnowledgeBase: []string{"security_kb"},
		quickSearchResult:     "raw knowledge result",
		compressedResult:      "compressed knowledge result",
	}

	loop := NewMinimalReActLoop(cfg, invoker)
	loop.loopName = "perception-knowledge-search-test"
	loop.perception = newPerceptionController(perceptionDefaultIterationInterval)
	loop.allowRAG = func() bool { return true }
	loop.maxIterations = 100
	loop.actionHistory = make([]*ActionRecord, 0)
	loop.actionHistoryMutex = new(sync.Mutex)
	loop.SetCurrentTask(aicommon.NewStatefulTaskBase("perception-knowledge-task", "help me fuzz this endpoint", context.Background(), cfg.GetEmitter(), true))
	loop.RegisterPerceptionContextProvider()

	originalKBLister := perceptionKnowledgeBaseNameLister
	perceptionKnowledgeBaseNameLister = func() ([]string, error) {
		return []string{"security_kb", "yaklang_docs"}, nil
	}
	defer func() {
		perceptionKnowledgeBaseNameLister = originalKBLister
	}()

	state := loop.TriggerPerception(PerceptionTriggerForced, true)
	require.NotNil(t, state)
	require.Equal(t, "security_kb,yaklang_docs", loop.Get("perception_selected_knowledge_bases"))
	require.Equal(t, "header malformed http fuzzing", loop.Get("perception_knowledge_query"))
	require.Contains(t, loop.Get("perception_knowledge_context"), "compressed knowledge result")
	require.Equal(t, int64(perceptionKnowledgeMaxContextTokens), invoker.lastCompressTarget)
	require.Contains(t, invoker.lastQuickSearchQuery, "focused summary from perception")
	require.Equal(t, []string{"header", "malformed", "http fuzzing"}, invoker.lastQuickSearchKeywords)
	require.Equal(t, []string{"security_kb", "yaklang_docs"}, invoker.lastQuickSearchCollections)
	require.Equal(t, 0, invoker.selectCalls)

	renderedDynamicContext := cfg.ContextProviderManager.Execute(cfg, cfg.GetEmitter())
	require.Contains(t, renderedDynamicContext, "## Perception Knowledge")
	require.Contains(t, renderedDynamicContext, "security_kb")
	require.Contains(t, renderedDynamicContext, "yaklang_docs")
	require.Contains(t, renderedDynamicContext, "compressed knowledge result")
}

func TestTriggerPerception_LimitsKnowledgeContextTo15K(t *testing.T) {
	oversizedKnowledge := strings.Repeat("knowledge ", perceptionKnowledgeMaxContextTokens+2048)

	cfg := &aicommon.Config{
		Ctx:                    context.Background(),
		ContextProviderManager: aicommon.NewContextProviderManager(),
	}
	invoker := &perceptionKnowledgeSearchTestInvoker{
		MockInvoker:           mockcfg.NewMockInvoker(context.Background()),
		cfg:                   cfg,
		selectedKnowledgeBase: []string{"security_kb"},
		quickSearchResult:     "raw knowledge result",
		compressedResult:      oversizedKnowledge,
	}

	loop := NewMinimalReActLoop(cfg, invoker)
	loop.loopName = "perception-knowledge-size-limit-test"
	loop.perception = newPerceptionController(perceptionDefaultIterationInterval)
	loop.allowRAG = func() bool { return true }
	loop.maxIterations = 100
	loop.actionHistory = make([]*ActionRecord, 0)
	loop.actionHistoryMutex = new(sync.Mutex)
	loop.SetCurrentTask(aicommon.NewStatefulTaskBase("perception-knowledge-limit-task", "help me fuzz this endpoint", context.Background(), cfg.GetEmitter(), true))

	originalKBLister := perceptionKnowledgeBaseNameLister
	perceptionKnowledgeBaseNameLister = func() ([]string, error) {
		return []string{"security_kb", "yaklang_docs"}, nil
	}
	defer func() {
		perceptionKnowledgeBaseNameLister = originalKBLister
	}()

	state := loop.TriggerPerception(PerceptionTriggerForced, true)
	require.NotNil(t, state)

	knowledgeContext := loop.Get("perception_knowledge_context")
	require.NotEmpty(t, knowledgeContext)
	require.LessOrEqual(t, aicommon.MeasureTokens(knowledgeContext), perceptionKnowledgeMaxContextTokens)
}

func TestHashTopics_DeterministicAndOrderIndependent(t *testing.T) {
	h1 := hashTopics([]string{"SQL Injection", "WAF Bypass"})
	h2 := hashTopics([]string{"WAF Bypass", "SQL Injection"})
	if h1 != h2 {
		t.Fatalf("hashTopics should be order-independent, got %s vs %s", h1, h2)
	}
	h3 := hashTopics([]string{"SQL Injection", "XSS"})
	if h1 == h3 {
		t.Fatalf("different topic sets should produce different hashes")
	}
}

func TestPerceptionState_ShouldUpdate_ForcedAlwaysTrue(t *testing.T) {
	prev := &PerceptionState{
		Topics:         []string{"SQL Injection"},
		PrevTopicsHash: hashTopics([]string{"SQL Injection"}),
	}

	newState := &PerceptionState{
		Topics:      []string{"SQL Injection"},
		Changed:     false,
		LastTrigger: PerceptionTriggerForced,
	}
	if !prev.ShouldUpdate(newState) {
		t.Fatal("forced trigger should always update")
	}

	newState.LastTrigger = PerceptionTriggerSpinDetected
	if !prev.ShouldUpdate(newState) {
		t.Fatal("spin_detected trigger should always update")
	}

	newState.LastTrigger = PerceptionTriggerLoopSwitch
	if !prev.ShouldUpdate(newState) {
		t.Fatal("loop_switch trigger should always update")
	}
}

func TestPerceptionState_ShouldUpdate_UnchangedSkips(t *testing.T) {
	prev := &PerceptionState{
		Topics:         []string{"SQL Injection"},
		PrevTopicsHash: hashTopics([]string{"SQL Injection"}),
	}

	newState := &PerceptionState{
		Topics:      []string{"SQL Injection"},
		Changed:     false,
		LastTrigger: PerceptionTriggerPostAction,
	}
	if prev.ShouldUpdate(newState) {
		t.Fatal("unchanged non-forced perception should not update")
	}
}

func TestPerceptionState_ShouldUpdate_ChangedWithNewTopics(t *testing.T) {
	prev := &PerceptionState{
		Topics:         []string{"SQL Injection"},
		PrevTopicsHash: hashTopics([]string{"SQL Injection"}),
	}

	newState := &PerceptionState{
		Topics:      []string{"SQL Injection", "WAF Bypass"},
		Changed:     true,
		LastTrigger: PerceptionTriggerPostAction,
	}
	if !prev.ShouldUpdate(newState) {
		t.Fatal("changed perception with new topics should update")
	}
}

func TestPerceptionState_ShouldUpdate_ChangedButSameTopicsHash(t *testing.T) {
	prev := &PerceptionState{
		Topics:         []string{"SQL Injection"},
		PrevTopicsHash: hashTopics([]string{"SQL Injection"}),
	}

	newState := &PerceptionState{
		Topics:      []string{"SQL Injection"},
		Changed:     true,
		LastTrigger: PerceptionTriggerPostAction,
	}
	if prev.ShouldUpdate(newState) {
		t.Fatal("changed=true but same topic hash should not update")
	}
}

func TestPerceptionState_ShouldUpdate_NilNewState(t *testing.T) {
	prev := &PerceptionState{Topics: []string{"test"}}
	if prev.ShouldUpdate(nil) {
		t.Fatal("nil new state should not trigger update")
	}
}

func TestPerceptionController_IntervalThrottling(t *testing.T) {
	pc := newPerceptionController(perceptionDefaultIterationInterval)

	state1 := &PerceptionState{
		Topics:      []string{"Topic A"},
		Changed:     true,
		LastTrigger: PerceptionTriggerPostAction,
	}
	pc.applyResult(state1)

	if pc.shouldSkipDueToInterval() {
		// just applied, interval has not elapsed; it should skip
	}
	if !pc.shouldSkipDueToInterval() {
		t.Fatal("immediately after apply, should skip due to interval")
	}
}

func TestPerceptionController_ExponentialBackoff(t *testing.T) {
	pc := newPerceptionController(perceptionDefaultIterationInterval)
	pc.currentInterval = 10 * time.Millisecond
	pc.minInterval = 10 * time.Millisecond
	pc.maxInterval = 100 * time.Millisecond

	unchanged := &PerceptionState{
		Topics:      []string{"Same"},
		Changed:     false,
		LastTrigger: PerceptionTriggerPostAction,
	}

	// First call initializes current (consecutiveUnchanged stays 0).
	// Second and third calls are "unchanged" increments.
	pc.applyResult(unchanged)
	pc.applyResult(unchanged)
	pc.applyResult(unchanged)
	if pc.consecutiveUnchanged < 2 {
		t.Fatalf("expected at least 2 consecutive unchanged, got %d", pc.consecutiveUnchanged)
	}
	if pc.currentInterval <= 10*time.Millisecond {
		t.Fatalf("interval should have doubled, got %v", pc.currentInterval)
	}
}

func TestPerceptionController_IntervalResetOnChange(t *testing.T) {
	pc := newPerceptionController(perceptionDefaultIterationInterval)
	pc.currentInterval = 10 * time.Millisecond
	pc.minInterval = 10 * time.Millisecond

	unchanged := &PerceptionState{
		Topics:      []string{"Same"},
		Changed:     false,
		LastTrigger: PerceptionTriggerPostAction,
	}
	pc.applyResult(unchanged)
	pc.applyResult(unchanged)
	pc.applyResult(unchanged)

	changed := &PerceptionState{
		Topics:      []string{"New Topic"},
		Changed:     true,
		LastTrigger: PerceptionTriggerForced,
	}
	pc.applyResult(changed)

	if pc.consecutiveUnchanged != 0 {
		t.Fatalf("change should reset consecutiveUnchanged, got %d", pc.consecutiveUnchanged)
	}
	if pc.currentInterval != pc.minInterval {
		t.Fatalf("change should reset interval to min, got %v", pc.currentInterval)
	}
}

func TestPerceptionController_ShouldTriggerOnIteration(t *testing.T) {
	pc := newPerceptionController(perceptionDefaultIterationInterval)
	pc.iterationTriggerInterval = 2

	if pc.shouldTriggerOnIteration(0) {
		t.Fatal("iteration 0 should not trigger")
	}
	if pc.shouldTriggerOnIteration(1) {
		t.Fatal("iteration 1 should not trigger with interval=2")
	}
	if !pc.shouldTriggerOnIteration(2) {
		t.Fatal("iteration 2 should trigger with interval=2")
	}
	if pc.shouldTriggerOnIteration(3) {
		t.Fatal("iteration 3 should not trigger with interval=2")
	}
	if !pc.shouldTriggerOnIteration(4) {
		t.Fatal("iteration 4 should trigger with interval=2")
	}
}

func TestPerceptionController_EpochIncrements(t *testing.T) {
	pc := newPerceptionController(perceptionDefaultIterationInterval)
	s1 := &PerceptionState{Topics: []string{"A"}, Changed: true, LastTrigger: PerceptionTriggerForced}
	pc.applyResult(s1)
	if pc.getCurrent().Epoch != 1 {
		t.Fatalf("expected epoch 1, got %d", pc.getCurrent().Epoch)
	}

	s2 := &PerceptionState{Topics: []string{"B"}, Changed: true, LastTrigger: PerceptionTriggerForced}
	pc.applyResult(s2)
	if pc.getCurrent().Epoch != 2 {
		t.Fatalf("expected epoch 2, got %d", pc.getCurrent().Epoch)
	}
}

func TestPerceptionState_FormatForContext(t *testing.T) {
	state := &PerceptionState{
		Topics:          []string{"SQL Injection", "WAF Bypass"},
		Keywords:        []string{"sqlmap", "union select"},
		OneLinerSummary: "Attempting SQL injection via UNION SELECT",
		Epoch:           3,
		LastUpdateAt:    time.Now().Add(-30 * time.Second),
	}
	output := state.FormatForContext()
	if !strings.Contains(output, "Current Perception") {
		t.Fatal("expected header in context output")
	}
	if !strings.Contains(output, "SQL Injection") {
		t.Fatal("expected topics in context output")
	}
	if !strings.Contains(output, "sqlmap") {
		t.Fatal("expected keywords in context output")
	}
	if !strings.Contains(output, "Attempting SQL injection") {
		t.Fatal("expected summary in context output")
	}
}

func TestPerceptionState_FormatForContext_Nil(t *testing.T) {
	var state *PerceptionState
	if state.FormatForContext() != "" {
		t.Fatal("nil state should produce empty string")
	}
}

func TestPerceptionState_FormatForContext_TokenLimit(t *testing.T) {
	topics := make([]string, 100)
	keywords := make([]string, 100)
	for i := range topics {
		topics[i] = strings.Repeat("topic_", 20)
		keywords[i] = strings.Repeat("keyword_", 20)
	}
	state := &PerceptionState{
		Topics:          topics,
		Keywords:        keywords,
		OneLinerSummary: strings.Repeat("very long summary ", 50),
		Epoch:           1,
		LastUpdateAt:    time.Now(),
	}
	output := state.FormatForContext()
	if output == "" {
		t.Fatal("expected non-empty output even when truncated")
	}
}

func TestMaybeTriggerPerceptionAfterAction_SyncPerceptionTriggerRunsInline(t *testing.T) {
	invoker := &perceptionMidtermSchedulerTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	cfg := invoker.GetConfig()
	cfg.SetConfig("SyncPerceptionTrigger", true)

	loop := NewMinimalReActLoop(cfg, invoker)
	loop.loopName = "perception-sync-trigger-test"
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.maxIterations = 100
	loop.actionHistory = make([]*ActionRecord, 0)
	loop.actionHistoryMutex = new(sync.Mutex)

	loop.MaybeTriggerPerceptionAfterAction(2)
	require.Equal(t, "focused summary from perception", invoker.scheduledSummary)
}

// TestPerceptionState_IsIntentPivot_ExplicitValues 验证三个枚举值的显式判定:
// 仅 IntentShiftPivot 返回 true, drift / none 返回 false. 不依赖 Changed.
//
// 关键词: TestPerceptionState_IsIntentPivot, IntentShift 枚举判定
func TestPerceptionState_IsIntentPivot_ExplicitValues(t *testing.T) {
	cases := []struct {
		name    string
		shift   string
		changed bool
		want    bool
	}{
		{"explicit pivot ignores changed=false", IntentShiftPivot, false, true},
		{"explicit pivot with changed=true", IntentShiftPivot, true, true},
		{"explicit drift ignores changed=true", IntentShiftDrift, true, false},
		{"explicit drift with changed=false", IntentShiftDrift, false, false},
		{"explicit none ignores changed=true", IntentShiftNone, true, false},
		{"explicit none with changed=false", IntentShiftNone, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &PerceptionState{IntentShift: tc.shift, Changed: tc.changed}
			require.Equal(t, tc.want, s.IsIntentPivot())
		})
	}
}

// TestPerceptionState_IsIntentPivot_BackwardCompat 验证 IntentShift 为空 (旧 AI / 旧 prompt
// 路径) 时回退到 Changed 的旧语义, 保证不破坏既有行为.
//
// 关键词: TestPerceptionState_IsIntentPivot 向后兼容回退 Changed
func TestPerceptionState_IsIntentPivot_BackwardCompat(t *testing.T) {
	require.True(t, (&PerceptionState{IntentShift: "", Changed: true}).IsIntentPivot(),
		"IntentShift 空 + Changed=true 应回退到 true (保持旧行为)")
	require.False(t, (&PerceptionState{IntentShift: "", Changed: false}).IsIntentPivot(),
		"IntentShift 空 + Changed=false 应回退到 false")

	// 大小写 / 空白也走归一化, 同样回退到 Changed 不会被未知字面量打断.
	require.True(t, (&PerceptionState{IntentShift: "  PIVOT  ", Changed: false}).IsIntentPivot(),
		"大写带空格的 PIVOT 也应识别为 pivot")
	require.False(t, (&PerceptionState{IntentShift: "Drift", Changed: true}).IsIntentPivot(),
		"大写首字母 Drift 应识别为 drift, 不再回退 Changed")

	// 完全未知字面量应当走 default 分支回退 Changed.
	require.True(t, (&PerceptionState{IntentShift: "unknown_value", Changed: true}).IsIntentPivot(),
		"未知字面量回退 Changed=true 仍触发 pivot")

	// nil 接收者必须安全返回 false.
	var nilState *PerceptionState
	require.False(t, nilState.IsIntentPivot())
}

// TestShouldRefreshDownstream_PivotOnly 验证下游门控核心规则: 非 forced trigger
// 下, 只有 IntentShift=pivot (或回退后 Changed=true) 才会放行.
//
// 关键词: TestShouldRefreshDownstream PivotOnly, drift/none/空 不触发下游
func TestShouldRefreshDownstream_PivotOnly(t *testing.T) {
	mk := func(shift string, changed bool) *PerceptionState {
		return &PerceptionState{
			IntentShift: shift,
			Changed:     changed,
			LastTrigger: PerceptionTriggerPostAction,
		}
	}

	require.True(t, mk(IntentShiftPivot, false).shouldRefreshDownstreamForState(true),
		"pivot + updated=true 应放行下游")
	require.False(t, mk(IntentShiftDrift, true).shouldRefreshDownstreamForState(true),
		"drift 即使 updated=true 也不放行下游")
	require.False(t, mk(IntentShiftNone, true).shouldRefreshDownstreamForState(true),
		"none 即使 updated=true 也不放行下游")
	require.False(t, mk("", false).shouldRefreshDownstreamForState(true),
		"IntentShift 空 + Changed=false (回退 false) 不放行")
	require.True(t, mk("", true).shouldRefreshDownstreamForState(true),
		"IntentShift 空 + Changed=true (回退 true) 放行, 保持旧行为")
}

// TestShouldRefreshDownstream_ForcedBypassesGate 验证 forced trigger 绕过 IntentShift 门控:
// 即使 AI 返回 drift / none, forced 仍然触发下游 (用户/系统显式请求).
//
// 关键词: TestShouldRefreshDownstream forced 绕门, PerceptionTriggerForced
func TestShouldRefreshDownstream_ForcedBypassesGate(t *testing.T) {
	mk := func(shift string) *PerceptionState {
		return &PerceptionState{
			IntentShift: shift,
			Changed:     false,
			LastTrigger: PerceptionTriggerForced,
		}
	}
	require.True(t, mk(IntentShiftDrift).shouldRefreshDownstreamForState(true),
		"forced + drift 必须绕门触发下游")
	require.True(t, mk(IntentShiftNone).shouldRefreshDownstreamForState(true),
		"forced + none 必须绕门触发下游")
	require.True(t, mk("").shouldRefreshDownstreamForState(true),
		"forced + 空 IntentShift 必须绕门触发下游")
}

// TestShouldRefreshDownstream_SpinAndLoopSwitchObeyGate 验证用户的明确选择:
// spin_detected / loop_switch 不再无条件触发下游, 仍受 IntentShift 门控约束.
//
// 关键词: TestShouldRefreshDownstream spin_detected loop_switch 受门控
func TestShouldRefreshDownstream_SpinAndLoopSwitchObeyGate(t *testing.T) {
	for _, trigger := range []string{PerceptionTriggerSpinDetected, PerceptionTriggerLoopSwitch} {
		t.Run(trigger+"/drift skipped", func(t *testing.T) {
			s := &PerceptionState{
				IntentShift: IntentShiftDrift,
				LastTrigger: trigger,
			}
			require.False(t, s.shouldRefreshDownstreamForState(true),
				"%s + drift 不应触发下游", trigger)
		})
		t.Run(trigger+"/pivot allowed", func(t *testing.T) {
			s := &PerceptionState{
				IntentShift: IntentShiftPivot,
				LastTrigger: trigger,
			}
			require.True(t, s.shouldRefreshDownstreamForState(true),
				"%s + pivot 应触发下游", trigger)
		})
	}
}

// TestShouldRefreshDownstream_RequiresUpdated 验证即使 trigger / IntentShift 都满足,
// updated=false (state 没真正覆盖) 也一律不触发下游 — 没有新 state 可推就不要骚扰下游.
//
// 关键词: TestShouldRefreshDownstream updated 必要条件
func TestShouldRefreshDownstream_RequiresUpdated(t *testing.T) {
	combos := []*PerceptionState{
		{IntentShift: IntentShiftPivot, LastTrigger: PerceptionTriggerForced, Changed: true},
		{IntentShift: IntentShiftPivot, LastTrigger: PerceptionTriggerPostAction, Changed: true},
		{IntentShift: "", LastTrigger: PerceptionTriggerForced, Changed: true},
	}
	for _, s := range combos {
		require.False(t, s.shouldRefreshDownstreamForState(false),
			"updated=false 时 (trigger=%s, shift=%q) 必须不触发下游", s.LastTrigger, s.IntentShift)
	}

	// nil 接收者也必须安全返回 false.
	var nilState *PerceptionState
	require.False(t, nilState.shouldRefreshDownstreamForState(true))
}
