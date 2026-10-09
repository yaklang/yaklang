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

// HTTPHistoryItem 是 HTTP 流量历史条目，嵌入 HTTPFlow 并补充展示预算、省略标记与文件导出能力。
// 分页查询只读取展示预算内的报文及选定元信息；Dump 输出解码后的原文，完整字段查询用 QueryHTTPFlowByID。
type HTTPHistoryItem struct {
	*schema.HTTPFlow
	DatabaseID                      string
	RequestBytes, ResponseBytes     int64
	RequestOmitted, ResponseOmitted bool
	config                          dbHistoryConfig
}

// HTTPHistoryPage 是按条件过滤后的 HTTP 流量分页结果，提供本页条目、总数与续查偏移。
type HTTPHistoryPage struct {
	Items                     []*HTTPHistoryItem
	Total, Offset, NextOffset int
	HasMore                   bool
}

// Dump 把分页信息与各条 HTTP 流量格式化为可直接 println 的文本。
// 请求和响应保持原始引号与换行；大报文显示省略提示，不进行 JSON quoted 序列化。
//
// 返回值:
//   - text: 总数、分页偏移、续查标记及每条流量的 Dump 文本
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.limit(5))~
// println(page.Dump())
// ```
func (p *HTTPHistoryPage) Dump() string {
	var b strings.Builder
	fmt.Fprintf(&b, "total=%d offset=%d next_offset=%d has_more=%t hits=%d\n", p.Total, p.Offset, p.NextOffset, p.HasMore, len(p.Items))
	for _, item := range p.Items {
		b.WriteString(item.Dump())
		b.WriteByte('\n')
	}
	return b.String()
}

// Dump 把单条流量的元信息和预算内请求/响应输出为原始报文文本。
// 超预算或旁路存储的报文只显示省略标记、存储大小及按 ID 获取的提示，完整内容用 ExportPackets 导出。
//
// 返回值:
//   - text: ID、数据库标识、URL、方法、状态码、来源、日期与原始报文或省略提示
//
// Example:
// ```
// page = db.QueryHTTPFlows(db.limit(1))~
// for item in page.Items { println(item.Dump()) }
// ```
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

// QueryHTTPFlows 按条件分页查询 HTTP 流量历史（导出名为 db.QueryHTTPFlows）。
//
// 默认查询本次运行绑定的当前项目数据库；db.projectID 可选择 db.ListYakProjects 返回的 DatabaseID，其他项目以只读方式打开，不切换当前库。
// 复用引擎 HTTPFlow 过滤器，按 ID 降序返回。关键词搜索数据库中的 URL、请求、响应等字段，不扫描大报文旁路文件的正文；该正文需按 ID 获取后导出搜索。
// 默认 limit=10、offset=0、packetLimit=2048；只将预算内的请求/响应读取到结果，大报文以 RequestOmitted/ResponseOmitted 标明，不能把空字段理解为完整空报文。
//
// 参数:
//   - opts: 查询选项（可变参数），支持 db.projectID、db.keyword、db.url、db.methods、db.statusCode、db.sourceType、db.afterID、db.beforeID、db.limit、db.offset、db.packetLimit
//
// 返回值:
//   - page: HTTPHistoryPage 分页结果；Items 为 HTTPHistoryItem 列表，Total 为过滤后总数，Offset 为本次偏移，NextOffset 为续查偏移，HasMore 表示还有下一页；无匹配时 Items 为空
//   - err: 数据库不可用、未知项目标识、选项无效、查询失败或上下文取消时返回错误
//
// packetLimit 范围为 1–32768 字节，预算按数据库中的 Go quoted 存储字节计算；limit*packetLimit*2 不得超过 128 KiB。
// page.Dump() 输出原始请求/响应文本，保留引号和换行；省略的报文可通过 Items 中条目的 ExportPackets 完整导出。
//
// <|EXAMPLE_START|> 保存流量、过滤查询并展示原文
// ```
// host = "doc-http-history.example.test"
// req = []byte(f"POST /api/orders HTTP/1.1\r\nHost: ${host}\r\n\r\n")
// rsp = []byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 4\r\n\r\nCSRF")
// db.SaveHTTPFlowFromRawWithType("http://"+host+"/api/orders", req, rsp, "mitm")~
// page, err = db.QueryHTTPFlows(db.url(host), db.methods("POST"), db.statusCode("400-499"), db.sourceType("mitm"), db.limit(5))
// if err != nil { die(err) }
// println(page.Dump())
// assert len(page.Items) >= 1, "saved flow should match the query"
// ```
// <|EXAMPLE_END|>
//
// <|EXAMPLE_START|> 使用返回的项目标识与分页偏移
// ```
//
//	for project in db.ListYakProjects()~ {
//	    if project.Current && project.Available && project.SupportsHTTP {
//	        page = db.QueryHTTPFlows(db.projectID(project.DatabaseID), db.limit(1))~
//	        println(page.Dump())
//	        if page.HasMore {
//	            next = db.QueryHTTPFlows(db.projectID(project.DatabaseID), db.limit(1), db.offset(page.NextOffset))~
//	            println(next.Dump())
//	        }
//	        break
//	    }
//	}
//
// ```
// <|EXAMPLE_END|>
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
// 批量按多个 ID 取用 db.QueryHTTPFlowsByID，条件分页查询用 db.QueryHTTPFlows。
// db.projectID 接受 db.ListYakProjects 返回的 DatabaseID；跨库查询只读打开，不切换当前项目。
// 保留已有 HTTPFlow 返回类型、完整存储字段与方法，可继续传入 db.SaveHTTPFlowInstance。大报文的旁路资源仍由原 HTTPFlow 字段描述。
// 本函数不支持展示预算；需要按预算展示、省略标记或完整文件导出时，使用 db.QueryHTTPFlows 返回的历史条目。
//
// 参数:
//   - id: HTTPFlow 的数据库 ID，必须大于 0；该 ID 只在对应数据库内有效
//   - opts: 可变查询选项，支持 db.projectID；默认当前库。非零 db.packetLimit 会报错，避免返回不完整的 HTTPFlow
//
// 返回值:
//   - flow: 原 HTTPFlow 对象，保留字段及 GetRequest、GetResponse、AddTag 等方法，可传给其他要求 HTTPFlow 类型的接口
//   - err: 数据库不可用、项目标识无效、选项无效、ID 不存在或上下文取消时返回错误
//
// <|EXAMPLE_START|> 保留已有按 ID 查询的调用方式
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
//	    assert str.Contains(one.GetResponse(), "200 OK")
//	    one.AddTag("doc-compatibility")
//	    db.SaveHTTPFlowInstance(one)~
//	    assert str.Contains(db.QueryHTTPFlowByID(id)~.Tags, "doc-compatibility")
//	}
//
// ```
// <|EXAMPLE_END|>
//
// <|EXAMPLE_START|> 小预算展示和完整文件导出
// ```
// host = "doc-http-export.example.test"
// req = []byte(f"GET /large HTTP/1.1\r\nHost: ${host}\r\n\r\n")
// rsp = []byte("HTTP/1.1 200 OK\r\n\r\n" + str.Repeat("raw evidence\n", 500))
// db.SaveHTTPFlowFromRaw("http://"+host+"/large", req, rsp)~
// page = db.QueryHTTPFlows(db.url(host), db.limit(1), db.packetLimit(256))~
// assert len(page.Items) == 1
// item = page.Items[0]
// println(item.Dump())
// // 保存接口可能补全 Content-Length；按数据库实际原文核对导出。
// expectedResponse = db.QueryHTTPFlowByID(item.ID)~.GetResponse()
// files = item.ExportPackets("", "both")~
//
//	for packetFile in files {
//	    println(packetFile.Dump())
//	    if packetFile.Part == "response" {
//	        assert string(file.ReadFile(packetFile.Path)~) == expectedResponse
//	    }
//	    file.Remove(packetFile.Path)~
//	}
//
// ```
// <|EXAMPLE_END|>
func QueryHTTPFlowByID(id int64, opts ...DBHistoryOption) (*schema.HTTPFlow, error) {
	if id < 1 {
		return nil, fmt.Errorf("HTTP flow ID must be positive")
	}
	// Preserve both the concrete HTTPFlow type and all stored fields for
	// existing callers. Bounded AI output uses QueryHTTPFlows history items.
	c, err := historyConfig(0, opts)
	if err != nil {
		return nil, err
	}
	if c.packetLimit != 0 {
		return nil, fmt.Errorf("QueryHTTPFlowByID preserves complete HTTPFlow fields; use QueryHTTPFlows for bounded history")
	}
	db, closeDB, err := historyProjectDatabase(c)
	if err != nil {
		return nil, err
	}
	defer closeDB()
	flow, err := yakit.GetHTTPFlow(db, id)
	if err != nil {
		return nil, err
	}
	return flow, c.ctx.Err()
}
