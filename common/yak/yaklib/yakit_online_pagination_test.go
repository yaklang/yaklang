package yaklib

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeQueryOnlinePluginsRequestAcceptsNil(t *testing.T) {
	req := normalizeQueryOnlinePluginsRequest(nil)

	require.NotNil(t, req)
	require.NotNil(t, req.GetData())
	require.Equal(t, int64(1), req.GetPagination().GetPage())
	require.Equal(t, int64(20), req.GetPagination().GetLimit())
	require.Equal(t, "updated_at", req.GetPagination().GetOrderBy())
	require.Equal(t, "desc", req.GetPagination().GetOrder())
}
