// Package mcpclient exposes outbound MCP clients and the profile server registry
// to Yak scripts without importing the Yak engine or the MCP server command.
package mcpclient

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/yaklang/gorm"
)

type options struct {
	ctx           context.Context
	db            *gorm.DB
	timeout       time.Duration
	env, headers  map[string]any
	enabled       bool
	limit, offset int
}

type Option func(*options)

func WithContext(ctx context.Context) Option { return func(o *options) { o.ctx = ctx } }
func WithDatabase(db *gorm.DB) Option        { return func(o *options) { o.db = db } }
func WithTimeout(seconds float64) Option {
	return func(o *options) {
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 3600 {
			o.timeout = 0
			return
		}
		o.timeout = time.Duration(seconds * float64(time.Second))
	}
}
func WithEnv(env map[string]any) Option { return func(o *options) { o.env = env } }
func WithHeaders(headers map[string]any) Option {
	return func(o *options) { o.headers = headers }
}
func WithEnabled(enabled bool) Option { return func(o *options) { o.enabled = enabled } }
func WithLimit(limit int) Option      { return func(o *options) { o.limit = limit } }
func WithOffset(offset int) Option    { return func(o *options) { o.offset = offset } }

func newOptions(opts ...Option) (*options, error) {
	o := &options{ctx: context.Background(), timeout: 30 * time.Second, enabled: true, limit: 20}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	if o.ctx == nil {
		return nil, fmt.Errorf("MCP context is nil")
	}
	if err := o.ctx.Err(); err != nil {
		return nil, err
	}
	if o.timeout <= 0 {
		return nil, fmt.Errorf("MCP timeout must be between 0 and 3600 seconds (exclusive of 0)")
	}
	if o.limit < 1 || o.limit > 100 || o.offset < 0 {
		return nil, fmt.Errorf("MCP limit must be 1..100 and offset must be nonnegative")
	}
	for _, values := range []map[string]any{o.env, o.headers} {
		for key, value := range values {
			if _, ok := value.(string); !ok || key == "" {
				return nil, fmt.Errorf("MCP env and headers must have nonempty keys and string values")
			}
		}
	}
	return o, nil
}

var Exports = map[string]any{
	"NewClient": NewClient, "Connect": Connect,
	"AddServer": AddServer, "DeleteServer": DeleteServer, "ListServers": ListServers,
	"context": WithContext, "timeout": WithTimeout, "env": WithEnv,
	"headers": WithHeaders, "enabled": WithEnabled, "limit": WithLimit, "offset": WithOffset,
}
