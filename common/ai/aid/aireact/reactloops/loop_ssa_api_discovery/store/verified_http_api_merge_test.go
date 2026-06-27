package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifiedHttpApiHasProbeEvidence(t *testing.T) {
	require.False(t, VerifiedHttpApiHasProbeEvidence(nil))
	require.False(t, VerifiedHttpApiHasProbeEvidence(&VerifiedHttpApi{RejectReason: "auth_required_skipped"}))
	require.True(t, VerifiedHttpApiHasProbeEvidence(&VerifiedHttpApi{ProbeStatusCode: 404}))
	require.True(t, VerifiedHttpApiHasProbeEvidence(&VerifiedHttpApi{ProbeAttemptsJSON: `[{"round":1}]`}))
	require.False(t, VerifiedHttpApiHasProbeEvidence(&VerifiedHttpApi{ProbeAttemptsJSON: "[]"}))
}

func TestMergeVerifiedHttpApiUpdate_PreservesSuccessfulProbe(t *testing.T) {
	existing := &VerifiedHttpApi{
		Method: "GET", PathPattern: "/api/methods",
		Verified: true, ProbeStatusCode: 200,
		FullSampleURL: "http://127.0.0.1:8080/api/methods",
		VerdictReason: "hit", Confidence: 100,
		ProbeAttemptsJSON: `[{"status":200}]`,
	}
	incoming := &VerifiedHttpApi{
		Method: "GET", PathPattern: "/api/methods",
		Verified: false, Source: "feature_verify",
		RejectReason: "feature_verify: not verified",
	}
	merged := MergeVerifiedHttpApiUpdate(existing, incoming)
	require.True(t, merged.Verified)
	require.Equal(t, 200, merged.ProbeStatusCode)
	require.Equal(t, existing.FullSampleURL, merged.FullSampleURL)
	require.Equal(t, existing.ProbeAttemptsJSON, merged.ProbeAttemptsJSON)
}

func TestMergeVerifiedHttpApiUpdate_IncomingProbeWins(t *testing.T) {
	existing := &VerifiedHttpApi{
		Method: "GET", PathPattern: "/api/x",
		Verified: false, ProbeStatusCode: 404,
		RejectReason: "not found",
	}
	incoming := &VerifiedHttpApi{
		Method: "GET", PathPattern: "/api/x",
		Verified: true, ProbeStatusCode: 200,
		FullSampleURL: "http://127.0.0.1:8080/api/x",
		ProbeAttemptsJSON: `[{"status":200}]`,
	}
	merged := MergeVerifiedHttpApiUpdate(existing, incoming)
	require.True(t, merged.Verified)
	require.Equal(t, 200, merged.ProbeStatusCode)
}
