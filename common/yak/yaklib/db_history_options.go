package yaklib

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/gorm"
)

// DBHistoryOption 是 ListYakProjects、QueryHTTPFlows、QueryHTTPFlowByID 和 QueryRiskInDatabase 的查询配置项，具体支持的选项以各函数说明为准。
type DBHistoryOption func(*dbHistoryConfig)
type dbHistoryConfig struct {
	ctx                                              context.Context
	projectDB, profileDB                             *gorm.DB
	projectID, keyword, url, methods, status, source string
	limit, offset, packetLimit                       int
	afterID, beforeID                                int64
}

// WithDBHistoryRuntime 为 Go 调用方绑定上下文、项目库与项目登记库；这是运行时注入接口，不导出为 Yak 函数。
// nil 数据库沿用引擎全局库；ctx 必须非 nil。
func WithDBHistoryRuntime(ctx context.Context, project, profile *gorm.DB) DBHistoryOption {
	return func(c *dbHistoryConfig) { c.ctx, c.projectDB, c.profileDB = ctx, project, profile }
}

// dbHistoryProjectID 设置项目数据库标识（导出名为 db.projectID）。
//
// 空字符串或 current 使用当前运行绑定的数据库；其他值必须是 db.ListYakProjects 返回的 DatabaseID，不能使用数字 ProjectID 或任意文件路径。只查询 Available=true 且 SupportsHTTP=true 的项目，跨库只读。
//
// 参数:
//   - id: 项目数据库标识
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.projectID("current"), db.limit(5))~
// println(page.Dump())
// ```
func dbHistoryProjectID(id string) DBHistoryOption {
	return func(c *dbHistoryConfig) { c.projectID = id }
}

// dbHistoryKeyword 设置查询关键字（导出名为 db.keyword）。
//
// ListYakProjects 中按名称/描述做不区分大小写的子串匹配；QueryHTTPFlows 中复用引擎 HTTPFlow 数据库关键字过滤，不扫描旁路文件正文；QueryRiskInDatabase 中搜索风险目标、标题、类型、参数、payload、details、描述和修复建议。默认空值不过滤，最多 1024 字节。
//
// 参数:
//   - s: 查询关键字
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.keyword("CSRF"), db.limit(5))~
// println(page.Dump())
// ```
func dbHistoryKeyword(s string) DBHistoryOption { return func(c *dbHistoryConfig) { c.keyword = s } }

// dbHistoryURL 设置 HTTP URL 子串（导出名为 db.url）。
//
// 用于 QueryHTTPFlows 和 QueryRiskInDatabase 的 URL 模糊匹配，默认空值不过滤，最多 2048 字节。
//
// 参数:
//   - s: HTTP URL 子串
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.url("/api/orders"), db.limit(5))~
// println(page.Dump())
// ```
func dbHistoryURL(s string) DBHistoryOption { return func(c *dbHistoryConfig) { c.url = s } }

// dbHistoryMethods 设置 HTTP 方法过滤条件（导出名为 db.methods）。
//
// 用于 QueryHTTPFlows，支持逗号分隔，如 GET,POST；默认空值不过滤，最多 256 字节。
//
// 参数:
//   - s: HTTP 方法过滤条件
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.methods("GET,POST"), db.limit(5))~
// println(page.Dump())
// ```
func dbHistoryMethods(s string) DBHistoryOption { return func(c *dbHistoryConfig) { c.methods = s } }

// dbHistoryStatus 设置 HTTP 状态码过滤条件（导出名为 db.statusCode）。
//
// 用于 QueryHTTPFlows，支持单个状态码、逗号列表和范围，如 200,400-499；默认空值不过滤，最多 256 字节，各端点为 0–999。
//
// 参数:
//   - s: HTTP 状态码过滤条件
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.statusCode("200,400-499"), db.limit(5))~
// println(page.Dump())
// ```
func dbHistoryStatus(s string) DBHistoryOption { return func(c *dbHistoryConfig) { c.status = s } }

// dbHistorySource 设置 HTTP 流量来源过滤条件（导出名为 db.sourceType）。
//
// 用于 QueryHTTPFlows，支持逗号分隔，如 mitm,scan,basic-crawler；默认空值不过滤，最多 256 字节。
//
// 参数:
//   - s: HTTP 流量来源过滤条件
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.sourceType("mitm,scan"), db.limit(5))~
// println(page.Dump())
// ```
func dbHistorySource(s string) DBHistoryOption { return func(c *dbHistoryConfig) { c.source = s } }

// dbHistoryLimit 设置每页结果数量（导出名为 db.limit）。
//
// 用于 ListYakProjects、QueryHTTPFlows 和 QueryRiskInDatabase，默认 10，范围 1–100；HTTP 查询还要求 limit*packetLimit*2 不超过 128 KiB，风险查询随条数调整每个文本字段的摘要预算。
//
// 参数:
//   - n: 每页结果数量
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.limit(5))~
// println(page.Dump())
// ```
func dbHistoryLimit(n int) DBHistoryOption { return func(c *dbHistoryConfig) { c.limit = n } }

// dbHistoryOffset 设置查询结果偏移（导出名为 db.offset）。
//
// 用于 ListYakProjects、QueryHTTPFlows 和 QueryRiskInDatabase，默认 0，范围 0–1000000；HTTP 与风险查询使用上一页的 NextOffset 续查，偏移按过滤后的结果计算。
//
// 参数:
//   - n: 查询结果偏移
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.offset(0), db.limit(5))~
// println(page.Dump())
// ```
func dbHistoryOffset(n int) DBHistoryOption { return func(c *dbHistoryConfig) { c.offset = n } }

// dbHistoryPacketLimit 设置单份 HTTP 报文的读取/展示预算（导出名为 db.packetLimit）。
//
// 按数据库 Go quoted 存储的字节数计算，原文字节数可能更小；超限标记 RequestOmitted/ResponseOmitted，不返回截断伪原文。QueryHTTPFlows 默认 2048，要求 1–32768 且总预算不超过 128 KiB；QueryHTTPFlowByID 为兼容原 HTTPFlow 返回对象仅允许 0。不限制 ExportPackets 导出的原始报文字节数。
//
// 参数:
//   - n: 单份 HTTP 报文的读取/展示预算
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.packetLimit(1024), db.limit(5))~
// println(page.Dump())
// ```
func dbHistoryPacketLimit(n int) DBHistoryOption {
	return func(c *dbHistoryConfig) { c.packetLimit = n }
}

// dbHistoryAfterID 设置记录 ID 下界（导出名为 db.afterID）。
//
// 用于 QueryHTTPFlows 和 QueryRiskInDatabase，只返回 ID 大于 n 的记录，不包含等于 n 的记录；默认 0 不限制，n 不得为负数，ID 只在选定数据库内有效。
//
// 参数:
//   - n: HTTPFlow 或 Risk 记录 ID 下界
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.afterID(0), db.limit(5))~
// println(page.Dump())
// ```
func dbHistoryAfterID(n int64) DBHistoryOption { return func(c *dbHistoryConfig) { c.afterID = n } }

// dbHistoryBeforeID 设置记录 ID 上界（导出名为 db.beforeID）。
//
// 用于 QueryHTTPFlows 和 QueryRiskInDatabase，只返回 ID 小于 n 的记录，不包含等于 n 的记录；默认 0 不限制，n 不得为负数，ID 只在选定数据库内有效。
//
// 参数:
//   - n: HTTPFlow 或 Risk 记录 ID 上界
//
// 返回值:
//   - option: DBHistoryOption 配置项，传给相应的 db 查询函数生效；无效值在查询时返回错误
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.beforeID(1000000), db.limit(5))~
// println(page.Dump())
// ```
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
		return nil, fmt.Errorf("record IDs must be nonnegative")
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
