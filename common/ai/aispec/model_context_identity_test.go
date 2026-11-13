package aispec

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestModelContextIdentitySeparatesConfiguredServices(t *testing.T) {
	base := &ypb.AIModelConfig{ContextOwnerId: uuid.NewString(), ModelName: "same-name", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: "https://a.example/v1", APIKey: "secret-account-a", APIType: "chat_completions"}}
	key := ModelContextIdentity(base)
	require.Len(t, key, 101)
	require.NotContains(t, key, base.Provider.APIKey)
	require.NotEqual(t, key, ModelContextIdentity(base, WithBaseURL("https://override.example/v1")), "a request-level connection override must not confirm the configured service")
	for name, edit := range map[string]func(*ypb.AIModelConfig){
		"owner":   func(c *ypb.AIModelConfig) { c.ContextOwnerId = uuid.NewString() },
		"address": func(c *ypb.AIModelConfig) { c.Provider.BaseURL = "https://b.example/v1" },
		"account": func(c *ypb.AIModelConfig) { c.Provider.APIKey = "secret-account-b" },
		"api":     func(c *ypb.AIModelConfig) { c.Provider.APIType = "responses" },
		"endpoint": func(c *ypb.AIModelConfig) {
			c.Provider.EnableEndpoint = true
			c.Provider.Endpoint = "https://c.example/custom"
		},
		"tenant_header": func(c *ypb.AIModelConfig) { c.Provider.Headers = []*ypb.KVPair{{Key: "X-Tenant", Value: "b"}} },
		"model":         func(c *ypb.AIModelConfig) { c.ModelName = "other-model" },
	} {
		t.Run(name, func(t *testing.T) {
			c := proto.Clone(base).(*ypb.AIModelConfig)
			edit(c)
			require.NotEqual(t, key, ModelContextIdentity(c))
		})
	}
	same := proto.Clone(base).(*ypb.AIModelConfig)
	same.ProviderId = 123
	same.Provider.Proxy = "http://proxy.example"
	effort := "none"
	same.Provider.ReasoningEffort = &effort
	require.Equal(t, key, ModelContextIdentity(same), "selection ID, transport proxy and reasoning are not a different model service")
	base.Provider.Headers = []*ypb.KVPair{{Key: "X-Tenant", Value: "a"}, {Key: "x-account", Value: "b"}}
	same = proto.Clone(base).(*ypb.AIModelConfig)
	same.Provider.Headers = []*ypb.KVPair{{Key: "X-Account", Value: "b"}, {Key: "x-tenant", Value: "a"}}
	require.Equal(t, ModelContextIdentity(base), ModelContextIdentity(same))
	require.Empty(t, ModelContextIdentity(nil))
	require.Equal(t, base.ContextOwnerId, ModelContextOwnerID(ModelContextIdentity(base)))
	base.ContextOwnerId = ""
	require.Empty(t, ModelContextIdentity(base), "unowned configurations must not share a context cache")
}
