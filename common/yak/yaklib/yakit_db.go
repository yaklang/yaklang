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

func queryDomainAssetByNetwork(network string) (chan *schema.Domain, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("domains")
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", network)
	return yakit.YieldDomains(db, context.Background()), nil
}

func queryDomainAssetByDomainKeyword(keyword string) (chan *schema.Domain, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("domains")
	db = bizhelper.FuzzQueryLike(db, "domain", keyword)
	return yakit.YieldDomains(db, context.Background()), nil
}

func queryDomainAssetByHTMLTitle(title string) (chan *schema.Domain, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("domains")
	db = bizhelper.FuzzQueryLike(db, "html_title", title)
	return yakit.YieldDomains(db, context.Background()), nil
}

func queryHostAssetByNetwork(network string) (chan *schema.Host, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("hosts")
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", network)
	return yakit.YieldHosts(db, context.Background()), nil
}

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

func saveHTTPFlowInstance(flow *schema.HTTPFlow) error {
	return yakit.SaveHTTPFlow(consts.GetGormProjectDatabase(), flow)
}

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

func queryHTTPFlowByID(id ...int64) chan *schema.HTTPFlow {
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

func queryAllUrls() chan string {
	return queryUrlsByKeyword("")
}

func savePayloads(group string, payloadRaw any) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	payloads := utils.InterfaceToStringSlice(payloadRaw)
	return yakit.SavePayloadGroup(consts.GetGormProfileDatabase(), group, payloads)
}

func savePayloadByFile(group string, fileName string) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	return yakit.SavePayloadByFilename(consts.GetGormProfileDatabase(), group, fileName)
}

func deletePayloadByGroup(group string) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	return yakit.DeletePayloadByGroup(consts.GetGormProfileDatabase(), group)
}

func getPayloadGroups(group string) []string {
	if consts.GetGormProfileDatabase() == nil {
		log.Error("no database connections")
		return nil
	}
	return yakit.PayloadGroups(consts.GetGormProfileDatabase(), group)
}

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

// YieldPayload means
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

}
