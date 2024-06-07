package yaklib

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestWithEngine(t *testing.T) {
	cfg := yakit.GetNetworkConfig()
	bak := cfg.AppConfigs
	defer func() {
		cfg.AppConfigs = bak
		yakit.ConfigureNetWork(cfg)
	}()
	cfg.AppConfigs = []*ypb.ThirdPartyApplicationConfig{
		{
			Type:           "test engine",
			APIKey:         "config key",
			Domain:         "domain",
			UserSecret:     "secret",
			UserIdentifier: "user",
			Namespace:      "namespace",
			WebhookURL:     "webhook",
		},
	}
	yakit.ConfigureNetWork(cfg)
	engineCfg := &_spaceEngineConfig{}
	withEngine("test engine", "this is api key")(engineCfg)
	assert.Equal(t, "this is api key", engineCfg.apiKey)
	assert.Equal(t, "user", engineCfg.user)
	assert.Equal(t, "domain", engineCfg.domain)
}
