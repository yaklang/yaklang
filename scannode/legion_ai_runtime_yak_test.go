package scannode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type noopAISessionRuntimeEmitter struct{}

func (noopAISessionRuntimeEmitter) Emit(string, []byte) {}

func (noopAISessionRuntimeEmitter) Done([]byte) {}

func (noopAISessionRuntimeEmitter) Failed(string, string, []byte) {}

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

func TestBuildYakAIEngineOptionsMapsExtendedRuntimeOptions(t *testing.T) {
	t.Parallel()

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

type stubAISyncOperator struct {
	*aicommon.AIEngineOperatorBase
	syncType  string
	syncInput string
}

func newStubAISyncOperator() *stubAISyncOperator {
	operator := &stubAISyncOperator{}
	operator.AIEngineOperatorBase = aicommon.NewAIEngineOperatorBase(func(event *ypb.AIInputEvent) error {
		if event.GetIsSyncMessage() {
			operator.syncType = event.GetSyncType()
			operator.syncInput = event.GetSyncJsonInput()
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
	})
	if err != nil {
		t.Fatalf("dispatchYakAISyncEvent() error = %v", err)
	}
	if operator.syncType != "skip_subtask_in_plan" {
		t.Fatalf("unexpected sync type: %s", operator.syncType)
	}
	assertJSONEqualRuntimeYak(t, []byte(operator.syncInput), `{"skip_current_task":true}`)
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
