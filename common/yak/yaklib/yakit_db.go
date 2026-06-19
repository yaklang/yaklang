package yaklib

import (
	"context"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// queryDomainAssetByNetwork 按网段查询域名资产（导出名为 db.QueryDomainsByNetwork）
// 参数:
//   - network: 网段表达式，如 "192.168.1.0/24"
//
// 返回值:
//   - 域名资产的 channel，可使用 for-range 遍历
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// ch = db.QueryDomainsByNetwork("192.168.1.0/24")~
// for domain := range ch { println(domain.Domain) }
// ```
func queryDomainAssetByNetwork(network string) (chan *schema.Domain, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("domains")
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", network)
	return yakit.YieldDomains(db, context.Background()), nil
}

// queryDomainAssetByDomainKeyword 按域名关键词模糊查询域名资产（导出名为 db.QueryDomainsByDomainKeyword）
// 参数:
//   - keyword: 域名关键词
//
// 返回值:
//   - 域名资产的 channel，可使用 for-range 遍历
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// ch = db.QueryDomainsByDomainKeyword("example.com")~
// for domain := range ch { println(domain.Domain) }
// ```
func queryDomainAssetByDomainKeyword(keyword string) (chan *schema.Domain, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("domains")
	db = bizhelper.FuzzQueryLike(db, "domain", keyword)
	return yakit.YieldDomains(db, context.Background()), nil
}

// queryDomainAssetByHTMLTitle 按 HTML 标题模糊查询域名资产（导出名为 db.QueryDomainsByTitle）
// 参数:
//   - title: HTML 标题关键词
//
// 返回值:
//   - 域名资产的 channel，可使用 for-range 遍历
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// ch = db.QueryDomainsByTitle("admin")~
// for domain := range ch { println(domain.Domain) }
// ```
func queryDomainAssetByHTMLTitle(title string) (chan *schema.Domain, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("domains")
	db = bizhelper.FuzzQueryLike(db, "html_title", title)
	return yakit.YieldDomains(db, context.Background()), nil
}

// queryHostAssetByNetwork 按网段查询主机资产（导出名为 db.QueryHostPortByKeyword 等相关接口的底层实现之一）
// 参数:
//   - network: 网段表达式，如 "192.168.1.0/24"
//
// 返回值:
//   - 主机资产的 channel，可使用 for-range 遍历
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// ch = db.QueryHostsByNetwork("192.168.1.0/24")~
// for host := range ch { println(host.IP) }
// ```
func queryHostAssetByNetwork(network string) (chan *schema.Host, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("hosts")
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", network)
	return yakit.YieldHosts(db, context.Background()), nil
}

// queryHostAssetByDomainKeyword 按关联域名关键词查询主机资产（导出名为 db.QueryHostsByDomain）
// 参数:
//   - keyword: 域名关键词
//
// 返回值:
//   - 主机资产的 channel，可使用 for-range 遍历
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// ch = db.QueryHostsByDomain("example.com")~
// for host := range ch { println(host.IP) }
// ```
func queryHostAssetByDomainKeyword(keyword string) (chan *schema.Host, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("hosts")
	db = bizhelper.FuzzQueryLike(db, "domains", keyword)
	return yakit.YieldHosts(db, context.Background()), nil
}

func queryPortAssetByNetwork(network string) (chan *schema.Port, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&schema.Port{})
	db = bizhelper.ExactQueryString(db, "state", "open")
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", network)
	return yakit.YieldPorts(db, context.Background()), nil
}

func queryPortAssetByNetworkAndPort(network string, port string) (chan *schema.Port, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&schema.Port{})
	db = bizhelper.ExactQueryString(db, "state", "open")
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", network)
	db = bizhelper.QueryBySpecificPorts(db, "port", port)
	return yakit.YieldPorts(db, context.Background()), nil
}

func queryPortAssetByKeyword(keyword string) (chan *schema.Port, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&schema.Port{})
	db = bizhelper.ExactQueryString(db, "state", "open")
	db = bizhelper.FuzzSearchEx(db, []string{
		"host", "service_type",
		"fingerprint", "cpe", "html_title",
	}, keyword, false)
	return yakit.YieldPorts(db, context.Background()), nil
}

func queryHostPortByTarget(target string) chan string {
	ch := make(chan string, 100)
	go func() {
		defer close(ch)
		res, err := queryPortAssetByKeyword(target)
		if err != nil {
			return
		}
		for r := range res {
			ch <- utils.HostPort(r.Host, r.Port)
		}
	}()
	return ch
}

// queryHostPortByNetwork 按网段查询开放端口并以 host:port 字符串返回（导出名为 db.QueryHostPortByNetwork）
// 参数:
//   - network: 网段表达式，如 "192.168.1.0/24"
//
// 返回值:
//   - host:port 字符串的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
//
//	for hostport := range db.QueryHostPortByNetwork("192.168.1.0/24") {
//	    println(hostport)
//	}
//
// ```
func queryHostPortByNetwork(network string) chan string {
	ch := make(chan string, 100)
	go func() {
		defer close(ch)
		res, err := queryPortAssetByNetwork(network)
		if err != nil {
			return
		}
		for r := range res {
			ch <- utils.HostPort(r.Host, r.Port)
		}
	}()
	return ch
}

func queryHostPortByPort(port string) chan string {
	ch := make(chan string, 100)
	go func() {
		defer close(ch)
		res, err := queryPortAssetByNetworkAndPort("", port)
		if err != nil {
			return
		}
		for r := range res {
			ch <- utils.HostPort(r.Host, r.Port)
		}
	}()
	return ch
}

func queryHostPortByNetworkAndPort(network, port string) chan string {
	ch := make(chan string, 100)
	go func() {
		defer close(ch)
		res, err := queryPortAssetByNetworkAndPort(network, port)
		if err != nil {
			return
		}
		for r := range res {
			ch <- utils.HostPort(r.Host, r.Port)
		}
	}()
	return ch
}

func queryHostPortAll() chan string {
	ch := make(chan string, 100)
	go func() {
		defer close(ch)
		res, err := queryPortAssetByNetworkAndPort("", "")
		if err != nil {
			return
		}
		for r := range res {
			ch <- utils.HostPort(r.Host, r.Port)
		}
	}()
	return ch
}

// saveCrawler 将一次 HTTP 请求/响应作为爬虫流量保存到项目数据库（导出名为 db.SaveHTTPFlowFromNative）
// 参数:
//   - url: 请求 URL
//   - req: HTTP 请求对象
//   - rsp: HTTP 响应对象
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库与 http 请求对象（示意性示例）
// db.SaveHTTPFlowFromNative("http://example.com", req, rsp)~
// ```
func saveCrawler(url string, req *http.Request, rsp *http.Response) error {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return utils.Errorf("cannot found database")
	}

	_, err := yakit.SaveFromHTTP(db, strings.HasPrefix(url, "https"), req, rsp, "basic-crawler", url, req.RemoteAddr)
	if err != nil {
		return err
	}
	return nil
}

// saveHTTPFlowWithType 将一次 HTTP 请求/响应按指定来源类型保存到项目数据库（导出名为 db.SaveHTTPFlowFromNativeWithType）
// 参数:
//   - url: 请求 URL
//   - req: HTTP 请求对象
//   - rsp: HTTP 响应对象
//   - typeStr: 流量来源类型，空字符串默认为 "mitm"
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库与 http 请求对象（示意性示例）
// db.SaveHTTPFlowFromNativeWithType("http://example.com", req, rsp, "mitm")~
// ```
func saveHTTPFlowWithType(url string, req *http.Request, rsp *http.Response, typeStr string) error {
	if typeStr == "" {
		typeStr = "mitm"
	}
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return utils.Errorf("cannot found database")
	}

	_, err := yakit.SaveFromHTTP(db, strings.HasPrefix(url, "https"), req, rsp, typeStr, url, req.RemoteAddr)
	if err != nil {
		return err
	}
	return nil
}

// saveHTTPFlowInstance 直接保存一个已构造好的 HTTPFlow 对象到项目数据库（导出名为 db.SaveHTTPFlowInstance）
// 参数:
//   - flow: HTTPFlow 对象
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库与 flow 对象（示意性示例）
// db.SaveHTTPFlowInstance(flow)~
// ```
func saveHTTPFlowInstance(flow *schema.HTTPFlow) error {
	return yakit.SaveHTTPFlow(consts.GetGormProjectDatabase(), flow)
}

// saveDomain 保存域名及其关联 IP 到项目数据库（导出名为 db.SaveDomain）
// 参数:
//   - domain: 域名
//   - ip: 零个或多个关联 IP
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// db.SaveDomain("example.com", "93.184.216.34")~
// ```
func saveDomain(domain string, ip ...string) error {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return utils.Errorf("cannot found database")
	}
	wg := new(sync.WaitGroup)
	wg.Add(len(ip))
	for _, ipSingle := range ip {
		ipSingle := ipSingle
		go func() {
			defer wg.Done()
			err := yakit.SaveDomain(db, domain, ipSingle)
			if err != nil {
				log.Error(err)
			}
		}()
	}
	wg.Wait()
	return nil
}

func interfaceToPort(t interface{}) (*schema.Port, error) {
	var r *schema.Port
	switch ret := t.(type) {
	case *fp.MatchResult:
		r = NewPortFromMatchResult(ret)
	case *synscan.SynScanResult:
		r = NewPortFromSynScanResult(ret)
	case *base.NetSpaceEngineResult:
		r = NewPortFromSpaceEngineResult(ret)
	case *schema.Port:
		r = ret
	default:
		return nil, utils.Errorf("unsupported(%v): %#v", reflect.TypeOf(t), spew.Sdump(t))
	}
	return r, nil
}

// savePortFromObj 从扫描结果对象提取端口信息并保存到项目数据库（导出名为 db.SavePortFromResult）
// 支持的对象类型包括 fp.MatchResult、synscan.SynScanResult、空间引擎结果以及 schema.Port
// 参数:
//   - t: 扫描结果对象
//   - RuntimeId: 可选的运行时 ID
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库与扫描结果对象（示意性示例）
// db.SavePortFromResult(result)~
// ```
func savePortFromObj(t interface{}, RuntimeId ...string) error {
	r, err := interfaceToPort(t)
	if err != nil {
		return err
	}
	if len(RuntimeId) > 0 {
		r.RuntimeId = RuntimeId[0]
	}

	return yakit.CreateOrUpdatePort(consts.GetGormProjectDatabase(), r.CalcHash(), r)
}

// queryUrlsByKeyword 按关键词模糊查询 URL 资产（导出名为 db.QueryUrlsByKeyword）
// 参数:
//   - k: URL 关键词
//
// 返回值:
//   - URL 字符串的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
//
//	for u := range db.QueryUrlsByKeyword("login") {
//	    println(u)
//	}
//
// ```
func queryUrlsByKeyword(k string) chan string {
	ch := make(chan string, 100)
	go func() {
		defer close(ch)
		db := consts.GetGormProjectDatabase()
		if db == nil {
			return
		}
		db = db.Select("url").Table("http_flows").Where("url LIKE ?", `%`+k+`%`)
		for u := range yakit.YieldHTTPUrl(db, context.Background()) {
			ch <- u.Url
		}
	}()
	return ch
}

// queryHTTPFlowByKeyword 按关键词在 URL/请求/响应中模糊查询 HTTP 流量（导出名为 db.QueryHTTPFlowsByKeyword）
// 参数:
//   - k: 关键词
//
// 返回值:
//   - HTTPFlow 对象的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
//
//	for flow := range db.QueryHTTPFlowsByKeyword("password") {
//	    println(flow.Url)
//	}
//
// ```
func queryHTTPFlowByKeyword(k string) chan *schema.HTTPFlow {
	ch := make(chan *schema.HTTPFlow, 100)
	go func() {
		defer close(ch)
		db := consts.GetGormProjectDatabase()
		if db == nil {
			return
		}
		db = bizhelper.FuzzSearchEx(db, []string{
			"url", "request", "response",
		}, k, false)
		for u := range yakit.YieldHTTPFlows(db, context.Background()) {
			ch <- u
		}
	}()
	return ch
}

// queryPortsByUpdatedAt 查询指定时间戳之后更新的开放端口（导出名为 db.QueryPortsByUpdatedAt）
// 参数:
//   - timestamp: Unix 时间戳，仅返回此后更新的端口
//
// 返回值:
//   - 端口资产的 channel，可使用 for-range 遍历
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// ch = db.QueryPortsByUpdatedAt(time.Now().Unix() - 3600)~
// for port := range ch { println(port.Host, port.Port) }
// ```
func queryPortsByUpdatedAt(timestamp int64) (chan *schema.Port, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&schema.Port{})
	db = bizhelper.ExactQueryString(db, "state", "open")
	db = bizhelper.QueryDateTimeAfterTimestampOr(db, "updated_at", timestamp)
	db = bizhelper.FuzzSearchEx(db, []string{
		"host", "service_type",
		"fingerprint", "cpe", "html_title",
	}, "", false)
	return yakit.YieldPorts(db, context.Background()), nil
}

// queryPortsByTaskName 按任务名称查询端口资产（导出名为 db.QueryPortsByTaskName）
// 参数:
//   - taskName: 任务名称
//
// 返回值:
//   - 端口资产的 channel，可使用 for-range 遍历
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// ch = db.QueryPortsByTaskName("scan-task-1")~
// for port := range ch { println(port.Host, port.Port) }
// ```
func queryPortsByTaskName(taskName string) (chan *schema.Port, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&schema.Port{})
	db = bizhelper.ExactQueryString(db, "task_name", taskName)
	db = bizhelper.FuzzSearchEx(db, []string{
		"host", "service_type",
		"fingerprint", "cpe", "html_title",
	}, "", false)
	return yakit.YieldPorts(db, context.Background()), nil
}

// queryPortsByRuntimeId 按运行时 ID 查询端口资产（导出名为 db.QueryPortsByRuntimeId）
// 参数:
//   - runtimeID: 运行时 ID
//
// 返回值:
//   - 端口资产的 channel，可使用 for-range 遍历
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// ch = db.QueryPortsByRuntimeId("xxxx-runtime-id")~
// for port := range ch { println(port.Host, port.Port) }
// ```
func queryPortsByRuntimeId(runtimeID string) (chan *schema.Port, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&schema.Port{})
	db = bizhelper.ExactQueryString(db, "runtime_id", runtimeID)
	db = bizhelper.FuzzSearchEx(db, []string{
		"host", "service_type",
		"fingerprint", "cpe", "html_title",
	}, "", false)
	return yakit.YieldPorts(db, context.Background()), nil
}

// queryHTTPFlowsByID 按一个或多个 ID 查询 HTTP 流量（导出名为 db.QueryHTTPFlowsByID）
// 参数:
//   - id: 一个或多个 HTTPFlow ID
//
// 返回值:
//   - HTTPFlow 对象的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
//
//	for flow := range db.QueryHTTPFlowsByID(1, 2, 3) {
//	    println(flow.Url)
//	}
//
// ```
func queryHTTPFlowsByID(id ...int64) chan *schema.HTTPFlow {
	ch := make(chan *schema.HTTPFlow, 100)
	go func() {
		defer close(ch)
		db := consts.GetGormProjectDatabase()
		if db == nil {
			return
		}
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", id)
		for u := range yakit.YieldHTTPFlows(db, context.Background()) {
			ch <- u
		}
	}()
	return ch
}

// queryHTTPFlowByID 按 ID 查询单条 HTTP 流量（导出名为 db.QueryHTTPFlowByID）
// 参数:
//   - id: HTTPFlow ID
//
// 返回值:
//   - HTTPFlow 对象
//   - 错误信息
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
// flow = db.QueryHTTPFlowByID(1)~
// println(flow.Url)
// ```
func queryHTTPFlowByID(id int64) (*schema.HTTPFlow, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("Query HTTPFlow By ID Failed: cannot found database")
	}
	var flow schema.HTTPFlow
	db.Model(&schema.HTTPFlow{}).Where("id = ?", id).First(&flow)
	if db.Error != nil {
		return nil, utils.Errorf("Query HTTPFlow By ID Failed: %s", db.Error)
	}
	return &flow, nil
}

// queryAllUrls 查询全部 URL 资产（导出名为 db.QueryUrlsAll）
// 参数:
//   - 无
//
// 返回值:
//   - URL 字符串的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 依赖本地项目数据库（示意性示例）
//
//	for u := range db.QueryUrlsAll() {
//	    println(u)
//	}
//
// ```
func queryAllUrls() chan string {
	return queryUrlsByKeyword("")
}

// savePayloads 将一组 payload 保存到指定字典组（导出名为 db.SavePayload）
// 参数:
//   - group: 字典组名
//   - payloadRaw: payload 内容（字符串或字符串列表）
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地数据库（示意性示例）
// db.SavePayload("my-group", ["admin", "root", "test"])~
// ```
func savePayloads(group string, payloadRaw any) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	payloads := utils.InterfaceToStringSlice(payloadRaw)
	return yakit.SavePayloadGroup(consts.GetGormProfileDatabase(), group, payloads)
}

// savePayloadByFile 从文件读取内容并保存到指定字典组（导出名为 db.SavePayloadByFile）
// 参数:
//   - group: 字典组名
//   - fileName: 文件路径
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地数据库与字典文件（示意性示例）
// db.SavePayloadByFile("my-group", "/tmp/dict.txt")~
// ```
func savePayloadByFile(group string, fileName string) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	return yakit.SavePayloadByFilename(consts.GetGormProfileDatabase(), group, fileName)
}

// deletePayloadByGroup 删除指定字典组及其全部 payload（导出名为 db.DeletePayloadByGroup）
// 参数:
//   - group: 字典组名
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地数据库（示意性示例）
// db.DeletePayloadByGroup("my-group")~
// ```
func deletePayloadByGroup(group string) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	return yakit.DeletePayloadByGroup(consts.GetGormProfileDatabase(), group)
}

// getPayloadGroups 查询匹配指定名称的字典组列表（导出名为 db.QueryPayloadGroups）
// 参数:
//   - group: 字典组名（可用于过滤）
//
// 返回值:
//   - 字典组名列表
//
// Example:
// ```
// // 依赖本地数据库（示意性示例）
// groups = db.QueryPayloadGroups("")
// dump(groups)
// ```
func getPayloadGroups(group string) []string {
	if consts.GetGormProfileDatabase() == nil {
		log.Error("no database connections")
		return nil
	}
	return yakit.PayloadGroups(consts.GetGormProfileDatabase(), group)
}

// getAllPayloadGroupsName 获取全部字典组名称（导出名为 db.GetAllPayloadGroupsName）
// 参数:
//   - 无
//
// 返回值:
//   - 全部字典组名列表
//
// Example:
// ```
// // 依赖本地数据库（示意性示例）
// groups = db.GetAllPayloadGroupsName()
// dump(groups)
// ```
func getAllPayloadGroupsName() []string {
	if consts.GetGormProfileDatabase() == nil {
		log.Error("no database connections")
		return nil
	}

	if allGroupName, err := yakit.GetAllPayloadGroupName(consts.GetGormProfileDatabase()); err != nil {
		return nil
	} else {
		return allGroupName
	}
}

// YieldPayload 以 channel 形式遍历一个或多个字典组中的 payload 内容（导出名为 db.YieldPayload）
// 参数:
//   - raw: 字典组名
//   - extra: 额外的字典组名
//
// 返回值:
//   - payload 内容的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 依赖本地数据库（示意性示例）
//
//	for payload := range db.YieldPayload("my-group") {
//	    println(payload)
//	}
//
// ```
func YieldPayload(raw any, extra ...any) chan string {
	db := consts.GetGormProfileDatabase().Model(&schema.Payload{})
	results := make([]any, 0, 1+len(extra))
	results = append(results, raw)
	for _, e := range extra {
		results = append(results, e)
	}
	db = bizhelper.ExactOrQueryArrayOr(db, "`group`", results)
	c := make(chan string)
	go func() {
		defer close(c)
		for p := range yakit.YieldPayloads(db, context.Background()) {
			if content := p.Content; content == nil {
				continue
			} else {
				res, err := strconv.Unquote(*p.Content)
				if err != nil {
					continue
				}
				c <- res
			}
		}
	}()
	return c
}

func init() {
	YakitExports["SaveHTTPFlow"] = saveCrawler
	YakitExports["SavePortFromResult"] = savePortFromObj
	YakitExports["SaveDomain"] = saveDomain
	YakitExports["SavePayload"] = savePayloads
	YakitExports["SavePayloadByFile"] = savePayloadByFile

	// 对象转port对象
	YakitExports["ObjToPort"] = interfaceToPort

	// HTTP 资产
	YakitExports["QueryUrlsByKeyword"] = queryUrlsByKeyword
	YakitExports["QueryUrlsAll"] = queryAllUrls
	YakitExports["QueryHTTPFlowsByKeyword"] = queryHTTPFlowByKeyword
	YakitExports["QueryHTTPFlowsAll"] = func() chan *schema.HTTPFlow {
		return queryHTTPFlowByKeyword("")
	}

	// Host:Port 资产
	YakitExports["QueryHostPortByNetwork"] = queryHostPortByNetwork
	YakitExports["QueryHostPortByKeyword"] = queryHostPortByTarget
	YakitExports["QueryHostPortByNetworkAndPort"] = queryHostPortByNetworkAndPort
	YakitExports["QueryHostPortAll"] = queryHostPortAll

	// 查询端口，主机与域名
	YakitExports["QueryPortAssetByNetwork"] = queryPortAssetByNetwork
	YakitExports["QueryHostsByNetwork"] = queryHostAssetByNetwork
	YakitExports["QueryHostsByDomain"] = queryHostAssetByDomainKeyword
	YakitExports["QueryDomainsByNetwork"] = queryDomainAssetByNetwork
	YakitExports["QueryDomainsByDomainKeyword"] = queryDomainAssetByDomainKeyword
	YakitExports["QueryDomainsByTitle"] = queryDomainAssetByHTMLTitle

	// YakitExports["QueryPortAssetByPort"] = queryPortAssetByNetwork
	// YakitExports["QueryPortAssetByKeyword"] = queryPortAssetByNetwork

	// DeletePayload
	YakitExports["DeletePayloadByGroup"] = deletePayloadByGroup

	// AI Event
	YakitExports["DeleteAllAIEvent"] = deleteAllAIEvent
	YakitExports["YieldAllAIEvent"] = yieldAllAIEvent

}

func deleteAllAIEvent() error {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return utils.Errorf("cannot found database")
	}
	return yakit.DeleteAllAIEvent(db)
}

func yieldAllAIEvent() chan *schema.AiOutputEvent {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		c := make(chan *schema.AiOutputEvent)
		close(c)
		return c
	}
	return yakit.YieldAllAIEvent(db, context.Background())
}
