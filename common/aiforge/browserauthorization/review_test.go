package browserauthorization

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/browser"
)

func asymmetricRoleWorkspace() browser.ExtensionAuthorizationWorkspace {
	requestResult := func(outcome string, status int, fingerprint string) *browser.ExtensionAuthorizationRequestExecution {
		return &browser.ExtensionAuthorizationRequestExecution{
			Outcome: outcome,
			Status:  status,
			Response: browser.ExtensionAuthorizationResponseSummary{
				AnalysisState:    "available",
				ValueFingerprint: fingerprint,
			},
		}
	}
	return browser.ExtensionAuthorizationWorkspace{
		ID:   "workspace-asymmetric-role",
		Mode: "horizontal",
		Execution: &browser.ExtensionAuthorizationExecution{
			ID:                "execution-asymmetric-role",
			PlanID:            "plan-asymmetric-role",
			State:             "completed",
			Verdict:           "confirmed",
			Confidence:        "high",
			RequestCount:      4,
			EvidenceAvailable: true,
			Cases: []browser.ExtensionAuthorizationCaseExecution{
				{ID: "a-own", State: "completed", Result: requestResult("success", 200, "admin-profile")},
				{ID: "b-own", State: "completed", Result: requestResult("success", 200, "user-profile")},
				{ID: "a-to-b", State: "completed", Result: requestResult("success", 200, "user-profile")},
				{ID: "b-to-a", State: "completed", Result: requestResult("denied", 403, "denied")},
			},
			Evidence: []browser.ExtensionAuthorizationCanaryEvidence{{
				Direction: "a-to-b",
				Path:      "body.user.id",
				Source:    "response-json-differential",
			}},
			Reasons: []string{"A-to-B reproduced the target identity resource value"},
		},
	}
}

func TestBlindReviewKeepsAsymmetricAccessFactSeparateFromRolePolicy(t *testing.T) {
	workspace := asymmetricRoleWorkspace()
	review := newAuthorizationBlindReview(workspace.ID)

	context, err := review.begin(workspace)
	require.NoError(t, err)
	require.Equal(t, authorizationReviewPhaseBlind, context.Phase)
	require.NoError(t, review.recordBundle(workspace.Execution.ID))
	require.NoError(t, review.recordDiff(workspace.Execution.ID, browser.ExtensionAuthorizationEvidenceDiff{
		Entries: []browser.ExtensionAuthorizationEvidenceDiffEntry{{Path: "body.user.id"}},
	}))
	require.NoError(t, review.recordValidation(
		workspace.Execution.ID,
		browser.ExtensionAuthorizationEvidenceValidation{
			Direction: "a-to-b",
			Verified:  true,
			Evidence: []browser.ExtensionAuthorizationCanaryEvidence{{
				Direction: "a-to-b",
				Path:      "body.user.id",
				Source:    "response-json-user-canary",
			}},
		},
	))

	commit, err := review.submit(workspace.Execution.ID, authorizationIndependentAssessment{
		AToBObservation:      "target-data-reproduced",
		BToAObservation:      "access-denied",
		IdentityRelationship: "different-privilege",
		PolicyAssessment:     "expected-access-plausible",
		PolicyBasis:          "role-hierarchy",
		EvidencePaths:        []string{"body.user.id"},
		Summary:              "A can read B's profile while B is denied access to A; the observed asymmetry may match an administrator hierarchy.",
	})
	require.NoError(t, err)
	require.Equal(t, "expected-access-plausible", commit.Assessment.PolicyAssessment)

	reconciliation, err := review.reconcile(workspace)
	require.NoError(t, err)
	require.Equal(t, "agree", reconciliation.FactAgreement)
	require.Equal(t, "confirmed", reconciliation.Deterministic.EvidenceGrade)
	require.Equal(t, "cross-identity-resource-reproduction-confirmed", reconciliation.Deterministic.Observation)
	require.Equal(t, "expected-access-plausible", reconciliation.IndependentReview.Assessment.PolicyAssessment)
	require.Equal(t, "not-evaluated-by-deterministic-engine", reconciliation.Deterministic.PolicyConclusion)
}

func TestBlindReviewRejectsUnsupportedPolicyViolationClaim(t *testing.T) {
	workspace := asymmetricRoleWorkspace()
	review := newAuthorizationBlindReview(workspace.ID)
	_, err := review.begin(workspace)
	require.NoError(t, err)
	require.NoError(t, review.recordBundle(workspace.Execution.ID))

	_, err = review.submit(workspace.Execution.ID, authorizationIndependentAssessment{
		AToBObservation:      "target-data-reproduced",
		BToAObservation:      "access-denied",
		IdentityRelationship: "different-privilege",
		PolicyAssessment:     "violation-supported",
		PolicyBasis:          "role-hierarchy",
		Summary:              "Unsupported policy conclusion.",
	})

	require.ErrorContains(t, err, "requires explicit policy or proven equivalent privileges")
}

func TestBlindReviewRequiresDirectionValidationForStrongFact(t *testing.T) {
	workspace := asymmetricRoleWorkspace()
	review := newAuthorizationBlindReview(workspace.ID)
	_, err := review.begin(workspace)
	require.NoError(t, err)
	require.NoError(t, review.recordBundle(workspace.Execution.ID))
	require.NoError(t, review.recordDiff(workspace.Execution.ID, browser.ExtensionAuthorizationEvidenceDiff{
		Entries: []browser.ExtensionAuthorizationEvidenceDiffEntry{{Path: "body.user.id"}},
	}))

	_, err = review.submit(workspace.Execution.ID, authorizationIndependentAssessment{
		AToBObservation:      "target-data-reproduced",
		BToAObservation:      "access-denied",
		IdentityRelationship: "not-proven",
		PolicyAssessment:     "requires-policy",
		PolicyBasis:          "none",
		EvidencePaths:        []string{"body.user.id"},
		Summary:              "A stable target field appears to match, but it has not been validated.",
	})

	require.ErrorContains(t, err, "a-to-b target-data-reproduced requires a verified evidence path")
}
