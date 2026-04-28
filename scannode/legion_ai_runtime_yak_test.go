package scannode

import (
	"context"
	"net/http"
	"net/http/httptest"
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
