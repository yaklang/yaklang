package yaklib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// RiskDatabaseItem 是风险库的查询摘要；不包含原始报文，长文本可能被截断。
type RiskDatabaseItem struct {
	ID                                                       uint
	CreatedAt, UpdatedAt                                     time.Time
	Hash, IP, Host, Url                                      string
	Port                                                     int
	Title, TitleVerbose, RiskType, RiskTypeVerbose, Severity string
	Description, Solution, Parameter, Payload, Details       string
	Tags, FromYakScript, RuntimeId                           string
	WaitingVerified, Truncated                               bool
}

// RiskDatabasePage 提供风险摘要、过滤后的总数及分页偏移。
type RiskDatabasePage struct {
	DatabaseID                            string
	Items                                 []*RiskDatabaseItem
	Total, Offset, NextOffset, FieldLimit int
	HasMore                               bool
}

// Dump 输出风险摘要和分页信息，普通 Yak 脚本和 AI 工具均可直接 println。
// 返回值:
//   - text: 数据库标识、总数、续查偏移及各条风险的标题、目标、证据和修复建议
//
// Example:
// ```
// page = risk.QueryRiskInDatabase({"severity": "high"}, db.limit(5))~
// println(page.Dump())
// ```
func (p *RiskDatabasePage) Dump() string {
	var b strings.Builder
	fmt.Fprintf(&b, "database_id=%s total=%d offset=%d next_offset=%d has_more=%t hits=%d field_limit=%d characters\n", p.DatabaseID, p.Total, p.Offset, p.NextOffset, p.HasMore, len(p.Items), p.FieldLimit)
	for _, r := range p.Items {
		fmt.Fprintf(&b, "\n--- risk_id=%d hash=%s ---\ncreated_at=%s updated_at=%s\ntitle=%s\ntitle_verbose=%s\ntype=%s type_verbose=%s severity=%s\nurl=%s host=%s ip=%s port=%d\nparameter=%s payload=%s\ntags=%s source=%s runtime_id=%s waiting_verified=%t\ndescription:\n%s\nsolution:\n%s\ndetails (stored text):\n%s\n", r.ID, r.Hash, r.CreatedAt.Format(time.RFC3339), r.UpdatedAt.Format(time.RFC3339), r.Title, r.TitleVerbose, r.RiskType, r.RiskTypeVerbose, r.Severity, r.Url, r.Host, r.IP, r.Port, r.Parameter, r.Payload, r.Tags, r.FromYakScript, r.RuntimeId, r.WaitingVerified, r.Description, r.Solution, r.Details)
		if r.Truncated {
			fmt.Fprintf(&b, "[truncated: one or more text fields exceed %d characters; narrow filters or lower limit for larger excerpts]\n", p.FieldLimit)
		}
	}
	return b.String()
}

type riskDatabaseFilter struct {
	Type            string  `json:"type"`
	Severity        string  `json:"severity"`
	Title           string  `json:"title"`
	RuntimeID       string  `json:"runtime_id"`
	IDs             []int64 `json:"ids"`
	WaitingVerified bool    `json:"waiting_verified"`
}

func init() {
	// 公共导出供普通脚本使用；AI 引擎仅为它补充运行时数据库绑定。
	RiskExports["QueryRiskInDatabase"] = QueryRiskInDatabase
}

// QueryRiskInDatabase 分页搜索已保存的风险（导出名为 risk.QueryRiskInDatabase）。
// 普通脚本默认查询当前项目库，AI 工具默认查询本次运行绑定的项目库；两者均可用 db.projectID 选择 db.ListYakProjects 返回的数据库标识，跨项目只读，不切换当前库。
// 参数:
//   - filter: 过滤字典；type 精确匹配风险类型或中文类型，支持逗号列表；severity 支持逗号列表；title 为标题关键词；runtime_id 为精确运行 ID；ids 为正整数 ID 列表；waiting_verified 默认 false，查询未处于等待验证状态的记录，true 仅查待验证记录。nil 或空字典使用默认过滤。
//   - opts: db.projectID、db.keyword、db.url、db.limit、db.offset、db.afterID、db.beforeID；按 ID 降序，limit 默认 10，范围 1–100。
//
// 返回值:
//   - page: RiskDatabasePage；Items 为摘要，Total 为匹配总数，NextOffset 和 HasMore 用于续查。
//   - err: 无效过滤、数据库不可用、查询失败或上下文取消时返回错误。
//
// 关键词复用风险库过滤器，检索目标、标题、类型、参数、payload 和 details；另外包含 description 和 solution。摘要不读取原始报文。
// 所有文本列在 SQL 中截断；每列最多 2048 字符，随 limit 减小可增大摘要预算，Truncated 标记被截断的条目。
// Example:
// ```
// page = risk.QueryRiskInDatabase({"type": "sqli", "severity": "high"}, db.keyword("login"), db.limit(5))~
// println(page.Dump())
// ```
func QueryRiskInDatabase(filter map[string]any, opts ...DBHistoryOption) (*RiskDatabasePage, error) {
	c, err := historyConfig(0, opts)
	if err != nil {
		return nil, err
	}
	if c.methods != "" || c.status != "" || c.source != "" || c.packetLimit != 0 {
		return nil, fmt.Errorf("HTTP-specific options are not supported by risk queries")
	}
	var f riskDatabaseFilter
	raw, err := json.Marshal(filter)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&f); err != nil {
		return nil, fmt.Errorf("invalid risk filter: %w", err)
	}
	if len(f.Type) > 256 || len(f.Severity) > 256 || len(f.Title) > 1024 || len(f.RuntimeID) > 256 || len(f.IDs) > 100 {
		return nil, fmt.Errorf("risk filter exceeds its size limit")
	}
	for _, id := range f.IDs {
		if id <= 0 {
			return nil, fmt.Errorf("risk IDs must be positive")
		}
	}
	db, closeDB, err := historyProjectDatabase(c)
	if err != nil {
		return nil, err
	}
	defer closeDB()
	query := yakit.FilterByQueryRisks(db, &ypb.QueryRisksRequest{Severity: f.Severity, Title: f.Title, RuntimeId: f.RuntimeID, Ids: f.IDs, WaitingVerified: f.WaitingVerified})
	query = bizhelper.FuzzSearchEx(query, []string{"ip", "url", "title", "title_verbose", "risk_type", "risk_type_verbose", "parameter", "payload", "details", "description", "solution"}, c.keyword, false)
	if types := utils.PrettifyListFromStringSplitEx(f.Type); len(types) > 0 {
		query = query.Where("(risk_type IN (?) OR risk_type_verbose IN (?))", types, types)
	}
	if c.url != "" {
		query = query.Where("url LIKE ?", "%"+c.url+"%")
	}
	if c.afterID > 0 {
		query = query.Where("id > ?", c.afterID)
	}
	if c.beforeID > 0 {
		query = query.Where("id < ?", c.beforeID)
	}
	var total int
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	// SQLite substr counts Unicode characters. Reserve four bytes per character
	// so even a page containing only multibyte text stays within the text budget.
	columns := []string{"hash", "ip", "host", "url", "title", "title_verbose", "risk_type", "risk_type_verbose", "severity", "description", "solution", "parameter", "payload", "details", "tags", "from_yak_script", "runtime_id"}
	fieldLimit := 131072 / (c.limit * len(columns) * 4)
	if fieldLimit > 2048 {
		fieldLimit = 2048
	}
	projection := []string{"id", "created_at", "updated_at", "port", "waiting_verified"}
	truncated := []string{}
	for _, col := range columns {
		projection = append(projection, fmt.Sprintf("substr(%s, 1, %d) AS %s", col, fieldLimit, col))
		truncated = append(truncated, fmt.Sprintf("coalesce(length(%s), 0) > %d", col, fieldLimit))
	}
	projection = append(projection, "("+strings.Join(truncated, " OR ")+") AS truncated")
	items := []*RiskDatabaseItem{}
	if err := query.Select(strings.Join(projection, ", ")).Order("id DESC").Offset(c.offset).Limit(c.limit).Scan(&items).Error; err != nil {
		return nil, err
	}
	id := c.projectID
	if id == "" {
		id = "current"
	}
	return &RiskDatabasePage{DatabaseID: id, Items: items, Total: total, Offset: c.offset, NextOffset: c.offset + len(items), HasMore: c.offset+len(items) < total, FieldLimit: fieldLimit}, c.ctx.Err()
}
