package yaklib

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type httpHistoryRecord struct {
	schema.HTTPFlow
	StoredRequestBytes, StoredResponseBytes int64
}

func (*httpHistoryRecord) TableName() string { return "http_flows" }

// HTTPHistoryItem retains normal HTTPFlow fields and adds bounded text and file
// export. The database stores Go-quoted packets; Dump always emits raw text.
type HTTPHistoryItem struct {
	*schema.HTTPFlow
	DatabaseID                      string
	RequestBytes, ResponseBytes     int64
	RequestOmitted, ResponseOmitted bool
	config                          dbHistoryConfig
}
type HTTPHistoryPage struct {
	Items                     []*HTTPHistoryItem
	Total, Offset, NextOffset int
	HasMore                   bool
}

func (p *HTTPHistoryPage) Dump() string {
	var b strings.Builder
	fmt.Fprintf(&b, "total=%d offset=%d next_offset=%d has_more=%t hits=%d\n", p.Total, p.Offset, p.NextOffset, p.HasMore, len(p.Items))
	for _, item := range p.Items {
		b.WriteString(item.Dump())
		b.WriteByte('\n')
	}
	return b.String()
}
func (p *HTTPHistoryItem) Dump() string {
	var b strings.Builder
	fmt.Fprintf(&b, "http_flow_id=%d database_id=%s method=%s status_code=%d url=%s\nsource=%s created_at=%s updated_at=%s\n", p.ID, p.DatabaseID, p.Method, p.StatusCode, p.Url, p.SourceType, p.CreatedAt.Format("2006-01-02 15:04:05"), p.UpdatedAt.Format("2006-01-02 15:04:05"))
	for _, part := range []struct {
		name, packet string
		bytes        int64
		omitted      bool
	}{{"request", p.GetRequest(), p.RequestBytes, p.RequestOmitted}, {"response", p.GetResponse(), p.ResponseBytes, p.ResponseOmitted}} {
		if part.omitted {
			fmt.Fprintf(&b, "[%s omitted: stored_bytes=%d; use fetch_http_packet_by_id with id=%d and database_id=%s]\n", part.name, part.bytes, p.ID, p.DatabaseID)
		} else {
			fmt.Fprintf(&b, "--- %s (%d bytes) ---\n%s\n", part.name, len([]byte(part.packet)), part.packet)
		}
	}
	return b.String()
}
func historyProjection(c *dbHistoryConfig) string {
	if c.packetLimit == 0 {
		return "*, length(CAST(request AS BLOB)) AS stored_request_bytes, length(CAST(response AS BLOB)) AS stored_response_bytes"
	}
	packet := func(column string) string {
		if c.packetLimit == 0 {
			return column
		}
		return fmt.Sprintf("CASE WHEN length(CAST(%s AS BLOB)) <= %d THEN %s ELSE '' END AS %s", column, c.packetLimit, column, column)
	}
	return "id, created_at, updated_at, url, method, status_code, source_type, request_length, body_length, is_http_s, is_too_large_request, too_large_request_header_file, too_large_request_body_file, is_too_large_response, is_read_too_slow_response, too_large_response_header_file, too_large_response_body_file, " + packet("request") + ", " + packet("response") + ", length(CAST(request AS BLOB)) AS stored_request_bytes, length(CAST(response AS BLOB)) AS stored_response_bytes"
}
func historyItem(row *httpHistoryRecord, c *dbHistoryConfig) *HTTPHistoryItem {
	id := c.projectID
	if id == "" {
		id = "current"
	}
	return &HTTPHistoryItem{HTTPFlow: &row.HTTPFlow, DatabaseID: id, RequestBytes: row.StoredRequestBytes, ResponseBytes: row.StoredResponseBytes,
		RequestOmitted:  row.IsTooLargeRequest || row.TooLargeRequestBodyFile != "" || (c.packetLimit > 0 && row.StoredRequestBytes > int64(c.packetLimit)),
		ResponseOmitted: row.IsTooLargeResponse || row.IsReadTooSlowResponse || row.TooLargeResponseBodyFile != "" || (c.packetLimit > 0 && row.StoredResponseBytes > int64(c.packetLimit)), config: *c}
}

// QueryHTTPFlows reuses the engine's HTTPFlow filter; only bounded packets enter
// a list page. projectID accepts identifiers returned by ListYakProjects.
func QueryHTTPFlows(opts ...DBHistoryOption) (*HTTPHistoryPage, error) {
	c, err := historyConfig(2048, opts)
	if err != nil {
		return nil, err
	}
	if c.packetLimit == 0 || c.limit*c.packetLimit*2 > 131072 {
		return nil, fmt.Errorf("query packet limit must be positive and total packet budget at most 128 KiB")
	}
	db, closeDB, err := historyProjectDatabase(c)
	if err != nil {
		return nil, err
	}
	defer closeDB()
	query := yakit.FilterHTTPFlow(db, &ypb.QueryHTTPFlowRequest{Keyword: c.keyword, SearchURL: c.url, Methods: c.methods, StatusCode: c.status, SourceType: c.source, AfterId: c.afterID, BeforeId: c.beforeID})
	var total int
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []*httpHistoryRecord
	if err := query.Select(historyProjection(c)).Order("id DESC").Offset(c.offset).Limit(c.limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	page := &HTTPHistoryPage{Total: total, Offset: c.offset, NextOffset: c.offset + len(rows), HasMore: c.offset+len(rows) < total, Items: []*HTTPHistoryItem{}}
	for _, row := range rows {
		page.Items = append(page.Items, historyItem(row, c))
	}
	return page, c.ctx.Err()
}

// QueryHTTPFlowByID 按数据库自增 ID 精确查询单条 HTTP 流量（导出名为 db.QueryHTTPFlowByID）
//
// 当你已经知道某条流量的 ID（例如从列表/表格中选中、或从其他查询里拿到 flow.ID）时，用它直接取回完整对象。
// 批量按多个 ID 取用 db.QueryHTTPFlowsByID。
//
// 参数:
//   - id: HTTPFlow 的数据库 ID
//
// 返回值:
//   - HTTPFlow 字段及可 Dump/ExportPackets 的历史条目
//   - 错误信息（数据库不可用或该 ID 不存在时返回）
//
// Example:
// ```
// // 先落一条流量，从遍历结果拿到它的 ID，再按 ID 精确取回（保存->拿ID->按ID查 联动）
// host = "doc-demo-byid.example.com"
// db.SaveHTTPFlowFromRaw("http://"+host+"/", []byte(f"GET / HTTP/1.1\r\nHost: ${host}\r\n\r\n"), []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))~
//
// id = 0
// for flow in db.QueryHTTPFlowsByKeyword(host) { id = flow.ID; break }
//
//	if id > 0 {
//	    one = db.QueryHTTPFlowByID(id)~
//	    println(one.Url)
//	    assert one.ID == id, "QueryHTTPFlowByID should return the same record"
//	}
//
// ```
func QueryHTTPFlowByID(id int64, opts ...DBHistoryOption) (*HTTPHistoryItem, error) {
	if id < 1 {
		return nil, fmt.Errorf("HTTP flow ID must be positive")
	}
	// No packetLimit preserves the old QueryHTTPFlowByID field contents for
	// standalone callers. AI tools explicitly supply a bounded packetLimit.
	c, err := historyConfig(0, opts)
	if err != nil {
		return nil, err
	}
	db, closeDB, err := historyProjectDatabase(c)
	if err != nil {
		return nil, err
	}
	defer closeDB()
	// Reuse GetHTTPFlow's exact-ID lookup while keeping oversized columns out of memory.
	flow, err := yakit.GetHTTPFlow(db.Select(historyProjection(c)), id)
	if err != nil {
		return nil, err
	}
	var lengths struct{ StoredRequestBytes, StoredResponseBytes int64 }
	if err := db.Model(&schema.HTTPFlow{}).Select("length(CAST(request AS BLOB)) AS stored_request_bytes, length(CAST(response AS BLOB)) AS stored_response_bytes").Where("id = ?", id).Scan(&lengths).Error; err != nil {
		return nil, err
	}
	return historyItem(&httpHistoryRecord{HTTPFlow: *flow, StoredRequestBytes: lengths.StoredRequestBytes, StoredResponseBytes: lengths.StoredResponseBytes}, c), c.ctx.Err()
}
