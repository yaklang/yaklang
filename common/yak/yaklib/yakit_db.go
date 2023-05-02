package yaklib

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"yaklang/common/consts"
	"yaklang/common/fp"
	"yaklang/common/log"
	"yaklang/common/synscan"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/utils/spacengine"
	"yaklang/common/yakgrpc/yakit"
)

func queryDomainAssetByNetwork(network string) (chan *yakit.Domain, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("domains")
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", network)
	return yakit.YieldDomains(db, context.Background()), nil
}

func queryDomainAssetByDomainKeyword(keyword string) (chan *yakit.Domain, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("domains")
	db = bizhelper.FuzzQueryLike(db, "domain", keyword)
	return yakit.YieldDomains(db, context.Background()), nil
}

func queryDomainAssetByHTMLTitle(title string) (chan *yakit.Domain, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("domains")
	db = bizhelper.FuzzQueryLike(db, "html_title", title)
	return yakit.YieldDomains(db, context.Background()), nil
}

func queryHostAssetByNetwork(network string) (chan *yakit.Host, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("hosts")
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", network)
	return yakit.YieldHosts(db, context.Background()), nil
}

func queryHostAssetByDomainKeyword(keyword string) (chan *yakit.Host, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}

	db = db.Table("hosts")
	db = bizhelper.FuzzQueryLike(db, "domains", keyword)
	return yakit.YieldHosts(db, context.Background()), nil
}

func queryPortAssetByNetwork(network string) (chan *yakit.Port, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&yakit.Port{})
	db = bizhelper.ExactQueryString(db, "state", "open")
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", network)
	return yakit.YieldPorts(db, context.Background()), nil
}

func queryPortAssetByNetworkAndPort(network string, port string) (chan *yakit.Port, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&yakit.Port{})
	db = bizhelper.ExactQueryString(db, "state", "open")
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", network)
	db = bizhelper.QueryBySpecificPorts(db, "port", port)
	return yakit.YieldPorts(db, context.Background()), nil
}

func queryPortAssetByKeyword(keyword string) (chan *yakit.Port, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&yakit.Port{})
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
	var db = consts.GetGormProjectDatabase()
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
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return utils.Errorf("cannot found database")
	}

	_, err := yakit.SaveFromHTTP(db, strings.HasPrefix(url, "https"), req, rsp, typeStr, url, req.RemoteAddr)
	if err != nil {
		return err
	}
	return nil
}

func saveDomain(domain string, ip ...string) error {
	var db = consts.GetGormProjectDatabase()
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

func savePortFromObj(t interface{}, taskNames ...string) error {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return utils.Errorf("cannot found database")
	}

	var r *yakit.Port
	switch ret := t.(type) {
	case *fp.MatchResult:
		r = NewPortFromMatchResult(ret)
	case *synscan.SynScanResult:
		r = NewPortFromSynScanResult(ret)
	case *spacengine.NetSpaceEngineResult:
		r = NewPortFromSpaceEngineResult(ret)
	}

	if r == nil {
		return utils.Errorf("unsupported(%v): %#v", reflect.TypeOf(t), spew.Sdump(t))
	}
	if len(taskNames) > 0 {
		r.TaskName = taskNames[0]
	}

	return yakit.CreateOrUpdatePort(db, r.CalcHash(), r)
}

func queryUrlsByKeyword(k string) chan string {
	ch := make(chan string, 100)
	go func() {
		defer close(ch)
		var db = consts.GetGormProjectDatabase()
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

func queryHTTPFlowByKeyword(k string) chan *yakit.HTTPFlow {
	ch := make(chan *yakit.HTTPFlow, 100)
	go func() {
		defer close(ch)
		var db = consts.GetGormProjectDatabase()
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

func queryPortsByUpdatedAt(timestamp int64) (chan *yakit.Port, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&yakit.Port{})
	db = bizhelper.ExactQueryString(db, "state", "open")
	db = bizhelper.QueryDateTimeAfterTimestampOr(db, "updated_at", timestamp)
	db = bizhelper.FuzzSearchEx(db, []string{
		"host", "service_type",
		"fingerprint", "cpe", "html_title",
	}, "", false)
	return yakit.YieldPorts(db, context.Background()), nil
}

func queryPortsByTaskName(taskName string) (chan *yakit.Port, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&yakit.Port{})
	db = bizhelper.ExactQueryString(db, "task_name", taskName)
	db = bizhelper.FuzzSearchEx(db, []string{
		"host", "service_type",
		"fingerprint", "cpe", "html_title",
	}, "", false)
	return yakit.YieldPorts(db, context.Background()), nil
}

func queryHTTPFlowByID(id ...int64) chan *yakit.HTTPFlow {
	ch := make(chan *yakit.HTTPFlow, 100)
	go func() {
		defer close(ch)
		var db = consts.GetGormProjectDatabase()
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

func savePayloads(group string, payloads []string) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	return yakit.SavePayloadGroup(consts.GetGormProfileDatabase(), group, payloads)
}

func savePayloadByFile(group string, fileName string) error {
	if consts.GetGormProfileDatabase() == nil {
		return utils.Error("no database connections")
	}
	return yakit.SavePayloadByFilename(consts.GetGormProfileDatabase(), group, fileName)
}

func deletePayload(group string) error {
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

func init() {
	YakitExports["SaveHTTPFlow"] = saveCrawler
	YakitExports["SavePortFromResult"] = savePortFromObj
	YakitExports["SaveDomain"] = saveDomain
	YakitExports["SavePayload"] = savePayloads
	YakitExports["SavePayloadByFile"] = savePayloadByFile

	// HTTP 资产
	YakitExports["QueryUrlsByKeyword"] = queryUrlsByKeyword
	YakitExports["QueryUrlsAll"] = queryAllUrls
	YakitExports["QueryHTTPFlowsByKeyword"] = queryHTTPFlowByKeyword
	YakitExports["QueryHTTPFlowsAll"] = func() chan *yakit.HTTPFlow {
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

	//YakitExports["QueryPortAssetByPort"] = queryPortAssetByNetwork
	//YakitExports["QueryPortAssetByKeyword"] = queryPortAssetByNetwork

	// DeletePayload
	YakitExports["DeletePayloadByGroup"] = deletePayload
}
