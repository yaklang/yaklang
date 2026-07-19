package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneralKVConfig_LiteForgeDisableTimeline(t *testing.T) {
	require.False(t, NewGeneralKVConfig().GetLiteForgeDisableTimeline())
	require.True(t, NewGeneralKVConfig(WithLiteForgeDisableTimeline()).GetLiteForgeDisableTimeline())
}
