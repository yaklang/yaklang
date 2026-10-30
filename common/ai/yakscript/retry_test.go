package yakscript

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestModelErrorSurvivesWrapper(t *testing.T) {
	req := &Request{}
	require.Error(t, ModelError(req, "http", 429, "busy", "retry later", "900"))
	f := req.Failure()
	require.Equal(t, "900", f.RetryAfter)
	require.Equal(t, 429, f.StatusCode)
	require.True(t, f.CanRetry())
	require.Error(t, ModelError(req, "invalid", 0, "", "", ""))
	require.False(t, req.Failure().CanRetry())
}
