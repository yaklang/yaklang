package httptpl

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/mixer"
)

type RequestConfig struct {
	JsEnableRedirect     bool
	JsMaxRedirects       int
	EnableRedirect       bool
	MaxRedirects         int
	EtcHosts             map[string]string
	DNSServers           []string
	Variables            *YakVariables
	RepeatTimes          int64
	RetryInStatusCode    string
	RetryNotInStatusCode string
	Concurrent           int64
	MaxRetryTimes        int64
	DelayMinSeconds      float64
	DelayMaxSeconds      float64
	ForceFuzz            bool
	RequestTimeout       float64
	NoSystemProxy        bool
	Proxy                string
	Host                 string
	IsGmTLS              bool
	IsHTTPS              bool
}
type YakTemplate struct {
	Comments []string
	RequestConfig
	Id            string   `json:"id"`
	Name          string   `json:"name"`
	NameZh        string   `json:"nameZh,omitempty"`
	Author        string   `json:"author"`
	Severity      string   `json:"severity,omitempty"`
	Description   string   `json:"description"`
	DescriptionZh string   `json:"descriptionZh"`
	Reference     []string `json:"reference"`
	Tags          []string `json:"tags"`
	CVE           string
	ShodanQuery   string
	Verified      string

	// interactsh
	ReverseConnectionNeed bool `json:"reverseConnectionNeed"`

	TCPRequestSequences  []*YakNetworkBulkConfig
	HTTPRequestSequences []*YakRequestBulkConfig

	// placeHolderMap
	PlaceHolderMap map[string]string
}

type YakRequestBulkConfig struct {
	RequestConfig

	Matcher   *YakMatcher
	Extractor []*YakExtractor

	HTTPRequests []*YakHTTPRequestPacket

	StopAtFirstMatch bool

	CookieInherit      bool
	MaxSize            int
	NoFixContentLength bool
	Payloads           *YakPayloads

	// req-condition - 为 true 的时候，要等所有的请求发送完在执行 Matcher
	AfterRequested bool

	// batteringram is not valid!
	// pitchfork means sync
	// cluster bomb means cartesian product
	AttackMode       string // sync // cartesian
	InheritVariables bool
	HotPatchCode     string
}

func (c *YakRequestBulkConfig) GenerateRaw() []*RequestBulk {
	var maxLen int
	var dicts = map[string][]string{}
	if c.Payloads != nil {
		for k, p := range c.Payloads.raw {
			if maxLen < len(p.Data) {
				maxLen = len(p.Data)
			}
			dicts[k] = p.Data
		}
	}

	if maxLen <= 0 {
		var requestSeq = &RequestBulk{RequestConfig: c, Requests: nil}
		for _, req := range c.HTTPRequests {
			for _, raw := range req.GenerateRaw() {
				raw.Origin = c
				requestSeq.Requests = append(requestSeq.Requests, raw)
			}
		}
		return []*RequestBulk{requestSeq}
	}

	var requests = make([]*RequestBulk, 0)
	switch c.AttackMode {
	case "sync", "pitchfork":
		for i := 0; i < maxLen; i++ {
			var vars = map[string]interface{}{}
			for k, v := range dicts {
				if i >= len(v) {
					vars[k] = ""
				} else {
					vars[k] = v[i]
				}
			}
			var requestsSeq []*requestRaw
			for _, req := range c.HTTPRequests {
				for _, raw := range req.GenerateRaw() {
					raw.Params = vars
					raw.Origin = c
					requestsSeq = append(requestsSeq, raw)
				}
			}
			if requestsSeq != nil {
				requests = append(requests, &RequestBulk{
					Requests:      requestsSeq,
					RequestConfig: c,
				})
			}
		}
	default:
		var indexToVar = map[int]string{}
		var data = make([][]string, len(dicts))
		var index int
		for k, v := range dicts {
			indexToVar[index] = k
			data[index] = v
			index++
		}
		mix, err := mixer.NewMixer(data...)
		if err != nil {
			log.Errorf("create mixer failed: %s", err)
			return requests
		}
		for {
			var vars = map[string]interface{}{}
			for index, data := range mix.Value() {
				vars[indexToVar[index]] = data
			}

			var requestSeq []*requestRaw
			for _, req := range c.HTTPRequests {
				for _, raw := range req.GenerateRaw() {
					raw.Params = vars
					raw.Origin = c
					requestSeq = append(requestSeq, raw)
				}
			}
			if len(requestSeq) > 0 {
				requests = append(requests, &RequestBulk{
					RequestConfig: c,
					Requests:      requestSeq,
				})
			}

			err := mix.Next()
			if err != nil {
				break
			}
		}
	}
	return requests
}

type YakHTTPRequestPacket struct {
	Request string
	// @SNI
	SNI string
	// @Timeout
	Timeout time.Duration
	// @Host
	OverrideHost string
}

func (s *YakHTTPRequestPacket) GenerateRaw() []*requestRaw {
	var requests = make([]*requestRaw, 0)
	var isHttps bool
	var err error
	_ = s.Request
	if err != nil {
		return nil
	}
	requests = append(requests, &requestRaw{
		Raw:     []byte(s.Request),
		IsHttps: isHttps,
	})
	return requests
}

func createVarsFromURL(u string) (map[string]interface{}, error) {
	https, raw, err := lowhttp.ParseUrlToHttpRequestRaw("GET", u)
	if err != nil {
		return nil, utils.Errorf("cannot convert url to http request: %v", err)
	}
	return createVarsFromHTTPRequest(https, raw)
}

func createVarsFromHTTPRequest(isHttps bool, s []byte) (map[string]interface{}, error) {
	req, err := lowhttp.ParseBytesToHttpRequest(s)
	if err != nil {
		return nil, err
	}
	extractedUrl, err := lowhttp.ExtractURLFromHTTPRequestRaw(s, isHttps)
	if err != nil {
		return nil, err
	}
	var host, port, _ = utils.ParseStringToHostPort(extractedUrl.String())
	baseUrl := extractedUrl.String()
	var rootUrl string
	if isHttps {
		if port == 443 {
			rootUrl = fmt.Sprintf("https://%v", host)
		} else {
			rootUrl = fmt.Sprintf("https://%v", utils.HostPort(host, port))
		}
	} else {
		if port == 80 {
			rootUrl = fmt.Sprintf("http://%v", host)
		} else {
			rootUrl = fmt.Sprintf("http://%v", utils.HostPort(host, port))
		}
	}
	hostname := utils.HostPort(host, port)
	pathRaw := req.RequestURI
	var file string
	if strings.Contains(pathRaw, "?") {
		pathNoQuery := pathRaw[:strings.Index(pathRaw, "?")]
		_, file = path.Split(pathNoQuery)
	}
	var schema string
	if isHttps {
		schema = "https"
	} else {
		schema = "http"
	}

	vars := map[string]interface{}{
		"url":                     extractedUrl.String(),
		"__host__":                host,
		"__port__":                port,
		"__hostname__":            hostname,
		"__root_url__":            rootUrl,
		"__base_url__":            baseUrl,
		"__path__":                pathRaw,
		"__path_trim_end_slash__": strings.TrimRight(pathRaw, "/"),
		"__file__":                file,
		"__schema__":              schema,
	}
	return vars, nil
}
