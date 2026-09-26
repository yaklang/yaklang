package scannode

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func testLegionContextForgeRelease(t *testing.T) *aiv1.ContextForgeRelease {
	t.Helper()
	release := &aiv1.ContextForgeRelease{
		SchemaVersion:     legionForgeReleaseSchemaV1,
		ReleaseId:         "aiforge_rel_demo_v1",
		ForgeId:           "aiforge_demo",
		Version:           "1.0.0",
		Name:              "demo-advisory",
		ExecutorKind:      legionForgeExecutorConfigV1,
		CapabilityProfile: legionForgeAdvisoryProfile,
		InitPrompt:        "Analyze {{.Forge.UserParams}} for {{.Forge.UserQuery}}.",
		PersistentPrompt:  "Use only supplied facts.",
		ResultPrompt:      "Return a Markdown summary.",
		// The server has already validated and normalized invocation parameters.
		// This intentionally invalid source must never be parsed by the node.
		ParameterRuleYaklangCode: `this is intentionally not valid Yak code`,
		Parameters: []*aiv1.ContextForgeParameter{
			{Key: "topic", Value: "bounded input", ValueKind: "string"},
		},
		InputSchemaJson: []byte(`{"type":"object","required":["topic"]}`),
	}
	rehashLegionContextForgeRelease(t, release)
	return release
}

func rehashLegionContextForgeRelease(t *testing.T, release *aiv1.ContextForgeRelease) {
	t.Helper()
	definitionDigest, err := contextForgeDefinitionSHA256(release)
	if err != nil {
		t.Fatal(err)
	}
	release.DefinitionSha256 = definitionDigest
	digest, err := contextForgeReleaseSHA256(release)
	if err != nil {
		t.Fatal(err)
	}
	release.Sha256 = digest
}

func TestBuildContextForgeBlueprintUsesInlineDefinition(t *testing.T) {
	release := testLegionContextForgeRelease(t)
	config, blueprint, params, err := buildContextForgeBlueprint(release)
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != release.Name || blueprint.Name != release.Name {
		t.Fatalf("unexpected inline definition: config=%q blueprint=%q", config.Name, blueprint.Name)
	}
	if len(blueprint.Tools) != 0 || len(params) != 1 || params[0].GetKey() != "topic" {
		t.Fatalf("unexpected tools or params: tools=%d params=%#v", len(blueprint.Tools), params)
	}
	if config.YakForgeBlueprintAIDOptionsConfig == nil || config.YakForgeBlueprintAIDOptionsConfig.DisableToolUse == nil || !*config.YakForgeBlueprintAIDOptionsConfig.DisableToolUse {
		t.Fatal("server advisory release must disable tool use")
	}
}

func TestValidateContextForgeReleaseRejectsTampering(t *testing.T) {
	release := testLegionContextForgeRelease(t)
	release.PlanPrompt += " changed"
	err := validateContextForgeRelease(release)
	if err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("expected identity mismatch, got %v", err)
	}
}

func TestValidateContextForgeReleaseRejectsToolsAndResources(t *testing.T) {
	t.Run("tool", func(t *testing.T) {
		release := testLegionContextForgeRelease(t)
		release.DeclaredToolNames = []string{"do_http_request"}
		release.Sha256, _ = contextForgeReleaseSHA256(release)
		if err := validateContextForgeRelease(release); err == nil || !strings.Contains(err.Error(), "cannot declare") {
			t.Fatalf("expected tool rejection, got %v", err)
		}
	})
	t.Run("resource", func(t *testing.T) {
		release := testLegionContextForgeRelease(t)
		release.Parameters[0].ValueKind = "resource"
		release.Parameters[0].Value = "attachment-1"
		release.Sha256, _ = contextForgeReleaseSHA256(release)
		if err := validateContextForgeRelease(release); err == nil || !strings.Contains(err.Error(), "unique and sorted") {
			t.Fatalf("expected resource rejection, got %v", err)
		}
	})
}

func TestValidateContextForgeReleaseAllowsExactReportAndHTTPProfiles(t *testing.T) {
	report := testLegionContextForgeRelease(t)
	report.CapabilityProfile = legionForgeReportProfile
	report.DeclaredToolNames = append([]string(nil), legionForgeReportTools...)
	report.Parameters[0] = &aiv1.ContextForgeParameter{
		Key: "report-file", Value: "inputs/field_123/01-report.xlsx", ValueKind: "resource",
	}
	rehashLegionContextForgeRelease(t, report)
	if err := validateContextForgeRelease(report); err != nil {
		t.Fatalf("valid report profile was rejected: %v", err)
	}
	_, blueprint, _, err := buildContextForgeBlueprint(report)
	if err != nil {
		t.Fatal(err)
	}
	if blueprint == nil {
		t.Fatal("report profile did not build an inline blueprint")
	}

	httpRelease := testLegionContextForgeRelease(t)
	httpRelease.CapabilityProfile = legionForgeHTTPProfile
	httpRelease.DeclaredToolNames = append([]string(nil), legionForgeHTTPTools...)
	httpRelease.Parameters[0] = &aiv1.ContextForgeParameter{
		Key: "target-url", Value: "https://owned.example/", ValueKind: "string",
	}
	rehashLegionContextForgeRelease(t, httpRelease)
	if err := validateContextForgeRelease(httpRelease); err != nil {
		t.Fatalf("valid HTTP profile was rejected: %v", err)
	}

	httpRelease.DeclaredToolNames = append([]string(nil), legionForgeHTTPTools[1:]...)
	rehashLegionContextForgeRelease(t, httpRelease)
	if err := validateContextForgeRelease(httpRelease); err == nil || !strings.Contains(err.Error(), "exact") {
		t.Fatalf("incomplete HTTP tool set error = %v", err)
	}
}

func TestBuildContextForgeBlueprintOptsIntoResultPolicy(t *testing.T) {
	release := testLegionContextForgeRelease(t)
	config, blueprint, params, err := buildContextForgeBlueprint(release)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var prompts []string
	var budgets []int64
	coordinator, err := blueprint.CreateCoordinatorWithQueryAndParams(ctx, "summarize the supplied facts", params,
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisableMemoryTriage(true),
		aicommon.WithAllowRequireForUserInteract(false),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompts = append(prompts, request.GetPrompt())
			model := &aispec.AIConfig{}
			aispec.WithMaxTokens(8192)(model)
			for _, option := range request.GetExtraSpecOpts() {
				option(model)
			}
			budgets = append(budgets, *model.MaxTokens)
			response := c.NewAIResponse()
			response.EmitReasonStream(strings.NewReader("internal reasoning"))
			output := ""
			if len(prompts) == 2 {
				output = "# Report\nOnly the supplied facts were reviewed."
			}
			response.EmitOutputStream(strings.NewReader(output))
			response.Close()
			return response, nil
		}))
	require.NoError(t, err)
	require.NotNil(t, coordinator.ResultHandler)
	coordinator.ResultHandler(coordinator)

	require.Equal(t, []int64{4096, 4096}, budgets, "only the server adapter opts into the report budget and retry")
	require.Contains(t, prompts[0], release.ResultPrompt)
	require.Contains(t, prompts[1], "最终输出通道")
	require.Equal(t, "# Report\nOnly the supplied facts were reviewed.", config.ForgeResult.Formated)
}
