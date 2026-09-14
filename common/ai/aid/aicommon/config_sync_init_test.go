package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllowSyncInitContextDefaultAndPropagation(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	require.False(t, cfg.AllowSyncInitContext)
	require.False(t, cfg.GetConfigBool("AllowSyncInitContext"))
	for _, allow := range []bool{true, false} {
		require.NoError(t, WithAllowSyncInitContext(allow)(cfg))
		child := NewConfig(context.Background(), ConvertConfigToOptions(cfg)...)
		require.Equal(t, allow, child.AllowSyncInitContext)
		require.Equal(t, allow, child.GetConfigBool("AllowSyncInitContext"))
	}
}
