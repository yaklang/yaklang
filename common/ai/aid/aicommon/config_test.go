package aicommon

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConfig_Smoking(t *testing.T) {
	config := NewConfig(context.Background())
	require.NotNil(t, config)
	require.NotNil(t, config.OriginalAICallback)
}
