package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestValidateAuthStageOutput_RequiresEvidence(t *testing.T) {
	batch := []WorklistSeedItem{{RelPath: "LoginController.java", Category: worklistCategoryAuthEntry, Priority: 2}}
	out := &CodeReadingStageOutput{Stage: 1, ReadFilesCompleted: []string{"LoginController.java"}}
	require.Error(t, validateAuthStageOutput(out, batch, nil))
}

func TestValidateAuthStageOutput_MinimalFields(t *testing.T) {
	batch := []WorklistSeedItem{{RelPath: "LoginController.java", Category: worklistCategoryAuthEntry, Priority: 2}}
	out := &CodeReadingStageOutput{
		Stage:              1,
		ReadFilesCompleted: []string{"LoginController.java"},
		AuthEvidence: &AuthEvidenceRecord{
			LoginEndpoints: []AuthLoginEndpoint{{
				Method:      "POST",
				Path:        "/admin/login",
				ContentType: "application/x-www-form-urlencoded",
			}},
		},
	}
	require.NoError(t, validateAuthStageOutput(out, batch, nil))
}

func TestValidateAuthStageOutput_ReachableRequiresProbe(t *testing.T) {
	batch := []WorklistSeedItem{{RelPath: "LoginController.java", Category: worklistCategoryAuthEntry, Priority: 2}}
	out := &CodeReadingStageOutput{
		Stage:              1,
		ReadFilesCompleted: []string{"LoginController.java"},
		AuthEvidence: &AuthEvidenceRecord{
			LoginEndpoints: []AuthLoginEndpoint{{
				Method:      "POST",
				Path:        "/admin/login",
				ContentType: "application/x-www-form-urlencoded",
			}},
		},
	}
	rt := &Runtime{Session: &store.DiscoverySession{TargetReachable: true}}
	require.Error(t, validateAuthStageOutput(out, batch, rt))
	out.AuthEvidence.LoginEndpoints[0].ProbeAttempted = true
	require.NoError(t, validateAuthStageOutput(out, batch, rt))
}

func TestWorklistPopBatchWithAuthGate(t *testing.T) {
	seed := []WorklistSeedItem{
		{RelPath: "a.java", Category: worklistCategoryAPIHandler, Priority: 3},
	}
	wl := newCodeReadingWorklist(seed)
	batch := wl.PopBatchWithAuthGate(30, true, false)
	require.Nil(t, batch)
	require.Equal(t, 1, wl.Len())
}

func TestIsLoginTemplateRelPath(t *testing.T) {
	require.True(t, isLoginTemplateRelPath("templates/admin/login.html"))
	require.False(t, isLoginTemplateRelPath("static/app.js"))
}
