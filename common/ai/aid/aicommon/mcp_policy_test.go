package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsMCPServersAllowedConfig_NilConfig(t *testing.T) {
	assert.True(t, IsMCPServersAllowedConfig(nil))
}

func TestIsMCPServersAllowedConfig_DisallowMCPServers(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisallowMCPServers(true))
	assert.False(t, IsMCPServersAllowedConfig(cfg))
	assert.True(t, cfg.AiToolManager != nil)
	assert.True(t, cfg.AiToolManager.DisallowMCPServers())
}
