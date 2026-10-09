package yaklib

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/gorm"
)

// DBHistoryOption configures project discovery and HTTP history reads.
type DBHistoryOption func(*dbHistoryConfig)
type dbHistoryConfig struct {
	ctx                                              context.Context
	projectDB, profileDB                             *gorm.DB
	projectID, keyword, url, methods, status, source string
	limit, offset, packetLimit                       int
	afterID, beforeID                                int64
}

func WithDBHistoryRuntime(ctx context.Context, project, profile *gorm.DB) DBHistoryOption {
	return func(c *dbHistoryConfig) { c.ctx, c.projectDB, c.profileDB = ctx, project, profile }
}
func dbHistoryProjectID(id string) DBHistoryOption {
	return func(c *dbHistoryConfig) { c.projectID = id }
}
func dbHistoryKeyword(s string) DBHistoryOption { return func(c *dbHistoryConfig) { c.keyword = s } }
func dbHistoryURL(s string) DBHistoryOption     { return func(c *dbHistoryConfig) { c.url = s } }
func dbHistoryMethods(s string) DBHistoryOption { return func(c *dbHistoryConfig) { c.methods = s } }
func dbHistoryStatus(s string) DBHistoryOption  { return func(c *dbHistoryConfig) { c.status = s } }
func dbHistorySource(s string) DBHistoryOption  { return func(c *dbHistoryConfig) { c.source = s } }
func dbHistoryLimit(n int) DBHistoryOption      { return func(c *dbHistoryConfig) { c.limit = n } }
func dbHistoryOffset(n int) DBHistoryOption     { return func(c *dbHistoryConfig) { c.offset = n } }
func dbHistoryPacketLimit(n int) DBHistoryOption {
	return func(c *dbHistoryConfig) { c.packetLimit = n }
}
func dbHistoryAfterID(n int64) DBHistoryOption  { return func(c *dbHistoryConfig) { c.afterID = n } }
func dbHistoryBeforeID(n int64) DBHistoryOption { return func(c *dbHistoryConfig) { c.beforeID = n } }
func historyConfig(packetLimit int, opts []DBHistoryOption) (*dbHistoryConfig, error) {
	c := &dbHistoryConfig{ctx: context.Background(), limit: 10, packetLimit: packetLimit}
	for _, opt := range opts {
		opt(c)
	}
	if c.ctx == nil {
		return nil, fmt.Errorf("history context is nil")
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if c.limit < 1 || c.limit > 100 || c.offset < 0 || c.offset > 1000000 {
		return nil, fmt.Errorf("limit must be 1..100 and offset 0..1000000")
	}
	if c.packetLimit < 0 || c.packetLimit > 32768 {
		return nil, fmt.Errorf("packet limit must be 0..32768 bytes")
	}
	if len(c.keyword) > 1024 || len(c.url) > 2048 || len(c.projectID) > 128 {
		return nil, fmt.Errorf("query or project ID is too long")
	}
	if len(c.methods) > 256 || len(c.source) > 256 || len(c.status) > 256 {
		return nil, fmt.Errorf("HTTP filters are too long")
	}
	// Bound ranges before delegating to the shared port/range parser.
	for _, interval := range strings.Split(c.status, ",") {
		if strings.TrimSpace(interval) == "" {
			continue
		}
		endpoints := strings.Split(strings.TrimSpace(interval), "-")
		if len(endpoints) > 2 {
			return nil, fmt.Errorf("invalid HTTP status range")
		}
		for _, endpoint := range endpoints {
			n, err := strconv.Atoi(strings.TrimSpace(endpoint))
			if err != nil || n < 0 || n > 999 {
				return nil, fmt.Errorf("HTTP status endpoints must be 0..999")
			}
		}
	}
	if c.afterID < 0 || c.beforeID < 0 {
		return nil, fmt.Errorf("flow IDs must be nonnegative")
	}
	return c, nil
}
func init() {
	DatabaseExports["ListYakProjects"] = ListYakProjects
	DatabaseExports["QueryHTTPFlows"] = QueryHTTPFlows
	for name, fn := range map[string]any{
		"projectID": dbHistoryProjectID, "keyword": dbHistoryKeyword, "url": dbHistoryURL,
		"methods": dbHistoryMethods, "statusCode": dbHistoryStatus, "sourceType": dbHistorySource,
		"limit": dbHistoryLimit, "offset": dbHistoryOffset, "packetLimit": dbHistoryPacketLimit,
		"afterID": dbHistoryAfterID, "beforeID": dbHistoryBeforeID,
	} {
		DatabaseExports[name] = fn
	}
}
