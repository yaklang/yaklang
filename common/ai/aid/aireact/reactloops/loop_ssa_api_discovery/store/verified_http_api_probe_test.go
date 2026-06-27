package store

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestListVerifiedHttpApisForProbe(t *testing.T) {
	repo, cleanup := openTestRepo(t)
	defer cleanup()
	sess := &DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	require.NoError(t, repo.UpsertVerifiedHttpApi(&VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/a",
		Verified: true, FullSampleURL: "http://x/a",
	}))
	require.NoError(t, repo.UpsertVerifiedHttpApi(&VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/b",
		Verified: false, RejectReason: "404",
	}))
	require.NoError(t, repo.UpsertVerifiedHttpApi(&VerifiedHttpApi{
		SessionID: sess.ID, Method: "POST", PathPattern: "/c",
		Verified: true, FullSampleURL: "",
	}))

	rows, err := repo.ListVerifiedHttpApisForProbe(sess.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "/a", rows[0].PathPattern)

	gate, err := repo.CountVerifiedHttpApiGate(sess.ID)
	require.NoError(t, err)
	require.Equal(t, 3, gate.Total)
	require.Equal(t, 1, gate.Verified)
	require.Equal(t, 2, gate.Rejected)
}
