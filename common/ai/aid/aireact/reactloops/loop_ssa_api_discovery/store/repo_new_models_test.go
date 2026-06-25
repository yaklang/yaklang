package store

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func openTestRepo(t *testing.T) (*Repository, func()) {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))
	repo := NewRepository(db)
	cleanup := func() {
		if sqlDB := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
	}
	return repo, cleanup
}

func TestAutoMigrate_Idempotent(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()

	require.NoError(t, AutoMigrate(db))
	require.NoError(t, AutoMigrate(db))
}

func TestAuthCredential_NewFields(t *testing.T) {
	repo, cleanup := openTestRepo(t)
	defer cleanup()

	sess := &DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	now := time.Now()
	cred := &AuthCredential{
		SessionID:    sess.ID,
		AuthType:     "cookie_session",
		HeadersJSON:  `{"Cookie":"sess=abc","X-CSRF":"tok123"}`,
		HeadersText:  "Cookie: sess=abc\r\nX-CSRF: tok123",
		Verified:     true,
		RefreshState: AuthRefreshStateFresh,
		LastAcquiredAt: &now,
		LastVerifiedAt: &now,
		ExpiresHint:   `{"strategy":"ttl_seconds","ttl_seconds":3600}`,
	}
	require.NoError(t, repo.CreateAuthCredential(cred))
	require.NotZero(t, cred.ID)

	got, err := repo.GetAuthCredential(sess.ID, cred.ID)
	require.NoError(t, err)
	require.Equal(t, `{"Cookie":"sess=abc","X-CSRF":"tok123"}`, got.HeadersJSON)
	require.Equal(t, AuthRefreshStateFresh, got.RefreshState)
	require.NotNil(t, got.LastAcquiredAt)
}

func TestAuthCredential_GetFreshestVerified(t *testing.T) {
	repo, cleanup := openTestRepo(t)
	defer cleanup()

	sess := &DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	now := time.Now()
	old := time.Now().Add(-1 * time.Hour)
	c1 := &AuthCredential{SessionID: sess.ID, AuthType: "basic_auth", Verified: true, LastVerifiedAt: &old}
	c2 := &AuthCredential{SessionID: sess.ID, AuthType: "jwt_bearer", Verified: true, LastVerifiedAt: &now}
	require.NoError(t, repo.CreateAuthCredential(c1))
	require.NoError(t, repo.CreateAuthCredential(c2))

	got, err := repo.GetFreshestVerifiedCredential(sess.ID)
	require.NoError(t, err)
	require.Equal(t, c2.ID, got.ID)
}

func TestAuthAcquisitionRecipe_CRUD(t *testing.T) {
	repo, cleanup := openTestRepo(t)
	defer cleanup()

	sess := &DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	recipe := &AuthAcquisitionRecipe{
		SessionID:    sess.ID,
		CredentialID: 1,
		Method:       "login_form_post",
		LoginURL:     "http://target/login",
		StepsJSON:    `[{"method":"POST","url":"http://target/login","body":"user=admin&pass=admin"}]`,
		VerifyURL:    "http://target/api/me",
	}
	require.NoError(t, repo.CreateAuthAcquisitionRecipe(recipe))
	require.NotZero(t, recipe.ID)

	got, err := repo.GetAuthAcquisitionRecipe(sess.ID, recipe.ID)
	require.NoError(t, err)
	require.Equal(t, "login_form_post", got.Method)
	require.Equal(t, "http://target/login", got.LoginURL)

	byCredID, err := repo.GetRecipeByCredentialID(sess.ID, 1)
	require.NoError(t, err)
	require.Equal(t, recipe.ID, byCredID.ID)
}

func TestEndpointValidationAttempt_CRUD(t *testing.T) {
	repo, cleanup := openTestRepo(t)
	defer cleanup()

	sess := &DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	attempt := &EndpointValidationAttempt{
		SessionID:      sess.ID,
		HttpEndpointID: 1,
		AttemptNo:      1,
		URL:            "http://target/api/users",
		Method:         "GET",
		StatusCode:     200,
		Verdict:        "alive",
		Reason:         "200 OK",
	}
	require.NoError(t, repo.CreateEndpointValidationAttempt(attempt))

	attempts, err := repo.ListEndpointValidationAttempts(sess.ID, 1)
	require.NoError(t, err)
	require.Len(t, attempts, 1)
	require.Equal(t, "alive", attempts[0].Verdict)

	n, err := repo.CountEndpointValidationAttempts(sess.ID, 1)
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestHttpEndpoint_StatusField(t *testing.T) {
	repo, cleanup := openTestRepo(t)
	defer cleanup()

	sess := &DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	ep := &HttpEndpoint{
		SessionID:   sess.ID,
		Method:      "GET",
		PathPattern: "/api/users",
		Source:      "ai",
		Status:      EndpointStatusRejected,
		RejectReason: "404",
	}
	require.NoError(t, repo.CreateHttpEndpoint(ep))

	aliveEps, err := repo.ListAliveHttpEndpoints(sess.ID)
	require.NoError(t, err)
	require.Empty(t, aliveEps, "rejected endpoints should not appear in alive list")

	ep.Status = EndpointStatusAlive
	ep.FunctionScore = 80
	require.NoError(t, repo.UpdateHttpEndpointStatus(ep))

	aliveEps, err = repo.ListAliveHttpEndpoints(sess.ID)
	require.NoError(t, err)
	require.Len(t, aliveEps, 1)
	require.Equal(t, 80, aliveEps[0].FunctionScore)
}

func TestHttpEndpoint_EmptyStatusBackwardCompat(t *testing.T) {
	repo, cleanup := openTestRepo(t)
	defer cleanup()

	sess := &DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	ep := &HttpEndpoint{
		SessionID:   sess.ID,
		Method:      "GET",
		PathPattern: "/legacy",
		Source:      "static",
	}
	require.NoError(t, repo.CreateHttpEndpoint(ep))

	aliveEps, err := repo.ListAliveHttpEndpoints(sess.ID)
	require.NoError(t, err)
	require.Len(t, aliveEps, 1, "empty status should be treated as alive for backward compat")
}

func TestCoverageWorkItem_ReadCount(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	require.NoError(t, AutoMigrate(db))
	repo := NewRepository(db)

	sess := &DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	row := &CoverageWorkItem{
		SessionID:     sess.ID,
		Kind:          CoverageKindHttpEndpoint,
		RefID:         1,
		RefLabel:      "GET /dead",
		Status:        CoverageStatusRejected,
		BlockedReason: "404",
	}
	require.NoError(t, db.Create(row).Error)

	n, err := repo.CountCoverageWorkItemsByStatus(sess.ID, CoverageKindHttpEndpoint, CoverageStatusRejected)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}
