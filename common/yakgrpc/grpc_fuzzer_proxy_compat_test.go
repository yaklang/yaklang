package yakgrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestBuildFuzzerProxyList_CompatEndpointIDInProxyField(t *testing.T) {
	oldCfg, err := yakit.GetGlobalProxyRulesConfig()
	require.NoError(t, err)
	defer func() {
		_, _ = yakit.SetGlobalProxyRulesConfig(oldCfg)
	}()

	cfg := &ypb.GlobalProxyRulesConfig{
		Endpoints: []*ypb.ProxyEndpoint{
			{Id: "ep-U9kZhbpZ", Url: "http://127.0.0.1:18080"},
		},
	}
	_, err = yakit.SetGlobalProxyRulesConfig(cfg)
	require.NoError(t, err)

	proxies, err := buildFuzzerProxyList(&ypb.FuzzerRequest{Proxy: "ep-U9kZhbpZ"})
	require.NoError(t, err)
	require.Contains(t, proxies, "http://127.0.0.1:18080")
}

