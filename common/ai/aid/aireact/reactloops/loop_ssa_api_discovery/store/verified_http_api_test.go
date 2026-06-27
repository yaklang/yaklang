package store

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestVerifiedHttpApi_UpsertUnique(t *testing.T) {
	repo, cleanup := openTestRepo(t)
	defer cleanup()

	sess := &DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	a := &VerifiedHttpApi{
		SessionID: sess.ID, Method: "POST", PathPattern: "/api/login",
		Verified: true, Confidence: 90, ProbeStatusCode: 200,
		ProbeAttemptsJSON: `[{"status":200}]`,
	}
	require.NoError(t, repo.UpsertVerifiedHttpApi(a))
	b := &VerifiedHttpApi{SessionID: sess.ID, Method: "POST", PathPattern: "/api/login", Verified: false, RejectReason: "x"}
	require.NoError(t, repo.UpsertVerifiedHttpApi(b))

	rows, err := repo.ListVerifiedHttpApis(sess.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].Verified, "weak sync must not clobber prior probe-verified row")

	c := &VerifiedHttpApi{
		SessionID: sess.ID, Method: "POST", PathPattern: "/api/login",
		Verified: false, RejectReason: "x",
		ProbeStatusCode: 404, ProbeAttemptsJSON: `[{"status":404}]`,
	}
	require.NoError(t, repo.UpsertVerifiedHttpApi(c))
	rows, err = repo.ListVerifiedHttpApis(sess.ID)
	require.NoError(t, err)
	require.False(t, rows[0].Verified)
	total, verified, err := repo.CountVerifiedHttpApis(sess.ID)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 0, verified)
}
