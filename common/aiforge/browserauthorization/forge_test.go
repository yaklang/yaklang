package browserauthorization

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type fakeAuthorizationService struct {
	available           bool
	workspace           browser.ExtensionAuthorizationWorkspace
	inspectIDs          []string
	plans               []browser.ExtensionAuthorizationPlanInput
	logicalBindings     []browser.ExtensionAuthorizationLogicalBindingInput
	transformLists      []string
	executions          []browser.ExtensionAuthorizationExecutionInput
	evidenceInspects    []browser.ExtensionAuthorizationEvidenceInspectInput
	evidencePackets     []browser.ExtensionAuthorizationEvidencePacketInput
	evidenceDiffs       []browser.ExtensionAuthorizationEvidenceDiffInput
	evidenceValidations []browser.ExtensionAuthorizationEvidenceValidationInput
}

func (f *fakeAuthorizationService) Available() bool {
	return f.available
}

func (f *fakeAuthorizationService) InspectWorkspace(
	_ context.Context,
	workspaceID string,
	_ bool,
) (browser.ExtensionAuthorizationWorkspace, error) {
	f.inspectIDs = append(f.inspectIDs, workspaceID)
	if workspaceID != f.workspace.ID {
		return browser.ExtensionAuthorizationWorkspace{}, errors.New("workspace not found")
	}
	return f.workspace, nil
}

func (f *fakeAuthorizationService) CreatePlan(
	_ context.Context,
	input browser.ExtensionAuthorizationPlanInput,
) (browser.ExtensionAuthorizationWorkspace, error) {
	f.plans = append(f.plans, input)
	if input.WorkspaceID != f.workspace.ID ||
		input.CandidateID != "candidate-1" ||
		f.workspace.Plan == nil {
		return browser.ExtensionAuthorizationWorkspace{}, errors.New("candidate mismatch")
	}
	return f.workspace, nil
}

func (f *fakeAuthorizationService) BindLogicalRequests(
	_ context.Context,
	input browser.ExtensionAuthorizationLogicalBindingInput,
) (browser.ExtensionAuthorizationWorkspace, error) {
	f.logicalBindings = append(f.logicalBindings, input)
	if input.WorkspaceID != f.workspace.ID ||
		input.TransformProfiles.Left == "" ||
		input.TransformProfiles.Right == "" {
		return browser.ExtensionAuthorizationWorkspace{}, errors.New("logical binding mismatch")
	}
	return f.workspace, nil
}

func (f *fakeAuthorizationService) ListTransformProfiles(
	_ context.Context,
	workspaceID string,
) (browser.ExtensionAuthorizationTransformProfileCandidates, error) {
	f.transformLists = append(f.transformLists, workspaceID)
	if workspaceID != f.workspace.ID {
		return browser.ExtensionAuthorizationTransformProfileCandidates{}, errors.New(
			"workspace not found",
		)
	}
	return browser.ExtensionAuthorizationTransformProfileCandidates{
		Left: []browser.ExtensionAuthorizationTransformProfileCandidate{{
			ID:                    "profile-left",
			Name:                  "A request envelope",
			Methods:               []string{"POST"},
			URLPattern:            "*/api/orders",
			OutputDestinations:    []string{"body.encryptedData"},
			Eligible:              true,
			DynamicFieldsEligible: false,
			LogicalBodyEligible:   true,
			UpdatedAt:             10,
		}},
		Right: []browser.ExtensionAuthorizationTransformProfileCandidate{{
			ID:                    "profile-right",
			Name:                  "B request envelope",
			Methods:               []string{"POST"},
			URLPattern:            "*/api/orders",
			OutputDestinations:    []string{"body.encryptedData"},
			Eligible:              true,
			DynamicFieldsEligible: false,
			LogicalBodyEligible:   true,
			UpdatedAt:             11,
		}},
	}, nil
}

func (f *fakeAuthorizationService) ExecutePlan(
	_ context.Context,
	input browser.ExtensionAuthorizationExecutionInput,
) (browser.ExtensionAuthorizationWorkspace, error) {
	f.executions = append(f.executions, input)
	if input.WorkspaceID != f.workspace.ID ||
		f.workspace.Plan == nil ||
		input.PlanID != f.workspace.Plan.ID {
		return browser.ExtensionAuthorizationWorkspace{}, errors.New("plan mismatch")
	}
	workspace := f.workspace
	workspace.Execution = &browser.ExtensionAuthorizationExecution{
		Version:     1,
		ID:          "execution-1",
		WorkspaceID: workspace.ID,
		PlanID:      input.PlanID,
		State:       "completed",
		Verdict:     "protected",
		Confidence:  "high",
		Cases: []browser.ExtensionAuthorizationCaseExecution{
			{ID: "a-own", State: "completed", Result: &browser.ExtensionAuthorizationRequestExecution{Outcome: "success", Status: 200}},
			{ID: "b-own", State: "completed", Result: &browser.ExtensionAuthorizationRequestExecution{Outcome: "success", Status: 200}},
			{ID: "a-to-b", State: "completed", Result: &browser.ExtensionAuthorizationRequestExecution{Outcome: "denied", Status: 403}},
			{ID: "b-to-a", State: "completed", Result: &browser.ExtensionAuthorizationRequestExecution{Outcome: "denied", Status: 403}},
		},
		RequestCount: 4,
		Evidence:     []browser.ExtensionAuthorizationCanaryEvidence{},
		Reasons:      []string{"both cross-identity requests were denied"},
	}
	f.workspace = workspace
	return workspace, nil
}

func (f *fakeAuthorizationService) InspectEvidence(
	_ context.Context,
	input browser.ExtensionAuthorizationEvidenceInspectInput,
) (browser.ExtensionAuthorizationEvidenceBundle, error) {
	f.evidenceInspects = append(f.evidenceInspects, input)
	if input.WorkspaceID != f.workspace.ID ||
		f.workspace.Execution == nil ||
		input.ExecutionID != f.workspace.Execution.ID {
		return browser.ExtensionAuthorizationEvidenceBundle{}, errors.New("execution mismatch")
	}
	return browser.ExtensionAuthorizationEvidenceBundle{
		Version: 1, WorkspaceID: input.WorkspaceID, ExecutionID: input.ExecutionID,
		Mode: f.workspace.Mode,
		Cases: []browser.ExtensionAuthorizationEvidenceCase{
			{ID: "a-to-b", Label: "A to B", ResponseAvailable: true},
			{ID: "b-own", Label: "B own", ResponseAvailable: true},
		},
		Comparisons: []browser.ExtensionAuthorizationEvidenceComparison{{
			ID: "a-to-b", LeftCaseID: "a-to-b", RightCaseID: "b-own",
			Purpose: "authorization",
		}},
	}, nil
}

func (f *fakeAuthorizationService) ReadEvidencePacket(
	_ context.Context,
	input browser.ExtensionAuthorizationEvidencePacketInput,
) (browser.ExtensionAuthorizationEvidencePacket, error) {
	f.evidencePackets = append(f.evidencePackets, input)
	if input.WorkspaceID != f.workspace.ID || input.CaseID == "" {
		return browser.ExtensionAuthorizationEvidencePacket{}, errors.New("packet mismatch")
	}
	return browser.ExtensionAuthorizationEvidencePacket{
		Version: 1, WorkspaceID: input.WorkspaceID, ExecutionID: input.ExecutionID,
		CaseID: input.CaseID, Side: input.Side, View: input.View,
		PacketBase64: "SFRUUC8xLjEgMjAwIE9LDQoNCg==",
	}, nil
}

func (f *fakeAuthorizationService) DiffEvidence(
	_ context.Context,
	input browser.ExtensionAuthorizationEvidenceDiffInput,
) (browser.ExtensionAuthorizationEvidenceDiff, error) {
	f.evidenceDiffs = append(f.evidenceDiffs, input)
	if input.WorkspaceID != f.workspace.ID ||
		input.LeftCaseID == "" ||
		input.RightCaseID == "" {
		return browser.ExtensionAuthorizationEvidenceDiff{}, errors.New("diff mismatch")
	}
	return browser.ExtensionAuthorizationEvidenceDiff{
		Version: 1, WorkspaceID: input.WorkspaceID, ExecutionID: input.ExecutionID,
		LeftCaseID: input.LeftCaseID, RightCaseID: input.RightCaseID,
		Scope: input.Scope, View: input.View, Representation: "structured",
		Entries: []browser.ExtensionAuthorizationEvidenceDiffEntry{{
			Path: "body.user.id", Kind: "changed", Left: "2", Right: "2",
			Semantic: true,
		}},
	}, nil
}

func (f *fakeAuthorizationService) ValidateEvidence(
	_ context.Context,
	input browser.ExtensionAuthorizationEvidenceValidationInput,
) (browser.ExtensionAuthorizationEvidenceValidation, error) {
	f.evidenceValidations = append(f.evidenceValidations, input)
	if input.WorkspaceID != f.workspace.ID ||
		input.ExecutionID == "" ||
		len(input.Paths) == 0 {
		return browser.ExtensionAuthorizationEvidenceValidation{}, errors.New(
			"validation mismatch",
		)
	}
	return browser.ExtensionAuthorizationEvidenceValidation{
		Version: 1, WorkspaceID: input.WorkspaceID, ExecutionID: input.ExecutionID,
		Direction: input.Direction, Verified: true,
		Evidence: []browser.ExtensionAuthorizationCanaryEvidence{{
			Direction: input.Direction, Path: input.Paths[0],
			Source: "response-json-user-canary",
		}},
		Verdict: "confirmed", Confidence: "high", VerdictChanged: true,
	}, nil
}

func testAuthorizationWorkspace() browser.ExtensionAuthorizationWorkspace {
	return browser.ExtensionAuthorizationWorkspace{
		Version: 1,
		ID:      "workspace-1",
		Mode:    "horizontal",
		State:   "ready",
		Left: browser.ExtensionAuthorizationIdentitySlot{
			Side:               "left",
			AccountLabel:       "Alice",
			Origin:             "https://example.test",
			IsolationContextID: "normal",
			ContextReference: browser.ExtensionAuthorizationContextReference{
				Kind: "handle",
				ID:   "secret-left-handle",
			},
			Authentication: browser.ExtensionAuthorizationAuthentication{
				Status:          "authenticated",
				AuthCookieNames: []string{"session"},
			},
		},
		Right: browser.ExtensionAuthorizationIdentitySlot{
			Side:               "right",
			AccountLabel:       "Bob",
			Origin:             "https://example.test",
			IsolationContextID: "incognito",
			ContextReference: browser.ExtensionAuthorizationContextReference{
				Kind: "attestation",
				ID:   "secret-right-attestation",
			},
			Authentication: browser.ExtensionAuthorizationAuthentication{
				Status:          "authenticated",
				AuthCookieNames: []string{"session"},
			},
		},
		Proof: browser.ExtensionAuthorizationProof{
			Level:   "strong",
			Reasons: []string{"separate cookie stores"},
		},
		Baselines: browser.ExtensionAuthorizationBaselineSet{
			Left: &browser.ExtensionAuthorizationBaseline{
				Version:          1,
				ID:               "baseline-left",
				DeviceID:         "secret-device-id",
				InstallationID:   "secret-installation-id",
				GrantID:          "secret-grant-id",
				NetworkRequestID: "secret-network-request-id",
				Origin:           "https://example.test",
				AuthContextReference: browser.ExtensionAuthorizationContextReference{
					Kind: "handle",
					ID:   "secret-baseline-handle",
				},
				Request: browser.ExtensionAuthorizationBaselineRequest{
					Method: "GET",
					URL:    "https://example.test/orders/1",
					Path:   "/orders/:resource",
				},
			},
		},
		BaselinePair: browser.ExtensionAuthorizationBaselinePair{
			State: "matched",
			ResourceCandidates: []browser.ExtensionAuthorizationResourceCandidate{{
				ID:       "candidate-1",
				Source:   "wire",
				Location: "path",
				Path:     "path.segment[1]",
				Category: "resource",
			}},
		},
		Plan: &browser.ExtensionAuthorizationPlan{
			Version:     1,
			ID:          "plan-1",
			WorkspaceID: "workspace-1",
			CandidateID: "candidate-1",
			State:       "ready",
			Selector: browser.ExtensionAuthorizationPlanSelector{
				Source:   "wire",
				Location: "path",
				Path:     "path.segment[1]",
			},
			Cases: []browser.ExtensionAuthorizationPlanCase{
				{ID: "a-own"},
				{ID: "b-own"},
				{ID: "a-to-b"},
				{ID: "b-to-a"},
			},
			RequestBudget: 4,
			ExpiresAt:     2000,
		},
		ExpiresAt: 2000,
	}
}

func toolByName(t *testing.T, tools []*aitool.Tool, name string) *aitool.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %s not found", name)
	return nil
}

func toolExecutionResult(t *testing.T, result *aitool.ToolResult) interface{} {
	t.Helper()
	require.True(t, result.Success)
	execution, ok := result.Data.(*aitool.ToolExecutionResult)
	require.True(t, ok)
	return execution.Result
}

func TestEmbeddedPromptsKeepDeterministicAuthorizationBoundary(t *testing.T) {
	require.Contains(t, initializePrompt, "authorization.plan.execute")
	require.Contains(t, initializePrompt, "authorization.review.submit")
	require.Contains(t, initializePrompt, "authorization.verdict.reconcile")
	require.Contains(t, initializePrompt, "authorization.transform.profiles")
	require.Contains(t, initializePrompt, "requiresLogicalBinding=false")
	require.Contains(t, initializePrompt, "GraphQL")
	require.Contains(t, initializePrompt, "Reply in the user's language")
	require.Contains(t, persistentPrompt, "`manual`, `ai`, or `yolo`")
	require.Contains(t, persistentPrompt, "No tool argument may switch to another workspace")
	require.Contains(t, persistentPrompt, "GraphQL query documents remain outside")
	require.Contains(t, initializePrompt, "never a directory")
	require.Contains(t, persistentPrompt, "never written into the Agent workspace")
	require.Contains(t, initializePrompt, "admin")
	require.Contains(t, initializePrompt, "immutable")
	require.NotContains(t, initializePrompt, "Cookie value")

	parsed, err := template.New("browser-authorization-init").Parse(initializePrompt)
	require.NoError(t, err)
	var rendered bytes.Buffer
	require.NoError(t, parsed.Execute(&rendered, map[string]interface{}{
		"Forge": map[string]interface{}{
			"UserParams": "workspace_id: workspace-1",
		},
	}))
	require.Contains(t, rendered.String(), "workspace_id: workspace-1")
}

func TestAuthorizationForgeRestrictsGenericFilesystemFallbacks(t *testing.T) {
	service := &fakeAuthorizationService{
		available: true,
		workspace: testAuthorizationWorkspace(),
	}
	tools, err := buildTools(service, "workspace-1")
	require.NoError(t, err)
	config := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithSystemFileOperator(),
		aicommon.WithTools(tools...),
		authorizationToolScopeOption(tools),
	)
	enabled, err := config.GetAiToolManager().GetEnableTools()
	require.NoError(t, err)
	names := make([]string, 0, len(enabled))
	for _, tool := range enabled {
		names = append(names, tool.Name)
	}
	require.Len(t, names, 14)
	require.Contains(t, names, "authorization.evidence.inspect")
	require.Contains(t, names, "authorization.evidence.validate")
	require.Contains(t, names, "authorization.review.submit")
	require.Contains(t, names, "authorization.verdict.reconcile")
	require.NotContains(t, names, "authorization.result.inspect")
	require.NotContains(t, names, "bash")
	require.NotContains(t, names, "read_file")
	require.NotContains(t, names, "tools_search")
}

func TestAuthorizationRuntimeReActBindsPromptAndOnlyWorkspaceTools(t *testing.T) {
	service := &fakeAuthorizationService{
		available: true,
		workspace: testAuthorizationWorkspace(),
	}
	preparation, err := NewRunner(service).PrepareReAct(
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "workspace_id", Value: "workspace-1"},
			{Key: "query", Value: "compare the four authorization cases"},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, preparation)

	options := []aicommon.ConfigOption{aicommon.WithSystemFileOperator()}
	options = append(options, preparation.Options...)
	config := aicommon.NewConfig(context.Background(), options...)
	require.Equal(t, ForgeName, config.GetForgeName())
	require.Contains(t, config.PlanPrompt, "workspace-1")
	require.Contains(t, config.PlanPrompt, "Evidence Bundle")
	require.True(t, config.DisallowMCPServers)
	require.True(t, config.IsAutoSkillsDisabled())
	require.True(t, config.DisableIntentRecognition)
	require.True(t, config.DisablePerception)
	require.False(t, config.EnableDispatchSubReactAgents)

	enabled, err := config.GetAiToolManager().GetEnableTools()
	require.NoError(t, err)
	names := make([]string, 0, len(enabled))
	for _, tool := range enabled {
		names = append(names, tool.Name)
	}
	require.Len(t, names, 14)
	require.Contains(t, names, "authorization.evidence.inspect")
	require.Contains(t, names, "authorization.evidence.diff")
	require.Contains(t, names, "authorization.review.submit")
	require.Contains(t, names, "authorization.verdict.reconcile")
	require.NotContains(t, names, "ls")
	require.NotContains(t, names, "read_file")
	require.NotContains(t, names, "tools_search")
}

func TestParseBrowserAuthorizationConfig(t *testing.T) {
	config, err := ParseConfig([]*ypb.ExecParamItem{
		{Key: "workspace-id", Value: "workspace-1"},
		{Key: "query", Value: "inspect the matrix"},
	})
	require.NoError(t, err)
	require.Equal(t, "workspace-1", config.WorkspaceID)
	require.Equal(t, "inspect the matrix", config.Query)

	_, err = ParseConfig(nil)
	require.ErrorContains(t, err, "workspace_id")
}

func TestAuthorizationToolsBindWorkspaceAndOnlyExecuteVerifiedPlan(t *testing.T) {
	service := &fakeAuthorizationService{
		available: true,
		workspace: testAuthorizationWorkspace(),
	}
	tools, err := buildTools(service, "workspace-1")
	require.NoError(t, err)
	require.Len(t, tools, 14)
	for _, tool := range tools {
		require.False(t, tool.NoNeedUserReview, tool.Name)
	}

	inspectResult, err := toolByName(
		t,
		tools,
		"authorization.workspace.inspect",
	).InvokeWithParams(map[string]interface{}{})
	require.NoError(t, err)
	view, ok := toolExecutionResult(t, inspectResult).(workspaceView)
	require.True(t, ok)
	require.Equal(t, "workspace-1", view.ID)
	require.Equal(t, "handle", view.Left.ContextKind)
	encoded, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret-left-handle")
	require.NotContains(t, string(encoded), "secret-right-attestation")
	require.NotContains(t, string(encoded), "secret-baseline-handle")
	require.NotContains(t, string(encoded), "secret-network-request-id")

	profileResult, err := toolByName(
		t,
		tools,
		"authorization.transform.profiles",
	).InvokeWithParams(map[string]interface{}{})
	require.NoError(t, err)
	profiles, ok := toolExecutionResult(t, profileResult).(browser.ExtensionAuthorizationTransformProfileCandidates)
	require.True(t, ok)
	require.Equal(t, "profile-left", profiles.Left[0].ID)
	require.Equal(t, "profile-right", profiles.Right[0].ID)
	require.Equal(t, []string{"workspace-1"}, service.transformLists)
	profileJSON, err := json.Marshal(profiles)
	require.NoError(t, err)
	require.NotContains(t, string(profileJSON), "secret")

	logicalResult, err := toolByName(
		t,
		tools,
		"authorization.logical.bind",
	).InvokeWithParams(map[string]interface{}{
		"leftProfileId":  "profile-left",
		"rightProfileId": "profile-right",
	})
	require.NoError(t, err)
	_, ok = toolExecutionResult(t, logicalResult).(workspaceView)
	require.True(t, ok)
	require.Equal(t, []browser.ExtensionAuthorizationLogicalBindingInput{{
		WorkspaceID: "workspace-1",
		TransformProfiles: browser.ExtensionAuthorizationTransformProfileInput{
			Left:  "profile-left",
			Right: "profile-right",
		},
	}}, service.logicalBindings)
	require.NotContains(t, string(encoded), "secret-device-id")
	require.NotContains(t, string(encoded), "secret-installation-id")
	require.NotContains(t, string(encoded), "secret-grant-id")

	partialProfileResult, err := toolByName(
		t,
		tools,
		"authorization.plan.propose",
	).InvokeWithParams(map[string]interface{}{
		"candidateId":   "candidate-1",
		"leftProfileId": "profile-left",
	})
	require.ErrorContains(t, err, "must be supplied together")
	require.NotNil(t, partialProfileResult)
	require.False(t, partialProfileResult.Success)
	require.Empty(t, service.plans)

	proposeResult, err := toolByName(
		t,
		tools,
		"authorization.plan.propose",
	).InvokeWithParams(map[string]interface{}{
		"candidateId":    "candidate-1",
		"leftProfileId":  "profile-left",
		"rightProfileId": "profile-right",
	})
	require.NoError(t, err)
	proposed, ok := toolExecutionResult(t, proposeResult).(*browser.ExtensionAuthorizationPlan)
	require.True(t, ok)
	require.Equal(t, "plan-1", proposed.ID)
	require.Equal(t, []browser.ExtensionAuthorizationPlanInput{{
		WorkspaceID: "workspace-1",
		CandidateID: "candidate-1",
		TransformProfiles: &browser.ExtensionAuthorizationTransformProfileInput{
			Left:  "profile-left",
			Right: "profile-right",
		},
	}}, service.plans)

	validateResult, err := toolByName(
		t,
		tools,
		"authorization.plan.validate",
	).InvokeWithParams(map[string]interface{}{"planId": "plan-1"})
	require.NoError(t, err)
	validation, ok := toolExecutionResult(t, validateResult).(planValidation)
	require.True(t, ok)
	require.True(t, validation.Valid)
	require.Equal(t, 4, validation.RequestBudget)

	invalidResult, err := toolByName(
		t,
		tools,
		"authorization.plan.execute",
	).InvokeWithParams(map[string]interface{}{"planId": "invented-plan"})
	require.ErrorContains(t, err, "planId does not match")
	require.NotNil(t, invalidResult)
	require.False(t, invalidResult.Success)
	require.Empty(t, service.executions)

	executeResult, err := toolByName(
		t,
		tools,
		"authorization.plan.execute",
	).InvokeWithParams(map[string]interface{}{"planId": "plan-1"})
	require.NoError(t, err)
	receipt, ok := toolExecutionResult(t, executeResult).(authorizationExecutionReceipt)
	require.True(t, ok)
	require.Equal(t, "execution-1", receipt.Execution.ID)
	require.Equal(t, "authorization.review.begin", receipt.NextTool)
	receiptJSON, err := json.Marshal(receipt)
	require.NoError(t, err)
	require.NotContains(t, string(receiptJSON), "verdict")
	require.NotContains(t, string(receiptJSON), "confidence")
	require.NotContains(t, string(receiptJSON), "both cross-identity requests")
	require.Equal(t, []browser.ExtensionAuthorizationExecutionInput{{
		WorkspaceID: "workspace-1",
		PlanID:      "plan-1",
	}}, service.executions)
	postExecutionInspect, err := toolByName(
		t,
		tools,
		"authorization.workspace.inspect",
	).InvokeWithParams(map[string]interface{}{})
	require.NoError(t, err)
	postExecutionView, ok := toolExecutionResult(t, postExecutionInspect).(workspaceView)
	require.True(t, ok)
	require.NotNil(t, postExecutionView.Execution)
	postExecutionJSON, err := json.Marshal(postExecutionView)
	require.NoError(t, err)
	require.NotContains(t, string(postExecutionJSON), `"verdict"`)
	require.NotContains(t, string(postExecutionJSON), "both cross-identity requests")
	postExecutionReferenceJSON, err := json.Marshal(postExecutionView.Execution)
	require.NoError(t, err)
	require.NotContains(t, string(postExecutionReferenceJSON), `"confidence"`)
	require.NotContains(t, string(postExecutionReferenceJSON), `"reasons"`)

	evidenceBeforeReview, err := toolByName(
		t,
		tools,
		"authorization.evidence.inspect",
	).InvokeWithParams(map[string]interface{}{
		"executionId": "execution-1",
	})
	require.ErrorContains(t, err, "authorization.review.begin")
	require.False(t, evidenceBeforeReview.Success)

	beginResult, err := toolByName(
		t,
		tools,
		"authorization.review.begin",
	).InvokeWithParams(map[string]interface{}{
		"executionId": "execution-1",
	})
	require.NoError(t, err)
	blindContext, ok := toolExecutionResult(t, beginResult).(authorizationBlindReviewContext)
	require.True(t, ok)
	require.Equal(t, authorizationReviewPhaseBlind, blindContext.Phase)
	blindContextJSON, err := json.Marshal(blindContext)
	require.NoError(t, err)
	require.NotContains(t, string(blindContextJSON), "verdict")
	require.NotContains(t, string(blindContextJSON), "confidence")

	reconcileBeforeSubmit, err := toolByName(
		t,
		tools,
		"authorization.verdict.reconcile",
	).InvokeWithParams(map[string]interface{}{
		"executionId": "execution-1",
	})
	require.ErrorContains(t, err, "submit an independent blind assessment")
	require.False(t, reconcileBeforeSubmit.Success)

	evidenceResult, err := toolByName(
		t,
		tools,
		"authorization.evidence.inspect",
	).InvokeWithParams(map[string]interface{}{
		"executionId": "execution-1",
	})
	require.NoError(t, err)
	bundle, ok := toolExecutionResult(t, evidenceResult).(authorizationBlindEvidenceBundle)
	require.True(t, ok)
	require.Equal(t, "execution-1", bundle.ExecutionID)
	bundleJSON, err := json.Marshal(bundle)
	require.NoError(t, err)
	require.NotContains(t, string(bundleJSON), "verdict")
	require.NotContains(t, string(bundleJSON), "confidence")
	require.NotContains(t, string(bundleJSON), "semantic")

	unobservedValidation, err := toolByName(
		t,
		tools,
		"authorization.evidence.validate",
	).InvokeWithParams(map[string]interface{}{
		"executionId": "execution-1",
		"direction":   "a-to-b",
		"paths":       []interface{}{"body.not.observed"},
	})
	require.ErrorContains(t, err, "was not returned by authorization.evidence.diff")
	require.False(t, unobservedValidation.Success)

	diffResult, err := toolByName(
		t,
		tools,
		"authorization.evidence.diff",
	).InvokeWithParams(map[string]interface{}{
		"executionId": "execution-1",
		"leftCaseId":  "a-to-b",
		"rightCaseId": "b-own",
	})
	require.NoError(t, err)
	diff, ok := toolExecutionResult(t, diffResult).(browser.ExtensionAuthorizationEvidenceDiff)
	require.True(t, ok)
	require.Equal(t, "response", diff.Scope)
	require.Equal(t, "redacted", diff.View)

	validationResult, err := toolByName(
		t,
		tools,
		"authorization.evidence.validate",
	).InvokeWithParams(map[string]interface{}{
		"executionId": "execution-1",
		"direction":   "a-to-b",
		"paths":       []interface{}{"body.user.id"},
	})
	require.NoError(t, err)
	evidenceValidation, ok := toolExecutionResult(t, validationResult).(authorizationBlindEvidenceValidation)
	require.True(t, ok)
	require.True(t, evidenceValidation.Verified)
	validationJSON, err := json.Marshal(evidenceValidation)
	require.NoError(t, err)
	require.NotContains(t, string(validationJSON), "verdict")
	require.NotContains(t, string(validationJSON), "confidence")
	require.NotContains(t, string(validationJSON), "fingerprint")

	packetResult, err := toolByName(
		t,
		tools,
		"authorization.evidence.packet",
	).InvokeWithParams(map[string]interface{}{
		"executionId": "execution-1",
		"caseId":      "a-to-b",
		"side":        "response",
	})
	require.NoError(t, err)
	packet, ok := toolExecutionResult(t, packetResult).(browser.ExtensionAuthorizationEvidencePacket)
	require.True(t, ok)
	require.Equal(t, "redacted", packet.View)

	submitResult, err := toolByName(
		t,
		tools,
		"authorization.review.submit",
	).InvokeWithParams(map[string]interface{}{
		"executionId":          "execution-1",
		"aToBObservation":      "access-denied",
		"bToAObservation":      "access-denied",
		"identityRelationship": "not-proven",
		"policyAssessment":     "protected",
		"policyBasis":          "none",
		"evidencePaths":        []interface{}{"body.user.id"},
		"limitations":          []interface{}{"account labels do not prove role equivalence"},
		"summary":              "Both bounded cross-identity requests were denied.",
	})
	require.NoError(t, err)
	commit, ok := toolExecutionResult(t, submitResult).(authorizationReviewCommit)
	require.True(t, ok)
	require.Equal(t, authorizationReviewPhaseSubmitted, commit.Phase)
	require.NotEmpty(t, commit.CommitFingerprint)

	resubmitResult, err := toolByName(
		t,
		tools,
		"authorization.review.submit",
	).InvokeWithParams(map[string]interface{}{
		"executionId":          "execution-1",
		"aToBObservation":      "target-data-reproduced",
		"bToAObservation":      "target-data-reproduced",
		"identityRelationship": "same-privilege-proven",
		"policyAssessment":     "violation-supported",
		"policyBasis":          "equivalent-privileges",
		"summary":              "Attempted rewrite.",
	})
	require.ErrorContains(t, err, "immutable")
	require.False(t, resubmitResult.Success)

	reconcileResult, err := toolByName(
		t,
		tools,
		"authorization.verdict.reconcile",
	).InvokeWithParams(map[string]interface{}{
		"executionId": "execution-1",
	})
	require.NoError(t, err)
	reconciliation, ok := toolExecutionResult(t, reconcileResult).(authorizationVerdictReconciliation)
	require.True(t, ok)
	require.Equal(t, "protected", reconciliation.Deterministic.EvidenceGrade)
	require.Equal(t, "cross-identity-probes-blocked", reconciliation.Deterministic.Observation)
	require.Equal(t, "not-evaluated-by-deterministic-engine", reconciliation.Deterministic.PolicyConclusion)
	require.Equal(t, "agree", reconciliation.FactAgreement)

	reportResult, err := toolByName(
		t,
		tools,
		"authorization.report.build",
	).InvokeWithParams(map[string]interface{}{
		"planId":      "plan-1",
		"executionId": "execution-1",
	})
	require.NoError(t, err)
	report, ok := toolExecutionResult(t, reportResult).(authorizationReport)
	require.True(t, ok)
	require.Equal(t, "protected", report.Deterministic.EvidenceGrade)
	require.Equal(t, "protected", report.IndependentReview.Assessment.PolicyAssessment)
	require.Equal(t, 4, report.RequestCount)
	require.NotEmpty(t, report.NextActions)

	for _, inspectedID := range service.inspectIDs {
		require.Equal(t, "workspace-1", inspectedID)
	}
}

func TestRunnerRequiresLiveAuthorizationServiceBeforeStartingAI(t *testing.T) {
	runner := &Runner{service: &fakeAuthorizationService{}}
	result, err := runner.Execute(context.Background(), []*ypb.ExecParamItem{
		{Key: "workspace_id", Value: "workspace-1"},
	})
	require.ErrorContains(t, err, "workspace service is not running")
	require.Nil(t, result)
}

func TestPlanProposalRejectsWireBodyCandidateRequiringLogicalBinding(t *testing.T) {
	workspace := testAuthorizationWorkspace()
	workspace.BaselinePair.ResourceCandidates[0].Source = "wire"
	workspace.BaselinePair.ResourceCandidates[0].Location = "body"
	workspace.BaselinePair.ResourceCandidates[0].Path = "body.encryptedData"
	workspace.BaselinePair.ResourceCandidates[0].RequiresLogicalBinding = true
	service := &fakeAuthorizationService{
		available: true,
		workspace: workspace,
	}
	tools, err := buildTools(service, workspace.ID)
	require.NoError(t, err)

	result, err := toolByName(
		t,
		tools,
		"authorization.plan.propose",
	).InvokeWithParams(map[string]interface{}{"candidateId": "candidate-1"})

	require.ErrorContains(t, err, "requires a verified logical plaintext binding")
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Empty(t, service.plans)
}

func TestPlanValidationFailsClosedForDynamicRebuild(t *testing.T) {
	workspace := testAuthorizationWorkspace()
	workspace.Plan.RequiresDynamicRebuild = true
	workspace.Plan.State = "blocked"
	validation := validatePlanView(workspace, workspace.Plan)
	require.False(t, validation.Valid)
	require.True(t, strings.Contains(strings.Join(validation.Reasons, " "), "dynamic"))
}

func TestVerticalAuthorizationPlanUsesOperationCandidateAndThreeCaseBudget(t *testing.T) {
	workspace := testAuthorizationWorkspace()
	workspace.Mode = "vertical"
	workspace.BaselinePair.ResourceCandidates = nil
	workspace.BaselinePair.OperationCandidates = []browser.ExtensionAuthorizationOperationCandidate{{
		ID:                  "candidate-1",
		TemplateSide:        "right",
		AuthContextSide:     "left",
		LowControlSide:      "left",
		Method:              "GET",
		Path:                "/admin/export",
		ActionFingerprint:   "sha256:privileged-operation",
		Eligible:            true,
		AuthenticationPaths: []string{"header.authorization"},
	}}
	workspace.Plan = &browser.ExtensionAuthorizationPlan{
		Version:     1,
		ID:          "plan-vertical",
		WorkspaceID: workspace.ID,
		Mode:        "vertical",
		CandidateID: "candidate-1",
		State:       "ready",
		Selector: browser.ExtensionAuthorizationPlanSelector{
			Source:   "operation",
			Location: "request",
			Path:     "right",
		},
		Operation: &browser.ExtensionAuthorizationPlanOperation{
			TemplateBaselineSide: "right",
			AuthContextSide:      "left",
			LowControlSide:       "left",
			AuthenticationPaths:  []string{"header.authorization"},
		},
		Cases: []browser.ExtensionAuthorizationPlanCase{
			{ID: "low-control"},
			{ID: "privileged-baseline"},
			{ID: "low-privileged-probe"},
		},
		RequestBudget: 3,
		ExpiresAt:     workspace.ExpiresAt,
	}

	validation := validatePlanView(workspace, workspace.Plan)

	require.True(t, validation.Valid)
	require.Equal(t, "vertical", validation.Mode)
	require.Equal(t, 3, validation.RequestBudget)

	service := &fakeAuthorizationService{available: true, workspace: workspace}
	tools, err := buildTools(service, workspace.ID)
	require.NoError(t, err)
	result, err := toolByName(
		t,
		tools,
		"authorization.plan.propose",
	).InvokeWithParams(map[string]interface{}{"candidateId": "candidate-1"})
	require.NoError(t, err)
	proposed, ok := toolExecutionResult(t, result).(*browser.ExtensionAuthorizationPlan)
	require.True(t, ok)
	require.Equal(t, "vertical", proposed.Mode)
}

func TestVerticalAuthorizationPlanAcceptsPinnedPostStateTopology(t *testing.T) {
	workspace := testAuthorizationWorkspace()
	workspace.Mode = "vertical"
	workspace.BaselinePair.ResourceCandidates = nil
	workspace.BaselinePair.OperationCandidates = []browser.ExtensionAuthorizationOperationCandidate{{
		ID:                  "candidate-1",
		TemplateSide:        "right",
		AuthContextSide:     "left",
		LowControlSide:      "left",
		Eligible:            true,
		AuthenticationPaths: []string{"header.authorization"},
	}}
	verification := *workspace.Baselines.Left
	verification.ID = "verification-1"
	workspace.Baselines.Verification = &verification
	workspace.Plan = &browser.ExtensionAuthorizationPlan{
		ID:          "plan-vertical-post-state",
		WorkspaceID: workspace.ID,
		Mode:        "vertical",
		CandidateID: "candidate-1",
		State:       "ready",
		Selector: browser.ExtensionAuthorizationPlanSelector{
			Source: "operation", Location: "request", Path: "right",
		},
		Operation: &browser.ExtensionAuthorizationPlanOperation{
			TemplateBaselineSide:   "right",
			AuthContextSide:        "left",
			LowControlSide:         "left",
			AuthenticationPaths:    []string{"header.authorization"},
			VerificationBaselineID: verification.ID,
		},
		Cases: []browser.ExtensionAuthorizationPlanCase{
			{ID: "low-control"},
			{ID: "privileged-baseline"},
			{ID: "post-state-before"},
			{ID: "low-privileged-probe"},
			{ID: "post-state-after"},
		},
		RequestBudget: 5,
		ExpiresAt:     workspace.ExpiresAt,
	}

	validation := validatePlanView(workspace, workspace.Plan)

	require.True(t, validation.Valid)
	require.Equal(t, 5, validation.RequestBudget)
}

func TestVerticalAuthorizationPlanAcceptsOneLowIdentityTransformProfile(t *testing.T) {
	workspace := testAuthorizationWorkspace()
	workspace.Mode = "vertical"
	workspace.BaselinePair.ResourceCandidates = nil
	workspace.BaselinePair.OperationCandidates = []browser.ExtensionAuthorizationOperationCandidate{{
		ID:                     "candidate-1",
		Eligible:               true,
		RequiresDynamicRebuild: true,
	}}
	service := &fakeAuthorizationService{available: true, workspace: workspace}
	tools, err := buildTools(service, workspace.ID)
	require.NoError(t, err)

	result, err := toolByName(
		t,
		tools,
		"authorization.plan.propose",
	).InvokeWithParams(map[string]interface{}{
		"candidateId":   "candidate-1",
		"leftProfileId": "profile-left",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, "profile-left", service.plans[0].OperationTransformProfileID)
	require.Nil(t, service.plans[0].TransformProfiles)
}
