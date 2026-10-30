package aicommon

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

const attachedBrowserHTTPRequestToolName = "do_http_request"

// BrowserHTTPFlowIdentityResolver resolves a selector against authenticated
// Bridge connections. Neither the model nor an HTTP header supplies the badge.
type BrowserHTTPFlowIdentityResolver func(deviceID, browserRef string) (httpctx.BrowserInstance, error)

func WithBrowserHTTPFlowIdentityResolver(resolver BrowserHTTPFlowIdentityResolver) ConfigOption {
	return func(c *Config) error {
		c.browserHTTPFlowIdentityResolver = resolver
		return nil
	}
}

func (c *Config) GetBrowserHTTPFlowIdentityResolver() BrowserHTTPFlowIdentityResolver {
	return c.browserHTTPFlowIdentityResolver
}

func bindBrowserHTTPFlowContext(ctx context.Context, config AICallerConfigIf, toolName string, params aitool.InvokeParams) (context.Context, error) {
	if toolName != attachedBrowserHTTPRequestToolName {
		return ctx, nil
	}
	deviceID := strings.TrimSpace(params.GetString("device_id"))
	browserRef := strings.TrimSpace(params.GetString("browser_ref"))
	if deviceID == "" && browserRef == "" {
		return ctx, nil
	}
	provider, ok := config.(interface {
		GetBrowserHTTPFlowIdentityResolver() BrowserHTTPFlowIdentityResolver
	})
	if !ok || provider.GetBrowserHTTPFlowIdentityResolver() == nil {
		return nil, utils.Error("browser HTTP testing requires an authenticated browser Bridge")
	}
	identity, err := provider.GetBrowserHTTPFlowIdentityResolver()(deviceID, browserRef)
	if err != nil {
		return nil, err
	}
	return httpctx.WithBrowserInstance(ctx, identity), nil
}
