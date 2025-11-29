package subdomain

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type (
	SearchRequestBuilder func(target string) (url string, reqRaw []byte, options []poc.PocConfigOption, err error)
	SearchAction         func(ctx context.Context, target string) ([]*SubdomainResult, error)
)

func timeoutFromContext(ctx context.Context, timeout time.Duration) time.Duration {
	ddl, ok := ctx.Deadline()
	if ok {
		return ddl.Sub(time.Now())
	}
	return timeout
}

func virustotalVisit(ctx context.Context, url string) (result []string, err error) {
	var m struct {
		Data []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
	}
	minVer, maxVer := consts.GetGlobalTLSVersion()
	client := http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         minVer, // nolint[:staticcheck]
			MaxVersion:         maxVer,
		}},
		Timeout: timeoutFromContext(ctx, 15*time.Second),
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	rsp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer rsp.Body.Close()
	body, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}

	for _, data := range m.Data {
		if data.Type == "domain" {
			result = append(result, data.ID)
		}
	}
	if m.Links.Next != "" {
		r, err := virustotalVisit(ctx, m.Links.Next)
		if err != nil {
			return nil, err
		}
		result = append(result, r...)
	}

	return
}

var (
	// todo:
	// BufferOver  需要处理cloudflare
	// ThreatCrowd  需要处理cloudflare
	// http://www.sitedossier.com/parentdomain/baidu.com/2 需要解析页面处理分页
	// http://www.dnsdb.org/ 需要获取token
	// https://searchdns.netcraft.com/?restriction=site+ends+with&host=%s 需要解析页面，处理分页

	// https://api.hackertarget.com/hostsearch/?q=%s
	HackerTarget SearchRequestBuilder = func(target string) (string, []byte, []poc.PocConfigOption, error) {
		url := fmt.Sprintf("https://api.hackertarget.com/hostsearch/?q=%s", target)
		packet := lowhttp.UrlToRequestPacket("GET", fmt.Sprintf("https://api.hackertarget.com/hostsearch/?q=%s", target), nil, true)
		return url, packet, nil, nil
	}

	// http://ce.baidu.com/index/getRelatedSites?site_address=%s
	BaiduCe SearchRequestBuilder = func(target string) (string, []byte, []poc.PocConfigOption, error) {
		url := fmt.Sprintf("http://ce.baidu.com/index/getRelatedSites?site_address=%s", target)
		packet := lowhttp.UrlToRequestPacket("GET", url, nil, true)
		return url, packet, nil, nil
	}

	// https://api.sublist3r.com/search.php?domain=%s
	Sublist3r SearchRequestBuilder = func(target string) (string, []byte, []poc.PocConfigOption, error) {
		url := fmt.Sprintf("https://api.sublist3r.com/search.php?domain=%s", target)
		packet := lowhttp.UrlToRequestPacket("GET", url, nil, true)
		return url, packet, nil, nil
	}

	// https://otx.alienvault.com/api/v1/indicators/domain/%s/url_list?page=1
	AlienVault SearchRequestBuilder = func(target string) (string, []byte, []poc.PocConfigOption, error) {
		url := fmt.Sprintf("https://otx.alienvault.com/api/v1/indicators/domain/%s/url_list?page=1", target)
		packet := lowhttp.UrlToRequestPacket("GET", url, nil, true)
		return url, packet, nil, nil
	}

	// http://web.archive.org/cdx/search/cdx?url=*.%s/*&output=txt&fl=original&collapse=urlkey
	ArchiveOrg SearchRequestBuilder = func(target string) (string, []byte, []poc.PocConfigOption, error) {
		url := fmt.Sprintf("http://web.archive.org/cdx/search/cdx?url=*.%s/*&output=txt&fl=original&collapse=urlkey", target)
		packet := lowhttp.UrlToRequestPacket("GET", url, nil, false)
		return url, packet, nil, nil
	}

	// https://crt.sh/?q=%25.%s
	CrtSh SearchRequestBuilder = func(target string) (string, []byte, []poc.PocConfigOption, error) {
		url := fmt.Sprintf("https://crt.sh/?q=%%25.%s", target)
		packet := lowhttp.UrlToRequestPacket("GET", url, nil, true)
		return url, packet, nil, nil
	}

	// https://api.certspotter.com/v1/issuances
	CertsPotter SearchRequestBuilder = func(target string) (string, []byte, []poc.PocConfigOption, error) {
		u := utils.ParseStringToUrl("https://api.certspotter.com/v1/issuances")
		u.RawQuery = url.Values{
			"domain":             {target},
			"include_subdomains": {"true"},
			"match_wildcards":    {"true"},
			"expand":             {"dns_names"},
		}.Encode()
		packet := lowhttp.UrlToRequestPacket("GET", u.String(), nil, true)
		return u.String(), packet, nil, nil
	}

	// https://ctsearch.entrust.com/api/v1/certificates
	// /u003d需要被特殊处理
	Entrust SearchRequestBuilder = func(target string) (string, []byte, []poc.PocConfigOption, error) {
		u := utils.ParseStringToUrl("https://ctsearch.entrust.com/api/v1/certificates")
		u.RawQuery = url.Values{
			"fields":         {"subjectO,issuerDN,subjectDN,signAlg,san,sn,subjectCNReversed,cert"},
			"domain":         {target},
			"includeExpired": {"true"},
			"exactMatch":     {"false"},
			"limit":          {"5000"},
		}.Encode()
		packet := lowhttp.UrlToRequestPacket("GET", u.String(), nil, true)
		return u.String(), packet, nil, nil
	}

	// https://www.virustotal.com/gui/domain/%s/relations
	VirustotalDomainRelations SearchAction = func(ctx context.Context, target string) (result []*SubdomainResult, e error) {
		url := fmt.Sprintf("https://www.virustotal.com/ui/domains/%s/subdomains?limit=40", target)
		r, err := virustotalVisit(ctx, url)
		if err != nil {
			return nil, err
		}
		for _, r := range r {
			result = append(result, &SubdomainResult{
				FromTarget:    target,
				FromDNSServer: "",
				FromModeRaw:   SEARCH,
				IP:            "",
				Domain:        r,
				Tags:          []string{url},
			})
		}
		return
	}

	GeneralAction SearchAction = func(ctx context.Context, target string) (result []*SubdomainResult, e error) {
		wg := sync.WaitGroup{}
		wg.Add(len(SearchSource))

		var results sync.Map
		for _, source := range SearchSource {
			url, reqRaw, options, err := source(target)
			if err != nil {
				// log.Warnf("%s", err)
				continue
			}
			go func() {
				defer wg.Done()

				rsp, _, err := poc.HTTP(reqRaw, options...)
				if err != nil {
					// log.Warnf("request %s failed: %s", request.URL.String(), err)
					return
				}

				if len(rsp) == 0 {
					// log.Infof("emtpy body %s", request.URL.String())
					return
				}

				_, raw := lowhttp.SplitHTTPPacketFast(rsp)
				if err != nil {
					// log.Infof("read [%s] body failed: %s", request.URL.String(), err)
					return
				}

				r := fmt.Sprintf(`[0-9a-zA-Z\.-]*\.%s`, regexp.QuoteMeta(target))
				re, err := regexp.Compile(r)
				if err != nil {
					// log.Errorf("compile %s failed: %s", r, err)
					return
				}

				// 针对ctsearch.entrust.com做特殊处理处理
				host := lowhttp.GetHTTPPacketHeader(reqRaw, "Host")
				if host == "ctsearch.entrust.com" {
					raw = []byte(strings.ReplaceAll(string(raw), "\\u003d", "="))
				}
				for _, match := range re.FindAll(raw, -1) {
					results.Store(string(match), url)
				}
			}()
		}
		wg.Wait()

		results.Range(func(key, value interface{}) bool {
			domain := key.(string)
			from := value.(string)
			result = append(result, &SubdomainResult{
				FromTarget:    target,
				FromDNSServer: "",
				FromModeRaw:   SEARCH,
				IP:            "",
				Domain:        domain,
				Tags:          []string{from},
			})
			return true
		})
		e = nil
		return
	}

	SearchSource = []SearchRequestBuilder{
		HackerTarget, BaiduCe, Sublist3r, CrtSh,
		ArchiveOrg, AlienVault, CertsPotter, Entrust,
	}

	SearchActions = []SearchAction{
		GeneralAction,
		VirustotalDomainRelations,
	}
)

func (s *SubdomainScanner) Search(ctx context.Context, target string) {
	target = formatDomain(target)

	s.logger.Infof("start to search subdomain from data sources for %s", target)

	wg := sync.WaitGroup{}
	for _, action := range SearchActions {

		wg.Add(1)
		go func(handler SearchAction) {
			defer wg.Done()

			c, _ := context.WithTimeout(ctx, s.config.TimeoutForEachHTTPSearch)
			results, err := handler(c, target)
			if err != nil {
				s.logger.Error(err.Error())
				return
			}

			// 进行结果处理，检查是否能解析到 IP 上，如果不能的话，可能需要输出在别的地方
			for _, result := range results {
				if ctx.Err() != nil {
					return
				}
				s.logger.Infof("search mode found: %s", result.Domain)
				if result.IP == "" {
					err := s.dnsQuerierSwg.AddWithContext(ctx)
					if err != nil {
						return
					}
					wg.Add(1)
					result := result
					go func() {
						defer s.dnsQuerierSwg.Done()
						defer wg.Done()
						ip, server, err := s.QueryA(ctx, result.Domain)
						if err != nil {
							s.logger.Infof("domain[%s] is found by searching mode but cannot be resolved to IP: %s", result.Domain, err)
							s.onResolveFailedResult(result)
							return
						}

						result.IP = ip
						result.FromDNSServer = server

						s.onResult(result)
					}()
				}
			}
		}(action)
	}
	wg.Wait()
}
