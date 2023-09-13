package subdomain

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/netx"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type SearchRequestBuilder func(target string) (*http.Request, error)
type SearchAction func(ctx context.Context, target string) ([]*SubdomainResult, error)

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
	client := netx.NewDefaultHTTPClient()
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
	HackerTarget SearchRequestBuilder = func(target string) (*http.Request, error) {
		url := fmt.Sprintf("https://api.hackertarget.com/hostsearch/?q=%s", target)
		return http.NewRequest("GET", url, nil)
	}

	// http://ce.baidu.com/index/getRelatedSites?site_address=%s
	BaiduCe SearchRequestBuilder = func(target string) (request *http.Request, e error) {
		url := fmt.Sprintf("http://ce.baidu.com/index/getRelatedSites?site_address=%s", target)
		return http.NewRequest("GET", url, nil)
	}

	// https://api.sublist3r.com/search.php?domain=%s
	Sublist3r SearchRequestBuilder = func(target string) (request *http.Request, e error) {
		url := fmt.Sprintf("https://api.sublist3r.com/search.php?domain=%s", target)
		return http.NewRequest("GET", url, nil)
	}

	// https://crt.sh/?q=%25.%s
	CrtSh SearchRequestBuilder = func(target string) (request *http.Request, e error) {
		url := fmt.Sprintf("https://crt.sh/?q=%%25.%s", target)
		return http.NewRequest("GET", url, nil)
	}

	// https://api.certspotter.com/v1/issuances
	CertsPotter SearchRequestBuilder = func(target string) (request *http.Request, e error) {
		u, _ := url.Parse("https://api.certspotter.com/v1/issuances")
		u.RawQuery = url.Values{
			"domain":             {target},
			"include_subdomains": {"true"},
			"match_wildcards":    {"true"},
			"expand":             {"dns_names"},
		}.Encode()
		return http.NewRequest("GET", u.String(), nil)
	}

	// https://ctsearch.entrust.com/api/v1/certificates
	// /u003d需要被特殊处理
	Entrust SearchRequestBuilder = func(target string) (request *http.Request, e error) {
		u, _ := url.Parse("https://ctsearch.entrust.com/api/v1/certificates")
		u.RawQuery = url.Values{
			"fields":         {"subjectO,issuerDN,subjectDN,signAlg,san,sn,subjectCNReversed,cert"},
			"domain":         {target},
			"includeExpired": {"true"},
			"exactMatch":     {"false"},
			"limit":          {"5000"},
		}.Encode()
		return http.NewRequest("GET", u.String(), nil)
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
		client := netx.NewDefaultHTTPClient()
		wg := sync.WaitGroup{}
		wg.Add(len(SearchSource))

		var results sync.Map
		for _, source := range SearchSource {
			req, err := source(target)
			if err != nil {
				//log.Warnf("%s", err)
				continue
			}
			go func(request *http.Request) {
				defer wg.Done()

				rsp, err := client.Do(request)
				if err != nil {
					//log.Warnf("request %s failed: %s", request.URL.String(), err)
					return
				}

				if rsp.Body == nil {
					//log.Infof("emtpy body %s", request.URL.String())
					return
				}

				raw, err := ioutil.ReadAll(rsp.Body)
				if err != nil {
					//log.Infof("read [%s] body failed: %s", request.URL.String(), err)
					return
				}

				r := fmt.Sprintf(`[0-9a-zA-Z\.-]*\.%s`, regexp.QuoteMeta(target))
				re, err := regexp.Compile(r)
				if err != nil {
					//log.Errorf("compile %s failed: %s", r, err)
					return
				}

				// 针对ctsearch.entrust.com做特殊处理处理
				if req.Host == "ctsearch.entrust.com" {
					raw = []byte(strings.ReplaceAll(string(raw), "\\u003d", "="))
				}
				//log.Debugf("body: %s", string(raw))
				for _, match := range re.FindAll(raw, -1) {
					results.Store(string(match), request.URL.String())
				}
			}(req)
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
		CertsPotter, Entrust,
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
