package browserauthorization

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/browser"
)

func TestBuildAuthorizationReportSeparatesFactsFromPolicy(t *testing.T) {
	workspace := browser.ExtensionAuthorizationWorkspace{
		ID:   "workspace-vertical",
		Mode: "vertical",
		Execution: &browser.ExtensionAuthorizationExecution{
			ID:           "execution-vertical",
			PlanID:       "plan-vertical",
			Verdict:      "confirmed",
			Confidence:   "high",
			RequestCount: 5,
			Cases: []browser.ExtensionAuthorizationCaseExecution{{
				ID:    "low-privileged-probe",
				State: "completed",
				Result: &browser.ExtensionAuthorizationRequestExecution{
					URL:                "https://private.example.test/admin",
					Status:             200,
					Outcome:            "success",
					DroppedHeaderNames: []string{"authorization"},
				},
			}},
			Evidence: []browser.ExtensionAuthorizationCanaryEvidence{{
				Direction:        "low-to-privileged",
				Path:             "body.revision",
				Source:           "vertical-post-state-json-differential",
				ValueFingerprint: "secret-fingerprint",
			}},
		},
	}
	reconciliation := authorizationVerdictReconciliation{
		WorkspaceID: workspace.ID,
		ExecutionID: workspace.Execution.ID,
		Mode:        workspace.Mode,
		IndependentReview: authorizationReviewCommit{
			Assessment: authorizationIndependentAssessment{
				Mode:                       "vertical",
				LowToPrivilegedObservation: "state-change-confirmed",
				IdentityRelationship:       "not-applicable",
				PolicyAssessment:           "requires-policy",
				PolicyBasis:                "none",
				Summary:                    "The operation changed state; intended policy is not available.",
			},
		},
		Deterministic: authorizationDeterministicFinding{
			Observation:   "low-identity-operation-state-change-confirmed",
			EvidenceGrade: "confirmed",
			Confidence:    "high",
			RequestCount:  5,
			Evidence: []authorizationReviewEvidence{{
				Direction: "low-to-privileged",
				Path:      "body.revision",
				Source:    "vertical-post-state-json-differential",
			}},
			PolicyConclusion: "not-evaluated-by-deterministic-engine",
		},
		FactAgreement:  "agree",
		PolicyBoundary: "facts and policy remain separate",
	}

	report := buildAuthorizationReport(workspace, reconciliation)

	require.Equal(t, "confirmed", report.Deterministic.EvidenceGrade)
	require.Equal(t, "requires-policy", report.IndependentReview.Assessment.PolicyAssessment)
	require.Contains(t, report.Summary, "状态发生变化")
	require.Equal(t, "body.revision", report.Evidence[0].Path)
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private.example.test")
	require.NotContains(t, string(encoded), "secret-fingerprint")
	require.NotContains(t, string(encoded), "authorization")
	require.NotContains(t, string(encoded), `"verdict"`)
}

func TestBuildAuthorizationReportDoesNotNameLikelyEvidenceAsVulnerability(t *testing.T) {
	workspace := browser.ExtensionAuthorizationWorkspace{
		ID:   "workspace-horizontal",
		Mode: "horizontal",
		Execution: &browser.ExtensionAuthorizationExecution{
			ID:         "execution-horizontal",
			PlanID:     "plan-horizontal",
			Verdict:    "likely",
			Confidence: "high",
		},
	}
	reconciliation := authorizationVerdictReconciliation{
		IndependentReview: authorizationReviewCommit{
			Assessment: authorizationIndependentAssessment{
				Mode:                 "horizontal",
				AToBObservation:      "target-response-matched",
				BToAObservation:      "access-denied",
				IdentityRelationship: "different-privilege",
				PolicyAssessment:     "expected-access-plausible",
				PolicyBasis:          "role-hierarchy",
				Summary:              "The asymmetric access may follow the role hierarchy.",
			},
		},
		Deterministic: authorizationDeterministicFinding{
			Observation:      "cross-identity-response-match-observed",
			EvidenceGrade:    "likely",
			Confidence:       "high",
			PolicyConclusion: "not-evaluated-by-deterministic-engine",
		},
		FactAgreement: "agree",
	}

	report := buildAuthorizationReport(workspace, reconciliation)

	require.Equal(t, "likely", report.Deterministic.EvidenceGrade)
	require.Equal(t, "expected-access-plausible", report.IndependentReview.Assessment.PolicyAssessment)
	require.Contains(t, report.Summary, "响应")
	require.Contains(t, report.Limitations, "尚未证明两个身份权限等价，跨身份访问事实不能自动命名为水平授权缺陷")
	require.NotContains(t, report.Summary, "越权")
}
