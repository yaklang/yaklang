package aispec

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
)

func WithNetworkDebug(enabled bool) AIConfigOption {
	return func(c *AIConfig) { c.NetworkDebug = enabled }
}

func prepareNetworkDebug(c *AIConfig) {
	scope := httptrace.GetScope(c.Context)
	if scope.CallID == "" {
		scope.CallID = uuid.NewString()
	}
	scope.Provider, scope.Model = c.Type, c.Model
	c.Context = httptrace.WithScope(c.Context, scope)
	enabled := c.NetworkDebug || consts.IsAINetworkDebugEnabled()
	if enabled {
		c.Context = httptrace.WithEnabled(c.Context, true)
	}
}
