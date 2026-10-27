package aicommon

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
)

func WithNetworkDebug(enabled bool) ConfigOption {
	return func(c *Config) error { c.NetworkDebug = enabled; return nil }
}
func (c *Config) GetNetworkDebug() bool { return c.NetworkDebug }

func modelNetworkDebugContext(ctx context.Context, cfg AICallerConfigIf, sequence int64) context.Context {
	scope := httptrace.Scope{RuntimeID: cfg.GetRuntimeId(), CallID: uuid.NewString(), Sequence: sequence}
	if session, ok := cfg.(interface{ GetPersistentSessionID() string }); ok {
		scope.SessionID = session.GetPersistentSessionID()
	}
	ctx = httptrace.WithScope(ctx, scope)
	enabled := consts.IsAINetworkDebugEnabled()
	if settings, ok := cfg.(interface{ GetNetworkDebug() bool }); ok {
		enabled = enabled || settings.GetNetworkDebug()
	}
	if enabled {
		ctx = httptrace.WithEnabled(ctx, true)
	}
	return ctx
}
