package browser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackgroundProcessName(t *testing.T) {
	require.Equal(t, "ai-browser", BackgroundProcessName("ai-browser"))
	require.Equal(t, "browser", BackgroundProcessName(""))
	require.Equal(t, "browser", BackgroundProcessName("   "))
}
