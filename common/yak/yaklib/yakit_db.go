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

// queryDomainAssetByNetwork 按网段（解析 IP 所在网段）查询域名资产（导出名为 db.QueryDomainsByNetwork）
//
// 与 db.QueryDomainsByDomainKeyword 的区别：本函数按域名“解析到的 IP”所属网段匹配（基于 ip_integer 字段），
// 用于回答“哪些域名落在某个 C 段 / 某个 IP 上”——典型用于把同一 IP 上的虚拟主机/旁站聚合出来。
//
// 参数:
//   - network: 网段表达式，如 "192.168.1.0/24"、"1.1.1.1/32"
//
// 返回值:
//   - 域名资产的 channel，可使用 for-range 遍历
//   - 错误信息（项目数据库不可用时返回）
//
// Example:
// ```
// // 保存带解析 IP 的域名，再按该 IP 网段反查域名（按 IP 聚合旁站的思路）
// db.SaveDomain("vhost-a.doc-demo-net.example.com", "203.0.113.7")~
// db.SaveDomain("vhost-b.doc-demo-net.example.com", "203.0.113.7")~
//
// got = []
// ch = db.QueryDomainsByNetwork("203.0.113.0/24")~
// for d in ch { got = append(got, d.Domain) }
// assert len(got) >= 2, "both vhosts resolving into 203.0.113.0/24 should be returned"
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
//
// 检索由 db.SaveDomain 入库的域名资产，在 domain 字段做模糊匹配（常用于按主域聚合所有子域名）。
// 返回的 Domain 对象可读取 .Domain（域名）、.IPAddr（关联 IP）、.HTTPTitle（网站标题）等字段。
//
// 参数:
//   - keyword: 域名关键词（如主域 "example.com"）
//
// 返回值:
//   - 域名资产的 channel，可使用 for-range 遍历
//   - 错误信息（项目数据库不可用时返回）
//
// Example:
// ```
// // 保存子域名资产后，按主域关键字聚合检索（保存->查询 联动）
// base = "doc-demo-dq.example.com"
// db.SaveDomain("a."+base, "1.1.1.1")~
// db.SaveDomain("b."+base, "2.2.2.2")~
//
// got = []
// ch = db.QueryDomainsByDomainKeyword(base)~
// for d in ch { got = append(got, d.Domain) }
// assert len(got) >= 2, "should retrieve the subdomains saved under the base domain"
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

// queryDomainAssetByHTMLTitle 按网站 HTML 标题模糊查询域名资产（导出名为 db.QueryDomainsByTitle）
//
// 在域名资产的 html_title 字段做模糊匹配，用于按网站标题定位资产（如所有标题含 "admin"/"后台"/"登录" 的站点）。
// 注意：仅当入库时记录了网站标题（例如经过抓取并回填标题）才能命中，仅用 db.SaveDomain 保存域名+IP 时标题为空。
//
// 参数:
//   - title: HTML 标题关键词
//
// 返回值:
//   - 域名资产的 channel，可使用 for-range 遍历
//   - 错误信息（项目数据库不可用时返回）
//
// Example:
// ```
// // 按网站标题筛选后台类资产（依赖资产已带标题，示意性示例）
// ch = db.QueryDomainsByTitle("admin")~
//
//	for domain in ch {
//	    println(domain.Domain, domain.HTTPTitle)
//	}
//
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

// QueryPortAssetByNetwork 按网段查询端口资产（导出名为 yakit.QueryPortAssetByNetwork）
// 从项目数据库中以管道方式返回匹配网段的端口资产对象
//
// 参数:
//   - network: 网段（如 "192.168.1.0/24" 或单个 IP）
//
// 返回值:
//   - Port 资产管道，可用 for range 逐条消费
//   - 错误信息（数据库不可用时返回）
//
// Example:
// ```
// // 查询某网段的端口资产（依赖本地数据库已有数据，示意性示例）
// ch = yakit.QueryPortAssetByNetwork("127.0.0.1/32")~
// for port in ch { println(port.Host, port.Port); break }
// ```
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

// QueryHostPortByKeyword 按关键字查询 Host:Port 资产（导出名为 yakit.QueryHostPortByKeyword）
// 从项目数据库匹配关键字，以管道方式返回形如 "host:port" 的字符串
//
// 参数:
//   - target: 查询关键字（可为 IP、域名片段等）
//
// 返回值:
//   - host:port 字符串管道，可用 for range 逐条消费
//
// Example:
// ```
// // 按关键字查询 Host:Port（依赖本地数据库已有数据，示意性示例）
// for hp in yakit.QueryHostPortByKeyword("127.0.0.1") { println(hp); break }
// ```
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

// queryHostPortByNetwork 按网段查询开放端口并以 "host:port" 字符串返回（导出名为 db.QueryHostPortByNetwork）
//
// 从端口资产（由 db.SavePortFromResult 入库）中筛选 state=open 且落在指定网段的记录，直接拼成 "host:port"。
// 非常适合把“之前扫到的开放端口”作为下一步动作的目标列表（如批量取 banner、批量发 poc）。
// 若需要端口资产的完整字段（服务、指纹、标题等），用 db.QueryPortAssetByNetwork 拿对象而非字符串。
//
// 参数:
//   - network: 网段表达式，如 "192.168.1.0/24" 或单个 IP
//
// 返回值:
//   - "host:port" 字符串的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 联动：把某网段已入库的开放端口取出，作为后续探测的目标列表（依赖已有端口资产，示意性示例）
// targets = []
//
//	for hostport in db.QueryHostPortByNetwork("192.168.1.0/24") {
//	    targets = append(targets, hostport)   // 形如 "192.168.1.10:80"
//	}
//
// yakit.Info("collected %d open host:port targets for next step", len(targets))
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

// QueryHostPortByNetworkAndPort 按网段与端口查询 Host:Port 资产（导出名为 yakit.QueryHostPortByNetworkAndPort）
// 从项目数据库匹配指定网段与端口范围，以管道方式返回形如 "host:port" 的字符串
//
// 参数:
//   - network: 网段（如 "192.168.1.0/24"）
//   - port: 端口或端口范围（如 "80" 或 "80,443" 或 "1-1000"）
//
// 返回值:
//   - host:port 字符串管道，可用 for range 逐条消费
//
// Example:
// ```
// // 按网段与端口查询 Host:Port（依赖本地数据库已有数据，示意性示例）
// for hp in yakit.QueryHostPortByNetworkAndPort("127.0.0.1/32", "80") { println(hp); break }
// ```
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

// QueryHostPortAll 遍历项目数据库中全部 Host:Port 资产（导出名为 yakit.QueryHostPortAll）
// 以管道方式返回形如 "host:port" 的字符串，适合流式处理全部端口资产
//
// 返回值:
//   - host:port 字符串管道，可用 for range 逐条消费
//
// Example:
// ```
// // 遍历全部 Host:Port 资产（依赖本地数据库已有数据，示意性示例）
// for hp in yakit.QueryHostPortAll() { println(hp); break }
// ```
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
//
// 与 db.SaveHTTPFlowFromRaw 的区别：本函数接收 Go/标准库风格的 *http.Request / *http.Response 对象，
// 而非原始字节。适合已经持有结构化请求/响应对象（如某些库的返回值）时直接落库；
// 来源类型固定标记为 "basic-crawler"。若你手里是原始报文字节，请优先用 db.SaveHTTPFlowFromRaw。
//
// 参数:
//   - url: 请求 URL
//   - req: *http.Request 请求对象
//   - rsp: *http.Response 响应对象
//
// 返回值:
//   - 错误信息（项目数据库不可用或保存失败时返回）
//
// Example:
// ```
// // 依赖结构化的 http 请求/响应对象（示意性示例）
// // 多数情况下推荐改用 db.SaveHTTPFlowFromRaw 直接保存原始报文字节
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
//
// 把（子）域名及其解析到的 IP 作为域名资产入库，常用于子域名爆破/收集（subdomain 库）之后的资产沉淀。
// 一个域名可关联多个 IP（多个 A 记录 / CDN 节点）。入库后可用 db.QueryDomainsByDomainKeyword /
// db.QueryDomainsByNetwork / db.QueryDomainsByTitle 检索。
//
// 参数:
//   - domain: 域名（如 "api.example.com"）
//   - ip: 零个或多个关联 IP（可变参数）
//
// 返回值:
//   - 错误信息（项目数据库不可用时返回）
//
// Example:
// ```
// // 保存若干子域名资产（可带解析 IP），再按关键字检索回来
// base = "doc-demo-asset.example.com"
// db.SaveDomain("api."+base, "93.184.216.34")~
// db.SaveDomain("cdn."+base, "93.184.216.34", "93.184.216.35")~   // 多个 IP
// db.SaveDomain("admin."+base)~                                     // 仅域名，无 IP
//
// found = 0
// ch = db.QueryDomainsByDomainKeyword(base)~
// for d in ch { found++ }
// println(found >= 3)   // OUT: true
// assert found >= 3, "the three saved subdomains should be retrievable by keyword"
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

// ObjToPort 将多种来源对象转换为统一的端口资产对象（导出名为 yakit.ObjToPort）
// 支持服务扫描结果(fp.MatchResult)等类型，便于后续统一入库或展示
//
// 参数:
//   - t: 待转换的对象（如 servicescan 的匹配结果）
//
// 返回值:
//   - 端口资产对象
//   - 错误信息（无法识别的类型时返回）
//
// Example:
// ```
// // 将服务扫描结果转换为端口资产对象（依赖扫描结果，示意性示例）
//
//	for result in servicescan.Scan("127.0.0.1", "80")~ {
//	    port = yakit.ObjToPort(result)~
//	    println(port.Host, port.Port)
//	    break
//	}
//
// ```
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

// savePortFromObj 从扫描结果对象提取端口信息并保存为端口资产（导出名为 db.SavePortFromResult）
//
// 这是端口/服务扫描结果落库的标准接口，直接接收各扫描器返回的结果对象，自动转换为统一的端口资产入库。
// 支持的对象类型：servicescan 的指纹结果(fp.MatchResult)、synscan 结果(synscan.SynScanResult)、
// 空间引擎结果、以及已构造的 schema.Port。可选传入 RuntimeId 把这批资产归属到某次扫描任务，
// 便于之后用 db.QueryPortsByRuntimeId 精确取回。入库后可用 db.QueryHostPortByNetwork 等检索。
//
// 参数:
//   - t: 扫描结果对象（servicescan/synscan/空间引擎结果或 schema.Port）
//   - RuntimeId: 可选的运行时 ID，用于把资产关联到具体扫描任务
//
// 返回值:
//   - 错误信息（类型不支持或保存失败时返回）
//
// Example:
// ```
// // 典型联动：servicescan 探测 -> SavePortFromResult 落库 -> 按网段查询回来
// // 端口扫描依赖网络与目标，这里对目标存活做容错处理
// runtimeId = "doc-demo-portscan"
// target = "scanme.nmap.org"
//
//	for result in servicescan.Scan(target, "80,443")~ {
//	    db.SavePortFromResult(result, runtimeId)~   // 把每个开放端口结果落库
//	    yakit.Info("saved port asset: %v:%v", result.Target, result.Port)
//	}
//
// // 按 runtimeId 取回本次扫描保存的端口资产
//
//	for port in db.QueryPortsByRuntimeId(runtimeId)~ {
//	    println(port.Host, port.Port, port.ServiceType)
//	}
//
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

// queryUrlsByKeyword 按关键词模糊查询历史流量中的 URL 资产（导出名为 db.QueryUrlsByKeyword）
//
// 与 db.QueryHTTPFlowsByKeyword 的区别：本函数只在 url 字段上匹配，且只返回去重后的 URL 字符串
// （而非完整流量对象），更轻量，适合“我只想要 URL 列表”的场景（如收集所有 /api 接口）。
//
// 参数:
//   - k: URL 关键词（在 url 字段做模糊匹配）
//
// 返回值:
//   - URL 字符串的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 落两条同站点不同路径的流量，再仅按 URL 关键字收集接口路径
// host = "doc-demo-urls.example.com"
// rsp = []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
// db.SaveHTTPFlowFromRaw("http://"+host+"/api/users", []byte(f"GET /api/users HTTP/1.1\r\nHost: ${host}\r\n\r\n"), rsp)~
// db.SaveHTTPFlowFromRaw("http://"+host+"/api/login", []byte(f"GET /api/login HTTP/1.1\r\nHost: ${host}\r\n\r\n"), rsp)~
//
// apis = []
// for u in db.QueryUrlsByKeyword(host+"/api") { apis = append(apis, u) }
// assert len(apis) >= 2, "should collect at least the two /api urls just saved"
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
//
// 在历史流量的 url、request、response 三个字段中做模糊匹配，是检索“流量证据”的主力接口。
// 例如检索包含 "password" 的流量定位登录点、检索某域名定位相关请求。返回 HTTPFlow 对象 channel，
// 可读取 .Url / .Method / .StatusCode / .BodyLength 等字段，常配合 yakit.NewTable 展示结果。
//
// 参数:
//   - k: 关键词（在 url/request/response 中模糊匹配；传 "" 等价于遍历全部）
//
// 返回值:
//   - HTTPFlow 对象的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 先落一条可检索的流量，再按关键字查询并用表格展示结果（保存->查询->展示 联动）
// host = "doc-demo-query.example.com"
// req = []byte(f"GET /login HTTP/1.1\r\nHost: ${host}\r\n\r\n")
// rsp = []byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nform")
// db.SaveHTTPFlowFromRaw("http://"+host+"/login", req, rsp)~
//
// table = yakit.NewTable("URL", "Method", "Status")
// hit = 0
//
//	for flow in db.QueryHTTPFlowsByKeyword(host) {
//	    table.Append(flow.Url, flow.Method, flow.StatusCode)
//	    hit++
//	}
//
// yakit.Output(table)
// assert hit >= 1, "the saved login flow should be found by keyword"
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
//
// 取回某一次扫描任务（runtimeId）保存的端口资产，是 db.SavePortFromResult(result, runtimeId) 的天然配对：
// 保存时用同一个 runtimeId 归档，查询时用它精确取回“本次扫描”的结果，避免与历史数据混在一起。
//
// 参数:
//   - runtimeID: 运行时 ID（与保存端口资产时传入的一致）
//
// 返回值:
//   - 端口资产的 channel，可使用 for-range 遍历
//   - 错误信息（项目数据库不可用时返回）
//
// Example:
// ```
// // 与保存端 SavePortFromResult(result, runtimeId) 配对使用（依赖一次真实扫描，示意性示例）
// runtimeId = "scan-"+uuid()
//
//	for result in servicescan.Scan("127.0.0.1", "1-100")~ {
//	    db.SavePortFromResult(result, runtimeId)~
//	}
//
// table = yakit.NewTable("Host", "Port", "Service")
//
//	for port in db.QueryPortsByRuntimeId(runtimeId)~ {
//	    table.Append(port.Host, port.Port, port.ServiceType)
//	}
//
// yakit.Output(table)
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

// queryHTTPFlowByID 按数据库自增 ID 精确查询单条 HTTP 流量（导出名为 db.QueryHTTPFlowByID）
//
// 当你已经知道某条流量的 ID（例如从列表/表格中选中、或从其他查询里拿到 flow.ID）时，用它直接取回完整对象。
// 批量按多个 ID 取用 db.QueryHTTPFlowsByID。
//
// 参数:
//   - id: HTTPFlow 的数据库 ID
//
// 返回值:
//   - HTTPFlow 对象
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
//
// payload 字典是爆破（brute）、fuzz、模糊测试等场景的弹药库。同一 group 名下的多次保存会累积入库（去重由底层处理）。
// 入库后用 db.YieldPayload 流式取出消费、db.GetAllPayloadGroupsName 列出所有组、db.DeletePayloadByGroup 删除整组。
// payloadRaw 既可传单个字符串，也可传字符串列表；大字典建议用 db.SavePayloadByFile 直接从文件导入。
//
// 参数:
//   - group: 字典组名（如 "common-users"、"weak-passwords"）
//   - payloadRaw: payload 内容，支持单个字符串或字符串列表
//
// 返回值:
//   - 错误信息（数据库不可用或保存失败时返回）
//
// Example:
// ```
// // 保存用户名/密码字典，再取出用于（演示性的）凭据组合
// db.SavePayload("doc-demo-users", ["admin", "root", "test"])~
// db.SavePayload("doc-demo-pass", ["123456", "admin123", "P@ssw0rd"])~
//
// users = []; pass = []
// for u in db.YieldPayload("doc-demo-users") { users = append(users, u) }
// for p in db.YieldPayload("doc-demo-pass")  { pass  = append(pass, p) }
// combos = len(users) * len(pass)
// println(combos)   // OUT: 9
// assert combos == 9, "3 users x 3 passwords should yield 9 credential combinations"
//
// // 清理演示数据
// db.DeletePayloadByGroup("doc-demo-users"); db.DeletePayloadByGroup("doc-demo-pass")
// ```
func savePayloads(group string, payloadRaw any) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	payloads := utils.InterfaceToStringSlice(payloadRaw)
	return yakit.SavePayloadGroup(consts.GetGormProfileDatabase(), group, payloads)
}

// savePayloadByFile 从字典文件按行读取内容并保存到指定字典组（导出名为 db.SavePayloadByFile）
//
// 与 db.SavePayload 等价，但数据源是文件：文件每一行作为一个 payload 入库，适合导入大字典（rockyou 等）。
// 导入后同样用 db.YieldPayload 取出消费。
//
// 参数:
//   - group: 字典组名
//   - fileName: 字典文件路径（每行一个 payload）
//
// 返回值:
//   - 错误信息（数据库不可用、文件不存在或读取失败时返回）
//
// Example:
// ```
// // 先用 file 库写一个临时字典文件，再导入字典组并读回
// dictPath = "/tmp/doc-demo-dict.txt"
// file.Save(dictPath, "admin\nroot\nguest\n")~
// db.SavePayloadByFile("doc-demo-fromfile", dictPath)~
//
// got = []
// for p in db.YieldPayload("doc-demo-fromfile") { got = append(got, p) }
// assert len(got) == 3, "should import 3 lines from the dictionary file"
// db.DeletePayloadByGroup("doc-demo-fromfile")
// ```
func savePayloadByFile(group string, fileName string) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	return yakit.SavePayloadByFilename(consts.GetGormProfileDatabase(), group, fileName)
}

// deletePayloadByGroup 删除指定字典组及其全部 payload（导出名为 db.DeletePayloadByGroup）
//
// 用于清理不再需要的字典组。删除后该组不再出现在 db.GetAllPayloadGroupsName 的结果里，
// db.YieldPayload 也取不到任何内容。删除不存在的组不会报错。
//
// 参数:
//   - group: 要删除的字典组名
//
// 返回值:
//   - 错误信息（数据库不可用时返回）
//
// Example:
// ```
// // 完整生命周期：创建 -> 确认存在 -> 删除 -> 确认消失
// db.SavePayload("doc-demo-tmp", ["a", "b"])~
// assert "doc-demo-tmp" in db.GetAllPayloadGroupsName(), "group should exist after SavePayload"
// db.DeletePayloadByGroup("doc-demo-tmp")~
// assert !("doc-demo-tmp" in db.GetAllPayloadGroupsName()), "group should be gone after delete"
// ```
func deletePayloadByGroup(group string) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	return yakit.DeletePayloadByGroup(consts.GetGormProfileDatabase(), group)
}

// getPayloadGroups 查询匹配指定名称关键字的字典组列表（导出名为 db.QueryPayloadGroups）
//
// 与 db.GetAllPayloadGroupsName 的区别：本函数可按关键字过滤组名（传空串等价于返回全部）。
// 适合在大量字典组中按命名前缀/关键字定位目标组。
//
// 参数:
//   - group: 字典组名关键字（传 "" 返回全部组）
//
// 返回值:
//   - 匹配的字典组名列表
//
// Example:
// ```
// // 创建两个带相同前缀的组，再用关键字过滤
// db.SavePayload("doc-grp-alpha", ["x"])~
// db.SavePayload("doc-grp-beta",  ["y"])~
// matched = db.QueryPayloadGroups("doc-grp-")
// println(len(matched) >= 2)   // OUT: true
// assert len(matched) >= 2, "keyword should match both doc-grp-* groups"
// db.DeletePayloadByGroup("doc-grp-alpha"); db.DeletePayloadByGroup("doc-grp-beta")
// ```
func getPayloadGroups(group string) []string {
	if consts.GetGormProfileDatabase() == nil {
		log.Error("no database connections")
		return nil
	}
	return yakit.PayloadGroups(consts.GetGormProfileDatabase(), group)
}

// getAllPayloadGroupsName 获取数据库中全部字典组的名称（导出名为 db.GetAllPayloadGroupsName）
//
// 用于枚举当前所有可用的 payload 字典组，常配合 db.YieldPayload 遍历每个组的内容做统计或导出。
//
// 返回值:
//   - 全部字典组名列表
//
// Example:
// ```
// // 创建若干组后枚举全部组名，并统计每组 payload 数量
// db.SavePayload("doc-all-1", ["a", "b"])~
// db.SavePayload("doc-all-2", ["c"])~
// groups = db.GetAllPayloadGroupsName()
// assert "doc-all-1" in groups && "doc-all-2" in groups, "both groups should be listed"
//
//	for g in groups {
//	    if g.HasPrefix("doc-all-") {
//	        cnt = 0
//	        for _ in db.YieldPayload(g) { cnt++ }
//	        println(g, cnt)
//	    }
//	}
//
// db.DeletePayloadByGroup("doc-all-1"); db.DeletePayloadByGroup("doc-all-2")
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

// YieldPayload 以 channel 形式流式遍历一个或多个字典组中的 payload 内容（导出名为 db.YieldPayload）
//
// 这是消费字典的标准方式：内存友好，适合大字典。可一次传入多个组名，把多个字典合并消费
// （例如把若干用户名字典拼成一个候选集）。常配合爆破/fuzz 的目标循环使用。
//
// 参数:
//   - raw: 字典组名
//   - extra: 额外的字典组名（可变参数，用于合并多组）
//
// 返回值:
//   - payload 内容的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 准备两个用户名字典，合并遍历得到去重后的候选集
// db.SavePayload("doc-y-a", ["admin", "root"])~
// db.SavePayload("doc-y-b", ["root", "guest"])~
//
// seen = {}
// for u in db.YieldPayload("doc-y-a", "doc-y-b") { seen[u] = true }
// println(len(seen))   // OUT: 3
// assert len(seen) == 3, "merged unique usernames should be admin/root/guest"
//
// // 典型联动：用字典构造 HTTP 登录请求（此处仅构造，不发送）
//
//	for u in db.YieldPayload("doc-y-a") {
//	    _ = f`POST /login HTTP/1.1\r\nHost: target\r\n\r\nusername=${u}&password=test`
//	}
//
// db.DeletePayloadByGroup("doc-y-a"); db.DeletePayloadByGroup("doc-y-b")
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
	YakitExports["QueryHTTPFlowsAll"] = queryHTTPFlowsAll

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

// QueryHTTPFlowsAll 以管道方式遍历数据库中保存的全部 HTTP 流量（导出名为 yakit.QueryHTTPFlowsAll）
// 等价于以空关键字调用 QueryHTTPFlowsByKeyword，适合流式处理海量历史流量
//
// 返回值:
//   - 一个 HTTPFlow 管道，可用 for range 逐条消费
//
// Example:
// ```
// // 遍历历史 HTTP 流量（依赖本地数据库已有数据，示意性示例）
//
//	for flow in yakit.QueryHTTPFlowsAll() {
//	    println(flow.Url)
//	    break
//	}
//
// ```
func queryHTTPFlowsAll() chan *schema.HTTPFlow {
	return queryHTTPFlowByKeyword("")
}

// DeleteAllAIEvent 删除项目数据库中保存的全部 AI 事件（导出名为 yakit.DeleteAllAIEvent）
//
// 返回值:
//   - 错误信息（数据库不可用或删除失败时返回）
//
// Example:
// ```
// // 清空项目库中的 AI 事件（会修改数据库，示意性示例）
// yakit.DeleteAllAIEvent()
// ```
func deleteAllAIEvent() error {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return utils.Errorf("cannot found database")
	}
	return yakit.DeleteAllAIEvent(db)
}

// YieldAllAIEvent 以管道方式遍历项目数据库中保存的全部 AI 事件（导出名为 yakit.YieldAllAIEvent）
//
// 返回值:
//   - 一个 AI 事件管道，可用 for range 逐条消费；数据库不可用时返回已关闭的空管道
//
// Example:
// ```
// // 遍历项目库中的 AI 事件（依赖本地数据库已有数据，示意性示例）
//
//	for event in yakit.YieldAllAIEvent() {
//	    println(event.Type)
//	    break
//	}
//
// ```
func yieldAllAIEvent() chan *schema.AiOutputEvent {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		c := make(chan *schema.AiOutputEvent)
		close(c)
		return c
	}
	return yakit.YieldAllAIEvent(db, context.Background())
}
