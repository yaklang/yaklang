package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func boolPointer(value bool) *bool { return &value }

func TestStatefulDriverKeepsConversationRuntimeAfterTurnFailure(t *testing.T) {
	engine := newFakeStatelessTurnEngine()
	engine.sendErr = errors.New("provider failed")
	emitter := newBlockingTurnFailureEmitter()
	handle := &yakAIEngineRuntimeHandle{
		engine:       engine,
		emitter:      emitter,
		messageQueue: make(chan yakAIQueuedMessage, 1),
	}

	go handle.sendMessage(yakAIQueuedMessage{turnID: "turn-failed", content: "first"})
	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("failing turn did not start")
	}
	close(engine.release)
	select {
	case <-emitter.started:
	case <-time.After(time.Second):
		t.Fatal("terminal failure publication did not start")
	}

	close(emitter.release)
	deadline := time.After(time.Second)
	for {
		handle.mu.Lock()
		active := handle.currentTurn
		handle.mu.Unlock()
		if active == "" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("stateful failed turn did not finish")
		case <-time.After(time.Millisecond):
		}
	}
	if handle.closed {
		t.Fatal("stateful conversation runtime was closed by a per-turn failure")
	}
}

func TestStatefulTurnWaitsForReActQueueDrainBeforeCompleting(t *testing.T) {
	engine := newFakeStatelessTurnEngine()
	engine.drainStarted = make(chan struct{})
	engine.drainRelease = make(chan struct{})
	emitter := recordingConversationTurnEmitter{
		completed: make(chan conversationTurnResult, 1),
		failed:    make(chan conversationTurnResult, 1),
	}
	handle := &yakAIEngineRuntimeHandle{
		engine:       engine,
		emitter:      emitter,
		messageQueue: make(chan yakAIQueuedMessage, 1),
	}

	go handle.sendMessage(yakAIQueuedMessage{turnID: "turn-with-queued-follow-up", content: "first"})
	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("root task did not start")
	}
	close(engine.release)
	select {
	case <-engine.drainStarted:
	case <-time.After(time.Second):
		t.Fatal("stateful runtime did not wait for the ReAct queue to drain")
	}
	select {
	case completed := <-emitter.completed:
		t.Fatalf("turn completed before queued follow-up drained: %#v", completed)
	case <-time.After(50 * time.Millisecond):
	}

	close(engine.drainRelease)
	select {
	case completed := <-emitter.completed:
		if completed.turnID != "turn-with-queued-follow-up" {
			t.Fatalf("completed turn = %q", completed.turnID)
		}
	case <-time.After(time.Second):
		t.Fatal("turn did not complete after queued follow-up drained")
	}
}

func TestStatefulControlInputIsLinearizedWithTerminalClose(t *testing.T) {
	engine := newFakeStatelessTurnEngine()
	engine.eventStarted = make(chan struct{})
	engine.eventRelease = make(chan struct{})
	handle := &yakAIEngineRuntimeHandle{
		engine:       engine,
		emitter:      noopEmitter{},
		messageQueue: make(chan yakAIQueuedMessage, 1),
	}
	inputDone := make(chan error, 1)
	go func() {
		inputDone <- handle.SendInput(context.Background(), aiSessionInput{
			Ref:         aiSessionCommandRef{CommandID: "hotpatch-before-close"},
			InputType:   "hotpatch",
			PayloadJSON: []byte(`{"hotpatch_type":"EnablePlan","params":{"enable_plan":true}}`),
		})
	}()
	select {
	case <-engine.eventStarted:
	case <-time.After(time.Second):
		t.Fatal("control input did not reach engine")
	}
	closeDone := make(chan struct{})
	go func() {
		handle.closeForTerminalFailure()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("terminal close crossed an in-flight control delivery")
	case <-time.After(20 * time.Millisecond):
	}
	close(engine.eventRelease)
	if err := <-inputDone; err != nil {
		t.Fatalf("linearized pre-close control input: %v", err)
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("terminal close did not complete after control delivery")
	}
	if err := handle.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "hotpatch-after-close"},
		InputType:   "hotpatch",
		PayloadJSON: []byte(`{"hotpatch_type":"EnablePlan","params":{"enable_plan":true}}`),
	}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("post-close hotpatch error = %v, want closed runtime", err)
	}
}

func TestStatefulHotpatchUpdatesSnapshotUsedByFirstDirectForge(t *testing.T) {
	engine := newFakeStatelessTurnEngine()
	handle := &yakAIEngineRuntimeHandle{
		engine:            engine,
		emitter:           noopEmitter{},
		binding:           aiSessionBinding{RuntimeOptionSnapshotJSON: []byte(`{"forge_name":"forge-a","review_policy":"yolo"}`)},
		runtime:           yakRuntimeOptions{ForgeName: "forge-a", ReviewPolicy: "yolo"},
		directForgeConfig: aiengine.NewAIEngineConfig(aiengine.WithReviewPolicy("yolo")),
		messageQueue:      make(chan yakAIQueuedMessage, 1),
	}
	if err := handle.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "hotpatch-before-forge"},
		InputType:   "hotpatch",
		PayloadJSON: []byte(`{"hotpatch_type":"AgreePolicy","params":{"review_policy":"manual"}}`),
	}); err != nil {
		t.Fatalf("hotpatch before first Forge: %v", err)
	}
	ok, runtime, binding, config := handle.claimDirectForge()
	if !ok {
		t.Fatal("expected first message to claim configured direct Forge")
	}
	if runtime.ReviewPolicy != "manual" || config == nil || config.ReviewPolicy != "manual" {
		t.Fatalf("direct Forge snapshot did not include hotpatch: runtime=%q config=%#v", runtime.ReviewPolicy, config)
	}
	var persisted yakRuntimeOptions
	if err := json.Unmarshal(binding.RuntimeOptionSnapshotJSON, &persisted); err != nil {
		t.Fatalf("decode persisted runtime snapshot: %v", err)
	}
	if persisted.ReviewPolicy != "manual" {
		t.Fatalf("persisted runtime review policy = %q, want manual", persisted.ReviewPolicy)
	}
	select {
	case event := <-engine.events:
		if !event.GetIsConfigHotpatch() {
			t.Fatalf("long-lived engine received non-hotpatch event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("long-lived engine did not receive hotpatch")
	}
}

func TestStatefulHotpatchDuringDirectForgeUpdatesLiveAndNextSnapshots(t *testing.T) {
	engine := newFakeStatelessTurnEngine()
	forgeInput := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 1)
	handle := &yakAIEngineRuntimeHandle{
		engine:            engine,
		emitter:           noopEmitter{},
		binding:           aiSessionBinding{RuntimeOptionSnapshotJSON: []byte(`{"forge_name":"forge-a","enable_plan":false}`)},
		runtime:           yakRuntimeOptions{ForgeName: "forge-a", EnablePlan: boolPointer(false)},
		directForgeConfig: aiengine.NewAIEngineConfig(),
		forgeInput:        forgeInput,
		forgeRunning:      true,
		forgeStarted:      true,
		messageQueue:      make(chan yakAIQueuedMessage, 1),
	}
	if err := handle.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "hotpatch-during-forge"},
		InputType:   "hotpatch",
		PayloadJSON: []byte(`{"hotpatch_type":"EnablePlan","params":{"enable_plan":true}}`),
	}); err != nil {
		t.Fatalf("hotpatch during direct Forge: %v", err)
	}
	select {
	case event := <-forgeInput.OutputChannel():
		if !event.GetIsConfigHotpatch() {
			t.Fatalf("direct Forge received non-hotpatch event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("direct Forge did not receive hotpatch")
	}
	if handle.runtime.EnablePlan == nil || !*handle.runtime.EnablePlan {
		t.Fatal("next-turn runtime snapshot did not retain active-Forge hotpatch")
	}
	select {
	case <-engine.events:
	case <-time.After(time.Second):
		t.Fatal("long-lived engine did not receive active-Forge hotpatch")
	}
}

func TestStatefulTaskScopedCapabilityHotpatchDoesNotPersist(t *testing.T) {
	engine := newFakeStatelessTurnEngine()
	handle := &yakAIEngineRuntimeHandle{
		engine:       engine,
		emitter:      noopEmitter{},
		currentTurn:  "turn-a",
		messageQueue: make(chan yakAIQueuedMessage, 1),
		runtime: yakRuntimeOptions{
			EnabledCapabilities: []yakAICapability{{Name: "base", Type: "tool"}},
		},
	}
	input := aiSessionInput{
		InputType:   "hotpatch",
		PayloadJSON: []byte(`{"hotpatch_type":"EnabledCapabilities","task_id":"task-a","params":{"enabled_capabilities":[{"name":"temporary","type":"tool"}]}}`),
	}
	if err := handle.SendInput(context.Background(), input); err != nil {
		t.Fatalf("active task-scoped hotpatch: %v", err)
	}
	if got := handle.runtime.EnabledCapabilities; len(got) != 1 || got[0].Name != "base" {
		t.Fatalf("task-scoped capability leaked into next-turn snapshot: %#v", got)
	}
	select {
	case <-engine.events:
	case <-time.After(time.Second):
		t.Fatal("active task did not receive task-scoped hotpatch")
	}
	handle.currentTurn = ""
	if err := handle.SendInput(context.Background(), input); err == nil || !strings.Contains(err.Error(), "requires an active task") {
		t.Fatalf("idle task-scoped hotpatch error = %v, want explicit rejection", err)
	}
}

func TestStatefulMessageQueueFullFailsAdmissionWithoutBlockingConsumer(t *testing.T) {
	engine := newFakeStatelessTurnEngine()
	handle := &yakAIEngineRuntimeHandle{
		engine:       engine,
		emitter:      noopEmitter{},
		messageQueue: make(chan yakAIQueuedMessage, 1),
	}
	handle.messageQueue <- yakAIQueuedMessage{turnID: "already-queued"}
	started := time.Now()
	err := handle.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "queue-overflow"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"do not block the node consumer"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "queue is full") {
		t.Fatalf("queue overflow error = %v, want explicit admission failure", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("queue overflow blocked for %s", elapsed)
	}
}

type noopAISessionRuntimeEmitter struct{}

func (noopAISessionRuntimeEmitter) Emit(string, []byte) {}

func (noopAISessionRuntimeEmitter) Done([]byte) {}

func (noopAISessionRuntimeEmitter) Failed(string, string, []byte) {}

type recordingAISessionRuntimeEmitter struct {
	eventType string
	payload   []byte
}

func (e *recordingAISessionRuntimeEmitter) Emit(eventType string, payload []byte) {
	e.eventType = eventType
	e.payload = append([]byte(nil), payload...)
}

func (*recordingAISessionRuntimeEmitter) Done([]byte) {}

func (*recordingAISessionRuntimeEmitter) Failed(string, string, []byte) {}

func TestStatefulRuntimeRejectsSingleRunInsteadOfLeavingFocusOpen(t *testing.T) {
	_, err := (yakAIEngineRuntimeDriver{}).Bind(
		context.Background(),
		aiSessionBinding{ExecutionMode: "single_run"},
		noopAISessionRuntimeEmitter{},
	)
	if err == nil || !strings.Contains(err.Error(), "single_run requires the stateless runtime") {
		t.Fatalf("expected explicit single-run rollback rejection, got %v", err)
	}
}

func TestBuildYakAIEngineOptionsDefersPinnedFocusToContextRelease(t *testing.T) {
	options, err := buildYakAIEngineOptions(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "ai-session-release"},
		RuntimeOptionSnapshotJSON: []byte(`{
			"focus_mode_loop":"http_fuzztest",
			"focus_release_id":"http_fuzztest@1.0.0+abcdef123456"
		}`),
	}, noopAISessionRuntimeEmitter{})
	if err != nil {
		t.Fatalf("build options: %v", err)
	}
	config := aiengine.NewAIEngineConfig(options...)
	if config.Focus != "" {
		t.Fatalf("logical engine focus must not execute directly when a server release is pinned: %q", config.Focus)
	}
}

func TestBuildYakAIEngineOptionsIncludesAttachmentContentAndCredentialProjection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer node-session-token" {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello from attachment"))
	}))
	defer server.Close()

	options, err := buildYakAIEngineOptions(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "ai-session-1"},
		Attachments: []aiSessionAttachmentRef{
			{
				AttachmentID: "inputf_123",
				Filename:     "targets.txt",
				ContentType:  "text/plain",
				DownloadURL:  server.URL,
			},
		},
		CredentialRefs: []aiSessionCredentialRef{
			{
				CredentialID:   "sourcecred-1",
				CredentialType: "ssa_source",
				Scope:          "ssa.source",
			},
		},
		PlatformBearerToken: "node-session-token",
		HTTPClient:          server.Client(),
	}, noopAISessionRuntimeEmitter{})
	if err != nil {
		t.Fatalf("build yak ai engine options: %v", err)
	}

	config := aiengine.NewAIEngineConfig(options...)
	if len(config.AttachedResources) != 2 {
		t.Fatalf("unexpected attached resource count: %d", len(config.AttachedResources))
	}

	attachmentContent := config.AttachedResources[0].Value
	if !strings.Contains(attachmentContent, "Filename: targets.txt") {
		t.Fatalf("unexpected attachment resource: %s", attachmentContent)
	}
	if !strings.Contains(attachmentContent, "hello from attachment") {
		t.Fatalf("unexpected attachment content: %s", attachmentContent)
	}

	credentialProjection := config.AttachedResources[1].Value
	if !strings.Contains(credentialProjection, "credential_id: sourcecred-1") {
		t.Fatalf("unexpected credential projection: %s", credentialProjection)
	}
	if !strings.Contains(credentialProjection, "Secret material is not exposed") {
		t.Fatalf("unexpected credential projection: %s", credentialProjection)
	}
}

func TestBuildYakAIEngineOptionsUsesDefaultAICallbackWhenEnabled(t *testing.T) {
	t.Parallel()

	originalTiered := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(originalTiered)

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled: true,
		IntelligentConfigs: []*ypb.AIModelConfig{
			{
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "openai",
					APIKey: "test-key",
				},
				ModelName: "gpt-4o",
			},
		},
	})

	options, err := buildYakAIEngineOptions(context.Background(), aiSessionBinding{
		Ref:                       aiSessionCommandRef{SessionID: "ai-session-default"},
		RuntimeOptionSnapshotJSON: []byte(`{"use_default_ai_config":true}`),
	}, noopAISessionRuntimeEmitter{})
	if err != nil {
		t.Fatalf("build yak ai engine options: %v", err)
	}

	config := aiengine.NewAIEngineConfig(options...)
	if config.AICallback == nil {
		t.Fatal("expected default ai callback to be configured")
	}
}

func TestBuildYakAIEngineOptionsUsesExplicitProviderSnapshotForAICallback(t *testing.T) {
	t.Parallel()

	options, err := buildYakAIEngineOptions(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "ai-session-provider"},
		ProviderPolicySnapshotJSON: []byte(`{
			"ai_service": "openai",
			"ai_model_name": "gpt-4o",
			"api_key": "test-key",
			"base_url": "https://api.openai.com/v1"
		}`),
	}, noopAISessionRuntimeEmitter{})
	if err != nil {
		t.Fatalf("build yak ai engine options: %v", err)
	}

	config := aiengine.NewAIEngineConfig(options...)
	if config.AICallback == nil {
		t.Fatal("expected explicit provider callback to be configured")
	}
}

func TestBuildYakAIEngineOptionsBindsThreeSessionModelRoles(t *testing.T) {
	originalLoader := loadYakProviderCallback
	t.Cleanup(func() { loadYakProviderCallback = originalLoader })
	loaded := make(map[consts.ModelTier]string)
	invoked := make([]string, 0, 4)
	loadYakProviderCallback = func(options yakRuntimeOptions, tier consts.ModelTier) (aicommon.AICallbackType, error) {
		loaded[tier] = options.AIService + "/" + options.AIModelName
		role := string(tier)
		return func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			invoked = append(invoked, role)
			return nil, nil
		}, nil
	}

	options, err := buildYakAIEngineOptions(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "ai-session-tiered"},
		ProviderPolicySnapshotJSON: []byte(`{
			"schema":"legion.ai.provider-policy.v2",
			"enabled":true,
			"intelligent_models":[{"ai_service":"high-provider","ai_model_name":"high-model","api_key":"high-key"}],
			"lightweight_models":[{"ai_service":"light-provider","ai_model_name":"light-model","api_key":"light-key"}],
			"vision_models":[{"ai_service":"vision-provider","ai_model_name":"vision-model","api_key":"vision-key"}]
		}`),
	}, noopAISessionRuntimeEmitter{})
	if err != nil {
		t.Fatalf("build tiered options: %v", err)
	}
	config := aiengine.NewAIEngineConfig(options...)
	if config.AICallback == nil || config.QualityPriorityAICallback == nil || config.SpeedPriorityAICallback == nil {
		t.Fatalf("missing engine role callbacks: %#v", config)
	}
	request := aicommon.NewAIRequest("test")
	_, _ = config.AICallback(nil, request)
	_, _ = config.QualityPriorityAICallback(nil, request)
	_, _ = config.SpeedPriorityAICallback(nil, request)
	visionConfig := &aicommon.Config{}
	_ = aicommon.WithContext(context.Background())(visionConfig)
	for _, option := range config.ExtOptions {
		_ = option(visionConfig)
	}
	if visionConfig.GetVisionPriorityRawAICallback() == nil {
		t.Fatal("missing session-scoped vision callback")
	}
	_, _ = visionConfig.GetVisionPriorityRawAICallback()(visionConfig, aicommon.NewAIRequest("image"))

	if loaded[consts.TierIntelligent] != "high-provider/high-model" ||
		loaded[consts.TierLightweight] != "light-provider/light-model" ||
		loaded[consts.TierVision] != "vision-provider/vision-model" {
		t.Fatalf("wrong provider loaded for model roles: %#v", loaded)
	}
	if got := strings.Join(invoked, ","); got != "intelligent,intelligent,lightweight,vision" {
		t.Fatalf("wrong callback routing: %s", got)
	}
}

func TestBuildYakAIEngineOptionsMapsExtendedRuntimeOptions(t *testing.T) {
	options, err := buildYakAIEngineOptions(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "ai-session-ext"},
		RuntimeOptionSnapshotJSON: []byte(`{
			"enable_system_file_system_operator": true,
			"disallow_require_for_user_prompt": true,
			"allow_plan_user_interact": true,
			"plan_user_interact_max_count": 4,
			"ai_review_risk_control_score": 0.7,
			"ai_call_auto_retry": 1,
			"ai_transaction_retry": 2,
			"disable_tool_interval_review": true,
			"ai_call_token_limit": 2048
		}`),
	}, noopAISessionRuntimeEmitter{})
	if err != nil {
		t.Fatalf("build yak ai engine options: %v", err)
	}

	config := aiengine.NewAIEngineConfig(options...)
	if len(config.ExtOptions) == 0 {
		t.Fatal("expected ext options to be configured")
	}

	aiConfig := aicommon.NewConfig(context.Background(), config.ExtOptions...)
	if !aiConfig.AllowPlanUserInteract {
		t.Fatal("expected allow plan user interact to be enabled")
	}
	if aiConfig.PlanUserInteractMaxCount != 4 {
		t.Fatalf("unexpected plan user interact max count: %d", aiConfig.PlanUserInteractMaxCount)
	}
	if aiConfig.AgreeAIScoreMiddle != 0.7 {
		t.Fatalf("unexpected ai review risk control score: %v", aiConfig.AgreeAIScoreMiddle)
	}
	if aiConfig.AiAutoRetry != 1 {
		t.Fatalf("unexpected ai auto retry: %d", aiConfig.AiAutoRetry)
	}
	if aiConfig.AiTransactionAutoRetry != 2 {
		t.Fatalf("unexpected ai transaction retry: %d", aiConfig.AiTransactionAutoRetry)
	}
	if !aiConfig.DisableIntervalReview {
		t.Fatal("expected disable tool interval review to be enabled")
	}
	if aiConfig.AiCallTokenLimit != 2048 {
		t.Fatalf("unexpected ai call token limit: %d", aiConfig.AiCallTokenLimit)
	}
	if aiConfig.AllowRequireForUserInteract {
		t.Fatal("expected require-for-user-interact to be disabled")
	}
	if _, err := aiConfig.GetAiToolManager().GetToolByName("ls"); err != nil {
		t.Fatalf("expected system file operator tools to be available: %v", err)
	}
}

func TestBuildYakAIEngineOptionsMapsPlanStrategyCapabilitiesAndSessionMCP(t *testing.T) {
	options, err := buildYakAIEngineOptions(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "ai-session-contract"},
		RuntimeOptionSnapshotJSON: []byte(`{
			"enable_plan":true,
			"enable_detached_plan":true,
			"plan_exec_task_concurrency":3,
			"forge_name":"yak-cve-analysis",
			"forge_params":[{"key":"target","value":"https://example.test"}],
			"enabled_capabilities":[{"name":"httpx","type":"tool"}],
			"strategy":{"enable_multi_agent":true,"enable_goal_mode":true,"goal_min_iterations":8},
			"session_mcp_servers":[{"name":"irify","url":"http://legion.test/mcp/sse","allowed_tools":["get_risk"]}]
		}`),
	}, noopAISessionRuntimeEmitter{})
	if err != nil {
		t.Fatalf("build yak ai engine options: %v", err)
	}

	engineConfig := aiengine.NewAIEngineConfig(options...)
	if len(engineConfig.ExtraMCPServers) != 1 || !engineConfig.RestrictToSessionMCP {
		t.Fatalf("session mcp contract was not applied: %#v", engineConfig.ExtraMCPServers)
	}
	if got := engineConfig.ExtraMCPServers[0].Server.Name; got != "irify" {
		t.Fatalf("session mcp server name was not preserved: %q", got)
	}
	config := aicommon.NewConfig(context.Background(), engineConfig.ExtOptions...)
	if !config.GetEnablePlanAndExec() || !config.GetEnableDetachedPlan() {
		t.Fatal("plan and detached plan must both be enabled")
	}
	if config.GetPlanExecTaskConcurrency() != 3 {
		t.Fatalf("unexpected plan concurrency: %d", config.GetPlanExecTaskConcurrency())
	}
	if config.GetForgeName() != "yak-cve-analysis" {
		t.Fatalf("unexpected forge: %q", config.GetForgeName())
	}
	runtime, err := mergedYakRuntimeOptions(aiSessionBinding{
		RuntimeOptionSnapshotJSON: []byte(`{
			"forge_name":"yak-cve-analysis",
			"forge_params":[{"key":"target","value":"https://example.test"}]
		}`),
	})
	if err != nil {
		t.Fatalf("decode forge runtime options: %v", err)
	}
	startParams := yakRuntimeOptionsToStartParams(runtime)
	if len(startParams.GetForgeParams()) != 1 ||
		startParams.GetForgeParams()[0].GetKey() != "target" ||
		startParams.GetForgeParams()[0].GetValue() != "https://example.test" {
		t.Fatalf("unexpected forge params: %#v", startParams.GetForgeParams())
	}
	if !config.EnableDispatchSubReactAgents || !config.GetEnableGoalMode() || config.GetGoalMinIterations() != 8 {
		t.Fatal("execution strategy was not applied")
	}
	capabilities := config.GetEnabledCapabilities()
	if len(capabilities) != 1 || capabilities[0].Name != "httpx" || capabilities[0].Type != "tool" {
		t.Fatalf("unexpected enabled capabilities: %#v", capabilities)
	}
}

func TestYakAIForgeExecParamsPreservesYakitNilVersusEmptySemantics(t *testing.T) {
	t.Parallel()

	if got := yakAIForgeExecParams(nil); got != nil {
		t.Fatalf("omitted forge params must remain nil, got %#v", got)
	}
	if got := yakAIForgeExecParams([]yakAIForgeParam{}); got == nil || len(got) != 0 {
		t.Fatalf("explicit empty forge params must remain non-nil and empty, got %#v", got)
	}
}

func TestYakRuntimeOptionsToStartParamsMapsHotpatchModelFields(t *testing.T) {
	params := yakRuntimeOptionsToStartParams(yakRuntimeOptions{
		AIService:   "openai",
		AIModelName: "gpt-test",
	})
	if params.GetAIService() != "openai" || params.GetAIModelName() != "gpt-test" {
		t.Fatalf("hotpatch model fields were not mapped: %#v", params)
	}
}

func TestBuildYakAIHotpatchEventRejectsUnsupportedAndUnknownParams(t *testing.T) {
	for name, payload := range map[string]string{
		"unsupported type":        `{"hotpatch_type":"UnknownPatch","params":{}}`,
		"unknown param":           `{"hotpatch_type":"EnablePlan","params":{"enable_plan":true,"desktop_window_id":"x"}}`,
		"missing value":           `{"hotpatch_type":"EnablePlan","params":{}}`,
		"unknown capability type": `{"hotpatch_type":"EnabledCapabilities","params":{"enabled_capabilities":[{"name":"x","type":"unknown"}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := buildYakAIHotpatchEvent(aiSessionInput{PayloadJSON: []byte(payload)})
			if err == nil {
				t.Fatalf("expected explicit hotpatch validation failure for %s", payload)
			}
		})
	}
}

func TestMergedYakRuntimeOptionsAllowsHotpatchToDisableAllProviderCapabilities(t *testing.T) {
	t.Parallel()

	options, err := mergedYakRuntimeOptions(aiSessionBinding{
		ProviderPolicySnapshotJSON: []byte(`{"enabled_capabilities":[{"name":"terminal","type":"tool"}]}`),
		RuntimeOptionSnapshotJSON:  []byte(`{"enabled_capabilities":[]}`),
	})
	if err != nil {
		t.Fatalf("merge runtime options: %v", err)
	}
	if options.EnabledCapabilities == nil || len(options.EnabledCapabilities) != 0 {
		t.Fatalf("explicit empty capability override was lost: %#v", options.EnabledCapabilities)
	}
}

func TestRunYakAIForgeDirectUsesExactForgeParamsAndEmitsResult(t *testing.T) {
	original := executeYakAIForge
	t.Cleanup(func() { executeYakAIForge = original })
	var gotName string
	var gotInput any
	var gotOptions int
	executeYakAIForge = func(name string, input any, options ...any) (any, error) {
		gotName = name
		gotInput = input
		gotOptions = len(options)
		return map[string]string{"summary": "done"}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	emitter := &recordingAISessionRuntimeEmitter{}
	err := runYakAIForgeDirect(
		ctx,
		aiengine.NewAIEngineConfig(aiengine.WithReviewPolicy("manual")),
		yakRuntimeOptions{
			ForgeName: " yak-cve-analysis ",
			ForgeParams: []yakAIForgeParam{
				{Key: "target", Value: "https://example.test"},
			},
		},
		aiSessionBinding{Ref: aiSessionCommandRef{SessionID: "session-1"}},
		chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10),
		emitter,
		"this query must not replace explicit ForgeParams",
	)
	if err != nil {
		t.Fatalf("runYakAIForgeDirect() error = %v", err)
	}
	if gotName != "yak-cve-analysis" || gotOptions == 0 {
		t.Fatalf("unexpected direct forge invocation: name=%q options=%d", gotName, gotOptions)
	}
	params, ok := gotInput.([]*ypb.ExecParamItem)
	if !ok || len(params) != 1 || params[0].GetKey() != "target" || params[0].GetValue() != "https://example.test" {
		t.Fatalf("explicit forge params were not preserved: %#v", gotInput)
	}
	if emitter.eventType != aiSessionRuntimeEventReason || !strings.Contains(string(emitter.payload), "done") {
		t.Fatalf("direct forge result was not emitted: type=%q payload=%s", emitter.eventType, emitter.payload)
	}
}

func TestBuildYakAIEngineOptionsRejectsUnknownRuntimeOption(t *testing.T) {
	t.Parallel()

	_, err := buildYakAIEngineOptions(context.Background(), aiSessionBinding{
		Ref:                       aiSessionCommandRef{SessionID: "ai-session-unknown"},
		RuntimeOptionSnapshotJSON: []byte(`{"desktop_window_id":"not-supported"}`),
	}, noopAISessionRuntimeEmitter{})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected explicit unsupported-field failure, got %v", err)
	}
}

type stubAISyncOperator struct {
	*aicommon.AIEngineOperatorBase
	syncType  string
	syncInput string
	syncID    string
}

func newStubAISyncOperator() *stubAISyncOperator {
	operator := &stubAISyncOperator{}
	operator.AIEngineOperatorBase = aicommon.NewAIEngineOperatorBase(func(event *ypb.AIInputEvent) error {
		if event.GetIsSyncMessage() {
			operator.syncType = event.GetSyncType()
			operator.syncInput = event.GetSyncJsonInput()
			operator.syncID = event.GetSyncID()
		}
		return nil
	}, nil, nil)
	return operator
}

func TestYakAIInputContentParsesSyncEvent(t *testing.T) {
	t.Parallel()

	content, interactive, syncEvent, options, err := yakAIInputContent(aiSessionInput{
		InputType:   "sync_event",
		PayloadJSON: []byte(`{"sync_type":"recovery_plan_and_exec","sync_json_input":{"coordinator_id":"coor-1","start_task_index":"1-2"}}`),
	})
	if err != nil {
		t.Fatalf("yakAIInputContent() error = %v", err)
	}
	if content != "" {
		t.Fatalf("unexpected content: %q", content)
	}
	if interactive {
		t.Fatal("sync_event should not be interactive")
	}
	if len(options) != 0 {
		t.Fatalf("sync_event should not have attached resource options: %d", len(options))
	}
	if syncEvent == nil {
		t.Fatal("expected sync event")
	}
	if syncEvent.SyncType != "recovery_plan_and_exec" {
		t.Fatalf("unexpected sync type: %s", syncEvent.SyncType)
	}
	assertJSONEqualRuntimeYak(t, []byte(syncEvent.SyncJSONInput), `{"coordinator_id":"coor-1","start_task_index":"1-2"}`)
}

func TestYakAIInputContentTreatsUserInterventionAsInteractivePayload(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"id":"interactive-1","option_value":"input_params","review_type":"exec_aiforge_review_require","params":{"query":"target"}}`)
	content, interactive, syncEvent, options, err := yakAIInputContent(aiSessionInput{
		InputType:   "user_intervention",
		PayloadJSON: payload,
	})
	if err != nil {
		t.Fatalf("yakAIInputContent() error = %v", err)
	}
	if !interactive {
		t.Fatal("user_intervention should be interactive")
	}
	if syncEvent != nil {
		t.Fatalf("user_intervention should not be a sync event: %#v", syncEvent)
	}
	if len(options) != 0 {
		t.Fatalf("user_intervention should not have attached resource options: %d", len(options))
	}
	assertJSONEqualRuntimeYak(t, []byte(content), string(payload))
}

func TestBuildYakAIInterventionEventCarriesEndpointID(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"id":"interactive-1","suggestion":"continue","review_type":"tool_use_review_require"}`)
	event, err := buildYakAIInterventionEvent(aiSessionInput{
		InputType:   "user_intervention",
		PayloadJSON: payload,
	})
	if err != nil {
		t.Fatalf("buildYakAIInterventionEvent() error = %v", err)
	}
	if !event.GetIsInteractiveMessage() {
		t.Fatal("expected interactive input event")
	}
	if event.GetInteractiveId() != "interactive-1" {
		t.Fatalf("unexpected interactive id: %q", event.GetInteractiveId())
	}
	assertJSONEqualRuntimeYak(t, []byte(event.GetInteractiveJSONInput()), string(payload))
}

func TestBuildYakAIInterventionEventUnwrapsComposerReviewPayload(t *testing.T) {
	t.Parallel()

	event, err := buildYakAIInterventionEvent(aiSessionInput{
		InputType: "user_intervention",
		PayloadJSON: []byte(`{
			"content":"{\"id\":\"interactive-nested\",\"suggestion\":\"enough-cancel\"}",
			"showQS":"Stop tool"
		}`),
	})
	if err != nil {
		t.Fatalf("buildYakAIInterventionEvent() error = %v", err)
	}
	if event.GetInteractiveId() != "interactive-nested" {
		t.Fatalf("unexpected interactive id: %q", event.GetInteractiveId())
	}
	assertJSONEqualRuntimeYak(
		t,
		[]byte(event.GetInteractiveJSONInput()),
		`{"id":"interactive-nested","suggestion":"enough-cancel"}`,
	)
}

func TestBuildYakAIInterventionEventMapsFreeInputWithoutEndpointID(t *testing.T) {
	t.Parallel()

	event, err := buildYakAIInterventionEvent(aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "intervention-command-1"},
		InputType:   "user_intervention",
		PayloadJSON: []byte(`{"content":"check the authorization path too"}`),
	})
	if err != nil {
		t.Fatalf("buildYakAIInterventionEvent() error = %v", err)
	}
	if !event.GetIsFreeInput() {
		t.Fatal("expected free input event")
	}
	if event.GetFreeInput() != "check the authorization path too" {
		t.Fatalf("unexpected free input: %q", event.GetFreeInput())
	}
	if event.GetIsInteractiveMessage() {
		t.Fatal("free input must not be marked interactive")
	}
	resources := event.GetAttachedResourceInfo()
	if len(resources) != 1 || resources[0].GetType() != aicommon.USER_FREE_INPUT_UUID ||
		resources[0].GetValue() != "intervention-command-1" {
		t.Fatalf("free intervention queue identity = %#v", resources)
	}
}

func TestBuildYakAIInterventionEventRejectsReviewWithoutEndpointID(t *testing.T) {
	t.Parallel()

	_, err := buildYakAIInterventionEvent(aiSessionInput{
		InputType:   "review_response",
		PayloadJSON: []byte(`{"content":"continue"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "interactive id is required") {
		t.Fatalf("expected missing interactive id error, got %v", err)
	}
}

func TestYakAIInputContentMapsAttachedResources(t *testing.T) {
	t.Parallel()

	content, interactive, syncEvent, options, err := yakAIInputContent(aiSessionInput{
		InputType: "message",
		PayloadJSON: []byte(`{
			"content":"scan target",
			"attached_resource_info":[
				{"type":"file","key":"file_path","value":"/tmp/targets.txt"},
				{"type":"knowledge_base","key":"system_flag","value":"all_knowledge_base"},
				{"type":"aiforge","key":"name","value":"yak-cve-analysis"},
				{"type":"aitool","key":"name","value":"httpx"}
			]
		}`),
	})
	if err != nil {
		t.Fatalf("yakAIInputContent() error = %v", err)
	}
	if content != "scan target" {
		t.Fatalf("unexpected content: %q", content)
	}
	if interactive {
		t.Fatal("message should not be interactive")
	}
	if syncEvent != nil {
		t.Fatalf("message should not be a sync event: %#v", syncEvent)
	}
	config := aiengine.NewAIEngineConfig(options...)
	if len(config.AttachedResources) != 4 {
		t.Fatalf("unexpected attached resource count: %d", len(config.AttachedResources))
	}
	if got := config.AttachedResources[0]; got.Type != "file" || got.Key != "file_path" || got.Value != "/tmp/targets.txt" {
		t.Fatalf("unexpected file attached resource: %#v", got)
	}
	if got := config.AttachedResources[1]; got.Type != "knowledge_base" || got.Key != "system_flag" || got.Value != "all_knowledge_base" {
		t.Fatalf("unexpected knowledge attached resource: %#v", got)
	}
	if got := config.AttachedResources[2]; got.Type != "aiforge" || got.Key != "name" || got.Value != "yak-cve-analysis" {
		t.Fatalf("unexpected forge attached resource: %#v", got)
	}
	if got := config.AttachedResources[3]; got.Type != "aitool" || got.Key != "name" || got.Value != "httpx" {
		t.Fatalf("unexpected tool attached resource: %#v", got)
	}
}

func TestDispatchYakAISyncEventUsesOperator(t *testing.T) {
	t.Parallel()

	operator := newStubAISyncOperator()
	err := dispatchYakAISyncEvent(operator, &yakAISyncEvent{
		SyncType:      "skip_subtask_in_plan",
		SyncJSONInput: `{"skip_current_task":true}`,
		SyncID:        "sync-123",
	})
	if err != nil {
		t.Fatalf("dispatchYakAISyncEvent() error = %v", err)
	}
	if operator.syncType != "skip_subtask_in_plan" {
		t.Fatalf("unexpected sync type: %s", operator.syncType)
	}
	if operator.syncID != "sync-123" {
		t.Fatalf("unexpected sync id: %s", operator.syncID)
	}
	assertJSONEqualRuntimeYak(t, []byte(operator.syncInput), `{"skip_current_task":true}`)
}

func TestBuildYakAIHotpatchEventMapsTypedConfiguration(t *testing.T) {
	t.Parallel()

	event, err := buildYakAIHotpatchEvent(aiSessionInput{
		InputType: "hotpatch",
		PayloadJSON: []byte(`{
			"hotpatch_type":"EnabledCapabilities",
			"task_id":"task-1",
			"params":{
				"forge_name":"yak-cve-analysis",
				"forge_params":[{"key":"target","value":"https://example.test"}],
				"enabled_capabilities":[{"name":"httpx","type":"tool"}]
			}
		}`),
	})
	if err != nil {
		t.Fatalf("build hotpatch: %v", err)
	}
	if !event.GetIsConfigHotpatch() || event.GetHotpatchType() != "EnabledCapabilities" || event.GetTaskId() != "task-1" {
		t.Fatalf("unexpected hotpatch envelope: %#v", event)
	}
	capabilities := event.GetParams().GetEnabledCapabilities()
	if len(capabilities) != 1 || capabilities[0].GetName() != "httpx" || capabilities[0].GetType() != "tool" {
		t.Fatalf("unexpected hotpatch capabilities: %#v", capabilities)
	}
	forgeParams := event.GetParams().GetForgeParams()
	if len(forgeParams) != 1 || forgeParams[0].GetKey() != "target" || forgeParams[0].GetValue() != "https://example.test" {
		t.Fatalf("unexpected hotpatch forge params: %#v", forgeParams)
	}
}

func assertJSONEqualRuntimeYak(t *testing.T, got []byte, want string) {
	t.Helper()

	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("unmarshal got json: %v", err)
	}

	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("unmarshal want json: %v", err)
	}

	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("unexpected json payload: got=%s want=%s", string(got), want)
	}
}
