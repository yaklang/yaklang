package browserauthorization

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/browser"
)

type Service interface {
	Available() bool
	InspectWorkspace(
		ctx context.Context,
		workspaceID string,
		revalidate bool,
	) (browser.ExtensionAuthorizationWorkspace, error)
	CreatePlan(
		ctx context.Context,
		input browser.ExtensionAuthorizationPlanInput,
	) (browser.ExtensionAuthorizationWorkspace, error)
	BindLogicalRequests(
		ctx context.Context,
		input browser.ExtensionAuthorizationLogicalBindingInput,
	) (browser.ExtensionAuthorizationWorkspace, error)
	ListTransformProfiles(
		ctx context.Context,
		workspaceID string,
	) (browser.ExtensionAuthorizationTransformProfileCandidates, error)
	ExecutePlan(
		ctx context.Context,
		input browser.ExtensionAuthorizationExecutionInput,
	) (browser.ExtensionAuthorizationWorkspace, error)
	InspectEvidence(
		ctx context.Context,
		input browser.ExtensionAuthorizationEvidenceInspectInput,
	) (browser.ExtensionAuthorizationEvidenceBundle, error)
	ReadEvidencePacket(
		ctx context.Context,
		input browser.ExtensionAuthorizationEvidencePacketInput,
	) (browser.ExtensionAuthorizationEvidencePacket, error)
	DiffEvidence(
		ctx context.Context,
		input browser.ExtensionAuthorizationEvidenceDiffInput,
	) (browser.ExtensionAuthorizationEvidenceDiff, error)
	ValidateEvidence(
		ctx context.Context,
		input browser.ExtensionAuthorizationEvidenceValidationInput,
	) (browser.ExtensionAuthorizationEvidenceValidation, error)
}

type identityView struct {
	Side               string                                       `json:"side"`
	AccountLabel       string                                       `json:"accountLabel,omitempty"`
	Origin             string                                       `json:"origin"`
	IsolationContextID string                                       `json:"isolationContextId"`
	ContextKind        string                                       `json:"contextKind"`
	Authentication     browser.ExtensionAuthorizationAuthentication `json:"authentication"`
	ExpiresAt          int64                                        `json:"expiresAt"`
}

type baselineView struct {
	Version        int                                                  `json:"version"`
	ID             string                                               `json:"id"`
	Side           string                                               `json:"side"`
	Origin         string                                               `json:"origin"`
	ContextKind    string                                               `json:"contextKind"`
	Request        browser.ExtensionAuthorizationBaselineRequest        `json:"request"`
	LogicalRequest *browser.ExtensionAuthorizationLogicalRequestBinding `json:"logicalRequest,omitempty"`
	CreatedAt      int64                                                `json:"createdAt"`
	ExpiresAt      int64                                                `json:"expiresAt"`
}

type baselineSetView struct {
	Left         *baselineView `json:"left,omitempty"`
	Right        *baselineView `json:"right,omitempty"`
	Verification *baselineView `json:"verification,omitempty"`
}

type isolationFactsView struct {
	SameOrigin                bool   `json:"sameOrigin"`
	CookieStoreRelation       string `json:"cookieStoreRelation"`
	AccountEvidenceRelation   string `json:"accountEvidenceRelation"`
	RequestCredentialRelation string `json:"requestCredentialRelation"`
	RefreshCheck              string `json:"refreshCheck"`
	ExpiresAt                 int64  `json:"expiresAt"`
}

type workspaceView struct {
	Version      int                                        `json:"version"`
	ID           string                                     `json:"id"`
	Mode         string                                     `json:"mode"`
	State        string                                     `json:"state"`
	Left         identityView                               `json:"left"`
	Right        identityView                               `json:"right"`
	Isolation    isolationFactsView                         `json:"isolation"`
	Baselines    baselineSetView                            `json:"baselines"`
	BaselinePair browser.ExtensionAuthorizationBaselinePair `json:"baselinePair"`
	Plan         *browser.ExtensionAuthorizationPlan        `json:"plan,omitempty"`
	Execution    *authorizationExecutionReference           `json:"execution,omitempty"`
	CreatedAt    int64                                      `json:"createdAt"`
	ExpiresAt    int64                                      `json:"expiresAt"`
	StaleReason  string                                     `json:"staleReason,omitempty"`
	Recovery     *browser.ExtensionAuthorizationRecovery    `json:"recovery,omitempty"`
}

type planValidation struct {
	Valid                  bool                                       `json:"valid"`
	WorkspaceID            string                                     `json:"workspaceId"`
	WorkspaceState         string                                     `json:"workspaceState"`
	PlanID                 string                                     `json:"planId"`
	Mode                   string                                     `json:"mode"`
	CandidateID            string                                     `json:"candidateId"`
	PlanState              string                                     `json:"planState"`
	Selector               browser.ExtensionAuthorizationPlanSelector `json:"selector"`
	RequestBudget          int                                        `json:"requestBudget"`
	RequiresDynamicRebuild bool                                       `json:"requiresDynamicRebuild"`
	Reasons                []string                                   `json:"reasons"`
	ExpiresAt              int64                                      `json:"expiresAt"`
}

func identitySummary(slot browser.ExtensionAuthorizationIdentitySlot) identityView {
	return identityView{
		Side:               slot.Side,
		AccountLabel:       slot.AccountLabel,
		Origin:             slot.Origin,
		IsolationContextID: slot.IsolationContextID,
		ContextKind:        slot.ContextReference.Kind,
		Authentication:     slot.Authentication,
		ExpiresAt:          slot.ExpiresAt,
	}
}

func baselineSummary(
	side string,
	baseline *browser.ExtensionAuthorizationBaseline,
) *baselineView {
	if baseline == nil {
		return nil
	}
	return &baselineView{
		Version:        baseline.Version,
		ID:             baseline.ID,
		Side:           side,
		Origin:         baseline.Origin,
		ContextKind:    baseline.AuthContextReference.Kind,
		Request:        baseline.Request,
		LogicalRequest: baseline.LogicalRequest,
		CreatedAt:      baseline.CreatedAt,
		ExpiresAt:      baseline.ExpiresAt,
	}
}

func newWorkspaceView(workspace browser.ExtensionAuthorizationWorkspace) workspaceView {
	return workspaceView{
		Version: workspace.Version,
		ID:      workspace.ID,
		Mode:    workspace.Mode,
		State:   workspace.State,
		Left:    identitySummary(workspace.Left),
		Right:   identitySummary(workspace.Right),
		Isolation: isolationFactsView{
			SameOrigin:                workspace.Proof.SameOrigin,
			CookieStoreRelation:       workspace.Proof.CookieStoreRelation,
			AccountEvidenceRelation:   workspace.Proof.AccountEvidenceRelation,
			RequestCredentialRelation: workspace.Proof.RequestCredentialRelation,
			RefreshCheck:              workspace.Proof.RefreshCheck,
			ExpiresAt:                 workspace.Proof.ExpiresAt,
		},
		Baselines: baselineSetView{
			Left:         baselineSummary("left", workspace.Baselines.Left),
			Right:        baselineSummary("right", workspace.Baselines.Right),
			Verification: baselineSummary("verification", workspace.Baselines.Verification),
		},
		BaselinePair: workspace.BaselinePair,
		Plan:         workspace.Plan,
		Execution:    authorizationExecutionSummary(workspace.Execution),
		CreatedAt:    workspace.CreatedAt,
		ExpiresAt:    workspace.ExpiresAt,
		StaleReason:  workspace.StaleReason,
		Recovery:     workspace.Recovery,
	}
}

func requiredString(params aitool.InvokeParams, name string) (string, error) {
	value, _ := params[name].(string)
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func inspectBoundWorkspace(
	ctx context.Context,
	service Service,
	workspaceID string,
) (browser.ExtensionAuthorizationWorkspace, error) {
	workspace, err := service.InspectWorkspace(ctx, workspaceID, true)
	if err != nil {
		return browser.ExtensionAuthorizationWorkspace{}, err
	}
	if workspace.ID != workspaceID {
		return browser.ExtensionAuthorizationWorkspace{}, errors.New("authorization service returned a different workspace")
	}
	return workspace, nil
}

func validatedBoundPlan(
	ctx context.Context,
	service Service,
	workspaceID string,
	planID string,
) (browser.ExtensionAuthorizationWorkspace, *browser.ExtensionAuthorizationPlan, error) {
	workspace, err := inspectBoundWorkspace(ctx, service, workspaceID)
	if err != nil {
		return browser.ExtensionAuthorizationWorkspace{}, nil, err
	}
	if workspace.Plan == nil || workspace.Plan.ID != planID {
		return browser.ExtensionAuthorizationWorkspace{}, nil, errors.New("planId does not match the bound workspace")
	}
	return workspace, workspace.Plan, nil
}

func validatePlanView(
	workspace browser.ExtensionAuthorizationWorkspace,
	plan *browser.ExtensionAuthorizationPlan,
) planValidation {
	reasons := append([]string{}, plan.Reasons...)
	workspaceMode := workspace.Mode
	if workspaceMode == "" {
		workspaceMode = "horizontal"
	}
	planMode := plan.Mode
	if planMode == "" {
		planMode = "horizontal"
	}
	modeMatches := workspaceMode == planMode
	workspaceReady := workspace.State == "ready" || workspace.State == "conditional"
	isolationEligible := workspace.Proof.Level == "strong" ||
		(workspace.State == "conditional" &&
			workspace.Proof.Level == "conditional" &&
			workspace.Proof.RefreshCheck == "passed" &&
			workspace.Proof.RequestCredentialRelation == "different")
	candidateFound := false
	candidateRequiresLogicalBinding := false
	candidateEligible := true
	candidateRequiresDynamicRebuild := false
	topologyValid := false
	if planMode == "vertical" {
		for _, candidate := range workspace.BaselinePair.OperationCandidates {
			if candidate.ID != plan.CandidateID {
				continue
			}
			candidateFound = true
			candidateEligible = candidate.Eligible
			candidateRequiresDynamicRebuild = candidate.RequiresDynamicRebuild &&
				(plan.Operation == nil || plan.Operation.Transform == nil)
			break
		}
		baseTopologyValid := plan.Operation != nil &&
			plan.Selector.Source == "operation" &&
			plan.Selector.Location == "request" &&
			plan.Selector.Path == "right"
		if plan.Operation != nil &&
			plan.Operation.VerificationBaselineID != "" {
			topologyValid = baseTopologyValid &&
				workspace.Baselines.Verification != nil &&
				plan.Operation.VerificationBaselineID ==
					workspace.Baselines.Verification.ID &&
				plan.RequestBudget == 5 &&
				len(plan.Cases) == 5 &&
				plan.Cases[0].ID == "low-control" &&
				plan.Cases[1].ID == "privileged-baseline" &&
				plan.Cases[2].ID == "post-state-before" &&
				plan.Cases[3].ID == "low-privileged-probe" &&
				plan.Cases[4].ID == "post-state-after"
		} else {
			topologyValid = baseTopologyValid &&
				workspace.Baselines.Verification == nil &&
				plan.RequestBudget == 3 &&
				len(plan.Cases) == 3 &&
				plan.Cases[0].ID == "low-control" &&
				plan.Cases[1].ID == "privileged-baseline" &&
				plan.Cases[2].ID == "low-privileged-probe"
		}
	} else if planMode == "horizontal" {
		for _, candidate := range workspace.BaselinePair.ResourceCandidates {
			if candidate.ID == plan.CandidateID {
				candidateFound = true
				candidateRequiresLogicalBinding = candidate.RequiresLogicalBinding
				break
			}
		}
		topologyValid = plan.RequestBudget == 4 &&
			len(plan.Cases) == 4 &&
			plan.Operation == nil &&
			plan.Cases[0].ID == "a-own" &&
			plan.Cases[1].ID == "b-own" &&
			plan.Cases[2].ID == "a-to-b" &&
			plan.Cases[3].ID == "b-to-a" &&
			((plan.Selector.Source == "wire" &&
				(plan.Selector.Location == "header" ||
					plan.Selector.Location == "path" ||
					plan.Selector.Location == "query" ||
					plan.Selector.Location == "body")) ||
				(plan.Selector.Source == "logical" &&
					plan.Selector.Location == "body" &&
					plan.Transforms != nil))
	}
	valid := workspaceReady &&
		modeMatches &&
		isolationEligible &&
		candidateFound &&
		candidateEligible &&
		!candidateRequiresLogicalBinding &&
		!candidateRequiresDynamicRebuild &&
		(plan.State == "ready" || plan.State == "review-required") &&
		!plan.RequiresDynamicRebuild &&
		topologyValid
	if !workspaceReady {
		reasons = append(reasons, "workspace is not ready after real-time revalidation")
	}
	if !modeMatches {
		reasons = append(reasons, "workspace and plan authorization modes do not match")
	}
	if !isolationEligible {
		reasons = append(reasons, "workspace isolation proof is not eligible for deterministic execution")
	}
	if !candidateFound {
		reasons = append(reasons, "plan candidate no longer exists in the current baseline pair")
	}
	if !candidateEligible {
		reasons = append(reasons, "the selected privileged operation template is not eligible")
	}
	if candidateRequiresLogicalBinding {
		reasons = append(reasons, "the selected wire Body candidate requires a verified logical plaintext binding")
	}
	if candidateRequiresDynamicRebuild {
		reasons = append(reasons, "the privileged operation requires low-identity dynamic field reconstruction")
	}
	if !topologyValid {
		reasons = append(reasons, "the fixed authorization case topology or request budget changed")
	}
	if plan.State != "ready" && plan.State != "review-required" {
		reasons = append(reasons, "plan is not eligible for deterministic execution")
	}
	if plan.State == "review-required" {
		reasons = append(reasons, "plan is structurally valid but requires explicit side-effect approval")
	}
	if plan.RequiresDynamicRebuild {
		reasons = append(reasons, "plan requires identity-bound dynamic field reconstruction")
	}
	return planValidation{
		Valid:                  valid,
		WorkspaceID:            workspace.ID,
		WorkspaceState:         workspace.State,
		PlanID:                 plan.ID,
		Mode:                   planMode,
		CandidateID:            plan.CandidateID,
		PlanState:              plan.State,
		Selector:               plan.Selector,
		RequestBudget:          plan.RequestBudget,
		RequiresDynamicRebuild: plan.RequiresDynamicRebuild,
		Reasons:                reasons,
		ExpiresAt:              plan.ExpiresAt,
	}
}

func buildTools(service Service, workspaceID string) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()
	review := newAuthorizationBlindReview(workspaceID)
	register := func(
		name string,
		description string,
		usage string,
		callback aitool.NoRuntimeInvokeCallback,
		options ...aitool.ToolOption,
	) error {
		common := []aitool.ToolOption{
			aitool.WithDescription(description),
			aitool.WithUsage(usage),
			aitool.WithKeywords([]string{
				"browser", "authorization", "access control", "BOLA", "IDOR",
				"浏览器", "越权", "授权差异", "双身份",
			}),
			aitool.WithNoRuntimeCallback(callback),
		}
		return factory.RegisterTool(name, append(common, options...)...)
	}

	if err := register(
		"authorization.workspace.inspect",
		"Revalidate and inspect the bound dual-identity workspace. Returns its horizontal or vertical mode, raw isolation facts, redacted baseline metadata, resource or privileged-operation candidates, the current deterministic plan, and only a neutral prior-execution reference. It never returns raw credentials, response bodies, an evidence grade, or a policy conclusion.",
		"Start here. In horizontal mode, confirm that A/B captured the same normal operation and choose a resource candidate. In vertical mode, confirm that A is a low-privilege control and B is the privileged operation template, then choose an operation candidate. GraphQL baselines expose only operation names and an operation fingerprint.",
		func(
			ctx context.Context,
			_ aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			workspace, err := inspectBoundWorkspace(ctx, service, workspaceID)
			if err != nil {
				return nil, err
			}
			return newWorkspaceView(workspace), nil
		},
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.transform.profiles",
		"List sanitized request Transform Profile candidates from the exact A/B identity documents. Returns only IDs, names, route metadata, declared output paths, eligibility reasons, and versions; it never returns Pipeline inputs, replay drafts, page values, credentials, or ciphertext.",
		"Use this for horizontal dynamic Header/Query reconstruction, encrypted Body binding, or vertical operation rebuilding. Horizontal mode may return one candidate list per identity. Vertical mode returns only identity A candidates matched against identity B's privileged route; choose an exact dynamicFieldsEligible Profile and pass it as leftProfileId only. Do not guess Profile IDs.",
		func(
			ctx context.Context,
			_ aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			return service.ListTransformProfiles(ctx, workspaceID)
		},
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.logical.bind",
		"Ask both identity-bound browser extensions to validate their local plaintext replay drafts against the captured wire baselines, then expose only logical field metadata, HMACs, and binding proofs to the deterministic workspace.",
		"Use this only for horizontal mode with exact logicalBodyEligible profile IDs returned by authorization.transform.profiles. This executes each selected Transform Profile once without calling page fetch; raw plaintext drafts, credentials, and generated ciphertext stay out of the Agent context.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			leftProfileID, err := requiredString(params, "leftProfileId")
			if err != nil {
				return nil, err
			}
			rightProfileID, err := requiredString(params, "rightProfileId")
			if err != nil {
				return nil, err
			}
			workspace, err := service.BindLogicalRequests(
				ctx,
				browser.ExtensionAuthorizationLogicalBindingInput{
					WorkspaceID: workspaceID,
					TransformProfiles: browser.ExtensionAuthorizationTransformProfileInput{
						Left:  leftProfileID,
						Right: rightProfileID,
					},
				},
			)
			if err != nil {
				return nil, err
			}
			return newWorkspaceView(workspace), nil
		},
		aitool.WithStringParam(
			"leftProfileId",
			aitool.WithParam_Description("Exact request Transform Profile ID from identity A's current document"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"rightProfileId",
			aitool.WithParam_Description("Exact request Transform Profile ID from identity B's current document"),
			aitool.WithParam_Required(true),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.plan.propose",
		"Ask the deterministic compiler to create a fixed plan from one exact candidate in the bound workspace. Horizontal mode produces A-own, B-own, A-to-B, and B-to-A. Vertical mode produces three controls/probe cases, or five cases when Yakit has bound an independent read-only post-state verifier. The Agent cannot author selectors, packets, credentials, Pipeline nodes, or case topology.",
		"Choose candidateId only from authorization.workspace.inspect. In horizontal mode, never choose requiresLogicalBinding=true before binding plaintext; eligible dynamic fields may use an exact A/B Profile pair. In vertical mode, choose an operationCandidate. When it requiresDynamicRebuild, call authorization.transform.profiles and pass one exact dynamicFieldsEligible identity-A Profile as leftProfileId; omit rightProfileId.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			candidateID, err := requiredString(params, "candidateId")
			if err != nil {
				return nil, err
			}
			workspace, err := inspectBoundWorkspace(ctx, service, workspaceID)
			if err != nil {
				return nil, err
			}
			candidateFound := false
			candidateEligible := true
			candidateRequiresLogicalBinding := false
			workspaceMode := workspace.Mode
			if workspaceMode == "" {
				workspaceMode = "horizontal"
			}
			if workspaceMode == "vertical" {
				for _, candidate := range workspace.BaselinePair.OperationCandidates {
					if candidate.ID == candidateID {
						candidateFound = true
						candidateEligible = candidate.Eligible
						break
					}
				}
			} else {
				for _, candidate := range workspace.BaselinePair.ResourceCandidates {
					if candidate.ID == candidateID {
						candidateFound = true
						candidateRequiresLogicalBinding = candidate.RequiresLogicalBinding
						break
					}
				}
			}
			if !candidateFound {
				return nil, errors.New("candidateId does not belong to the bound baseline pair")
			}
			if !candidateEligible {
				return nil, errors.New("candidateId is not an eligible privileged operation template")
			}
			if candidateRequiresLogicalBinding {
				return nil, errors.New("candidateId requires a verified logical plaintext binding")
			}
			leftProfileID, _ := params["leftProfileId"].(string)
			rightProfileID, _ := params["rightProfileId"].(string)
			leftProfileID = strings.TrimSpace(leftProfileID)
			rightProfileID = strings.TrimSpace(rightProfileID)
			var transformProfiles *browser.ExtensionAuthorizationTransformProfileInput
			operationTransformProfileID := ""
			if workspaceMode == "vertical" {
				if rightProfileID != "" {
					return nil, errors.New(
						"vertical plans accept only identity A's operation Transform Profile",
					)
				}
				operationTransformProfileID = leftProfileID
			} else if (leftProfileID == "") != (rightProfileID == "") {
				return nil, errors.New(
					"leftProfileId and rightProfileId must be supplied together",
				)
			} else if leftProfileID != "" {
				transformProfiles = &browser.ExtensionAuthorizationTransformProfileInput{
					Left:  leftProfileID,
					Right: rightProfileID,
				}
			}
			workspace, err = service.CreatePlan(ctx, browser.ExtensionAuthorizationPlanInput{
				WorkspaceID:                 workspaceID,
				CandidateID:                 candidateID,
				TransformProfiles:           transformProfiles,
				OperationTransformProfileID: operationTransformProfileID,
			})
			if err != nil {
				return nil, err
			}
			if workspace.Plan == nil || workspace.Plan.CandidateID != candidateID {
				return nil, errors.New("authorization compiler did not return a matching plan")
			}
			return workspace.Plan, nil
		},
		aitool.WithStringParam(
			"candidateId",
			aitool.WithParam_Description("Exact candidate ID returned by authorization.workspace.inspect"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"leftProfileId",
			aitool.WithParam_Description("Optional exact eligible identity-A Transform Profile; pair it with rightProfileId in horizontal mode, or use it alone to rebuild a vertical operation"),
		),
		aitool.WithStringParam(
			"rightProfileId",
			aitool.WithParam_Description("Optional exact eligible request Transform Profile ID for identity B; must be paired with leftProfileId"),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.plan.validate",
		"Deterministically validate that a plan belongs to the bound workspace, survived real-time identity and isolation revalidation, retains its exact mode-specific three- or four-case topology, and is eligible for Yak's HTTP data plane either directly or through explicit side-effect Review.",
		"Use the exact planId returned by authorization.workspace.inspect. A false valid result is an explanation boundary, not permission to construct a replacement request.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			planID, err := requiredString(params, "planId")
			if err != nil {
				return nil, err
			}
			workspace, plan, err := validatedBoundPlan(ctx, service, workspaceID, planID)
			if err != nil {
				return nil, err
			}
			return validatePlanView(workspace, plan), nil
		},
		aitool.WithStringParam(
			"planId",
			aitool.WithParam_Description("Exact verified plan ID returned by authorization.workspace.inspect"),
			aitool.WithParam_Required(true),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.plan.execute",
		"Execute one already-verified authorization plan through Yak's deterministic HTTP data plane. Horizontal mode has four requests. Vertical mode has at most three requests and sends the low-identity privileged-operation probe only after both controls succeed. No retries, redirects, raw packet persistence, or model-authored request mutation are allowed. Non-read-only plans require approveSideEffects=true and still enter the current Review policy.",
		"Call authorization.plan.validate first with the same planId. This call performs real network requests and therefore follows the current Manual, AI, or YOLO Review policy. Set approveSideEffects only when the verified plan state is review-required.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			planID, err := requiredString(params, "planId")
			if err != nil {
				return nil, err
			}
			if _, _, err := validatedBoundPlan(ctx, service, workspaceID, planID); err != nil {
				return nil, err
			}
			workspace, err := service.ExecutePlan(ctx, browser.ExtensionAuthorizationExecutionInput{
				WorkspaceID:        workspaceID,
				PlanID:             planID,
				ApproveSideEffects: params.GetBool("approveSideEffects"),
			})
			if err != nil {
				return nil, err
			}
			if workspace.Execution == nil || workspace.Execution.PlanID != planID {
				return nil, errors.New("authorization execution did not return a matching result")
			}
			review.resetForExecution(workspace.Execution, workspace.Mode)
			return newAuthorizationExecutionReceipt(workspace), nil
		},
		aitool.WithStringParam(
			"planId",
			aitool.WithParam_Description("Exact plan ID validated for the bound workspace"),
			aitool.WithParam_Required(true),
		),
		aitool.WithBoolParam(
			"approveSideEffects",
			aitool.WithParam_Description("Explicitly approve the fixed mode-specific request plan when plan.state is review-required; leave false for read-only plans"),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.review.begin",
		"Begin the server-enforced blind evidence-review phase for one completed execution. The deterministic evidence grade, confidence, reasons, and policy wording remain hidden until authorization.review.submit records an immutable independent assessment.",
		"Call this immediately after authorization.plan.execute, or for the neutral prior-execution reference returned by authorization.workspace.inspect. Then inspect the Evidence Bundle, compare cases, and submit your own fact and policy assessment before requesting reconciliation.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			executionID, err := requiredString(params, "executionId")
			if err != nil {
				return nil, err
			}
			workspace, err := service.InspectWorkspace(ctx, workspaceID, false)
			if err != nil {
				return nil, err
			}
			if workspace.Execution == nil || workspace.Execution.ID != executionID {
				return nil, errors.New("executionId does not match the bound workspace")
			}
			return review.begin(workspace)
		},
		aitool.WithStringParam(
			"executionId",
			aitool.WithParam_Description("Exact neutral execution ID returned by plan.execute or workspace.inspect"),
			aitool.WithParam_Required(true),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.evidence.inspect",
		"Inspect the short-lived Evidence Bundle during the blind phase. Returns cases, packet availability, timing, comparison pairs, representations, and expiry. It deliberately omits the engine evidence grade, confidence, reasons, and preselected semantic evidence.",
		"Call after authorization.review.begin. Choose an authorization comparison such as A-to-B versus B-own before requesting a diff or packet. Reuse the exact executionId and case IDs returned here.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			executionID, err := requiredString(params, "executionId")
			if err != nil {
				return nil, err
			}
			if err := review.requireBlind(executionID); err != nil {
				return nil, err
			}
			workspace, err := service.InspectWorkspace(ctx, workspaceID, false)
			if err != nil {
				return nil, err
			}
			if workspace.Execution == nil || workspace.Execution.ID != executionID {
				return nil, errors.New("executionId does not match the bound workspace")
			}
			bundle, err := service.InspectEvidence(ctx, browser.ExtensionAuthorizationEvidenceInspectInput{
				WorkspaceID: workspaceID,
				ExecutionID: executionID,
			})
			if err != nil {
				return nil, err
			}
			if err := review.recordBundle(executionID); err != nil {
				return nil, err
			}
			return newAuthorizationBlindEvidenceBundle(bundle), nil
		},
		aitool.WithStringParam(
			"executionId",
			aitool.WithParam_Description("Exact execution ID returned for the bound workspace"),
			aitool.WithParam_Required(true),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.evidence.diff",
		"Compute a deterministic, bounded request or response difference between two cases in the Evidence Bundle. Structured JSON, form, and HTML/DOM fields are compared by path; volatile and sensitive fields are marked separately. Redacted values are the default, while raw values remain governed by the session Review policy.",
		"Prefer response scope and redacted view first. Compare each cross-identity response to the target owner's normal response. If that pair is exactly equal and therefore has no changed paths, inspect the A-own versus B-own control comparison to locate candidate ownership paths, then validate those paths in the intended cross direction. Ignore entries marked volatile; do not promote a verdict solely from a model reading.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			executionID, err := requiredString(params, "executionId")
			if err != nil {
				return nil, err
			}
			if err := review.requireBlind(executionID); err != nil {
				return nil, err
			}
			leftCaseID, err := requiredString(params, "leftCaseId")
			if err != nil {
				return nil, err
			}
			rightCaseID, err := requiredString(params, "rightCaseId")
			if err != nil {
				return nil, err
			}
			scope := strings.ToLower(strings.TrimSpace(params.GetString("scope")))
			if scope == "" {
				scope = "response"
			}
			view := strings.ToLower(strings.TrimSpace(params.GetString("view")))
			if view == "" {
				view = "redacted"
			}
			diff, err := service.DiffEvidence(ctx, browser.ExtensionAuthorizationEvidenceDiffInput{
				WorkspaceID: workspaceID, ExecutionID: executionID,
				LeftCaseID: leftCaseID, RightCaseID: rightCaseID,
				Scope: scope, View: view,
			})
			if err != nil {
				return nil, err
			}
			if err := review.recordDiff(executionID, diff); err != nil {
				return nil, err
			}
			return diff, nil
		},
		aitool.WithStringParam(
			"executionId",
			aitool.WithParam_Description("Exact execution ID containing both cases"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"leftCaseId",
			aitool.WithParam_Description("Exact left case ID returned by authorization.evidence.inspect"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"rightCaseId",
			aitool.WithParam_Description("Exact right case ID returned by authorization.evidence.inspect"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"scope",
			aitool.WithParam_Description("Compare request or response packets; response is the default"),
			aitool.WithParam_EnumString("request", "response"),
		),
		aitool.WithStringParam(
			"view",
			aitool.WithParam_Description("Use redacted first; raw exposes exact values under Review"),
			aitool.WithParam_EnumString("redacted", "raw"),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.evidence.validate",
		"Validate one or more response paths proposed from the Evidence Bundle with deterministic cross-identity equality rules. A horizontal path passes only when A/B normal values differ and the cross response exactly reproduces the target identity's value. A vertical operation-response path cannot replace independent post-state proof. Verified paths strengthen the internal evidence classification without exposing its aggregate grade during blind review.",
		"Call only with exact paths returned by authorization.evidence.diff. Use a-to-b or b-to-a for horizontal ownership, low-to-privileged for vertical response context, and post-state only for the bound before/after verifier. During blind review the tool returns verification facts and evidence sources, never raw values, fingerprints, the aggregate evidence grade, or confidence.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			executionID, err := requiredString(params, "executionId")
			if err != nil {
				return nil, err
			}
			direction, err := requiredString(params, "direction")
			if err != nil {
				return nil, err
			}
			paths := params.GetStringSlice("paths")
			if len(paths) == 0 {
				return nil, errors.New("paths are required")
			}
			if err := review.validateExposedPaths(executionID, paths); err != nil {
				return nil, err
			}
			validation, err := service.ValidateEvidence(
				ctx,
				browser.ExtensionAuthorizationEvidenceValidationInput{
					WorkspaceID: workspaceID,
					ExecutionID: executionID,
					Direction:   direction,
					Paths:       paths,
				},
			)
			if err != nil {
				return nil, err
			}
			if err := review.recordValidation(executionID, validation); err != nil {
				return nil, err
			}
			return newAuthorizationBlindEvidenceValidation(validation), nil
		},
		aitool.WithStringParam(
			"executionId",
			aitool.WithParam_Description("Exact execution ID containing the proposed response paths"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"direction",
			aitool.WithParam_Description("Exact ownership or post-state direction being validated"),
			aitool.WithParam_Required(true),
			aitool.WithParam_EnumString(
				"a-to-b",
				"b-to-a",
				"low-to-privileged",
				"post-state",
			),
		),
		aitool.WithStringArrayParam(
			"paths",
			aitool.WithParam_Description("One to sixteen exact body paths returned by the deterministic diff"),
			aitool.WithParam_Required(true),
			aitool.WithParam_MinLength(1),
			aitool.WithParam_MaxLength(16),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.evidence.packet",
		"Read one bounded HTTP request or response packet from the short-lived Evidence Bundle. Redacted mode masks credential headers and structured secret fields. Raw mode exposes the exact captured packet when the user's current Review policy approves it.",
		"Use only when the structured diff lacks enough context or the user asks to inspect the traffic. Start with redacted. Never copy credentials into the report or conversation unless the user explicitly needs the exact raw value.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			executionID, err := requiredString(params, "executionId")
			if err != nil {
				return nil, err
			}
			if err := review.requireBlind(executionID); err != nil {
				return nil, err
			}
			caseID, err := requiredString(params, "caseId")
			if err != nil {
				return nil, err
			}
			side, err := requiredString(params, "side")
			if err != nil {
				return nil, err
			}
			view := strings.ToLower(strings.TrimSpace(params.GetString("view")))
			if view == "" {
				view = "redacted"
			}
			return service.ReadEvidencePacket(ctx, browser.ExtensionAuthorizationEvidencePacketInput{
				WorkspaceID: workspaceID, ExecutionID: executionID,
				CaseID: caseID, Side: side, View: view,
			})
		},
		aitool.WithStringParam(
			"executionId",
			aitool.WithParam_Description("Exact execution ID containing the case"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"caseId",
			aitool.WithParam_Description("Exact case ID returned by authorization.evidence.inspect"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"side",
			aitool.WithParam_Description("Read the request or response packet"),
			aitool.WithParam_Required(true),
			aitool.WithParam_EnumString("request", "response"),
		),
		aitool.WithStringParam(
			"view",
			aitool.WithParam_Description("Use redacted first; raw exposes exact credentials and values under Review"),
			aitool.WithParam_EnumString("redacted", "raw"),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.review.submit",
		"Commit one immutable independent assessment while the deterministic evidence grade is still hidden. The submission separates observed request/response facts, identity or privilege equivalence, and the authorization-policy assessment. Evidence paths must have been returned by a diff or validation in this blind phase; target-data-reproduced and state-change-confirmed additionally require direction-matched deterministic validation.",
		"Submit after inspecting enough evidence. Horizontal mode requires A-to-B and B-to-A observations; vertical mode requires the low-to-privileged observation. Use requires-policy when role, tenant, ownership, or explicit policy evidence is insufficient. The same execution cannot be resubmitted after the deterministic result is revealed.",
		func(
			_ context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			executionID, err := requiredString(params, "executionId")
			if err != nil {
				return nil, err
			}
			identityRelationship, err := requiredString(params, "identityRelationship")
			if err != nil {
				return nil, err
			}
			policyAssessment, err := requiredString(params, "policyAssessment")
			if err != nil {
				return nil, err
			}
			policyBasis, err := requiredString(params, "policyBasis")
			if err != nil {
				return nil, err
			}
			summary, err := requiredString(params, "summary")
			if err != nil {
				return nil, err
			}
			return review.submit(executionID, authorizationIndependentAssessment{
				AToBObservation:            params.GetString("aToBObservation"),
				BToAObservation:            params.GetString("bToAObservation"),
				LowToPrivilegedObservation: params.GetString("lowToPrivilegedObservation"),
				IdentityRelationship:       identityRelationship,
				PolicyAssessment:           policyAssessment,
				PolicyBasis:                policyBasis,
				EvidencePaths:              params.GetStringSlice("evidencePaths"),
				Limitations:                params.GetStringSlice("limitations"),
				Summary:                    summary,
			})
		},
		aitool.WithStringParam(
			"executionId",
			aitool.WithParam_Description("Exact execution ID in the active blind review"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"aToBObservation",
			aitool.WithParam_Description("Horizontal A-auth/B-resource fact; required in horizontal mode"),
			aitool.WithParam_EnumString(
				"target-data-reproduced",
				"target-response-matched",
				"access-denied",
				"response-different",
				"not-executed",
				"unknown",
			),
		),
		aitool.WithStringParam(
			"bToAObservation",
			aitool.WithParam_Description("Horizontal B-auth/A-resource fact; required in horizontal mode"),
			aitool.WithParam_EnumString(
				"target-data-reproduced",
				"target-response-matched",
				"access-denied",
				"response-different",
				"not-executed",
				"unknown",
			),
		),
		aitool.WithStringParam(
			"lowToPrivilegedObservation",
			aitool.WithParam_Description("Vertical low-identity operation fact; required in vertical mode"),
			aitool.WithParam_EnumString(
				"state-change-confirmed",
				"operation-response-accepted",
				"access-denied",
				"not-executed",
				"unknown",
			),
		),
		aitool.WithStringParam(
			"identityRelationship",
			aitool.WithParam_Description("Independent relationship conclusion; account labels alone do not prove it"),
			aitool.WithParam_Required(true),
			aitool.WithParam_EnumString(
				"same-privilege-proven",
				"different-privilege",
				"not-proven",
				"not-applicable",
			),
		),
		aitool.WithStringParam(
			"policyAssessment",
			aitool.WithParam_Description("Independent authorization-policy assessment, separate from observed access facts"),
			aitool.WithParam_Required(true),
			aitool.WithParam_EnumString(
				"violation-supported",
				"expected-access-plausible",
				"protected",
				"requires-policy",
				"inconclusive",
			),
		),
		aitool.WithStringParam(
			"policyBasis",
			aitool.WithParam_Description("Basis for the policy assessment; violation-supported requires explicit policy or proven equivalent privileges"),
			aitool.WithParam_Required(true),
			aitool.WithParam_EnumString(
				"equivalent-privileges",
				"explicit-policy",
				"role-hierarchy",
				"none",
			),
		),
		aitool.WithStringArrayParam(
			"evidencePaths",
			aitool.WithParam_Description("Zero to sixteen exact paths already exposed by diff or validation"),
			aitool.WithParam_MaxLength(16),
		),
		aitool.WithStringArrayParam(
			"limitations",
			aitool.WithParam_Description("Zero to eight concrete limits on the independent assessment"),
			aitool.WithParam_MaxLength(8),
		),
		aitool.WithStringParam(
			"summary",
			aitool.WithParam_Description("Concise evidence-based independent conclusion written before engine-result disclosure"),
			aitool.WithParam_Required(true),
			aitool.WithParam_MaxLength(2000),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.verdict.reconcile",
		"Reveal the deterministic evidence observation only after an immutable independent review exists, then reconcile factual agreement without treating deterministic access evidence as a business-policy conclusion.",
		"Call immediately after authorization.review.submit. Report the independent policy assessment separately from deterministic.evidenceGrade. A factual agreement does not prove the tested access violates the application's intended role, tenant, or ownership policy.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			executionID, err := requiredString(params, "executionId")
			if err != nil {
				return nil, err
			}
			workspace, err := service.InspectWorkspace(ctx, workspaceID, false)
			if err != nil {
				return nil, err
			}
			if workspace.Execution == nil || workspace.Execution.ID != executionID {
				return nil, errors.New("executionId does not match the bound workspace")
			}
			return review.reconcile(workspace)
		},
		aitool.WithStringParam(
			"executionId",
			aitool.WithParam_Description("Exact execution ID from the immutable review commit"),
			aitool.WithParam_Required(true),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"authorization.report.build",
		"Build a redacted report that keeps the independent policy assessment, deterministic access observation, and their reconciliation visibly separate. It excludes URLs, credentials, bodies, raw values, and fingerprints.",
		"Call only after authorization.review.submit and authorization.verdict.reconcile. Never rename a deterministic evidence grade into a vulnerability verdict, and preserve any unresolved policy limitation.",
		func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			planID, err := requiredString(params, "planId")
			if err != nil {
				return nil, err
			}
			executionID, err := requiredString(params, "executionId")
			if err != nil {
				return nil, err
			}
			workspace, _, err := validatedBoundPlan(ctx, service, workspaceID, planID)
			if err != nil {
				return nil, err
			}
			if workspace.Execution == nil ||
				workspace.Execution.ID != executionID ||
				workspace.Execution.PlanID != planID {
				return nil, errors.New("executionId does not match the bound plan")
			}
			reconciliation, err := review.reconcile(workspace)
			if err != nil {
				return nil, err
			}
			return buildAuthorizationReport(workspace, reconciliation), nil
		},
		aitool.WithStringParam(
			"planId",
			aitool.WithParam_Description("Exact verified plan ID"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"executionId",
			aitool.WithParam_Description("Exact execution ID returned for that plan"),
			aitool.WithParam_Required(true),
		),
	); err != nil {
		return nil, err
	}

	return factory.Tools(), nil
}
