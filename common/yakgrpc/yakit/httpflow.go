package yakit

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	uuid "github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/domainextractor"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	globalHTTPFlowCache = utils.NewTTLCache[*ypb.HTTPFlow](10 * time.Minute)
	cacheMu             = new(sync.RWMutex)
)

const COLORPREFIX = "YAKIT_COLOR_"

func yakitColor(i string) string {
	return COLORPREFIX + i
}

func init() {
	RegisterPostInitDatabaseFunction(func() error {
		lowhttp.RegisterSaveHTTPFlowHandler(func(https bool, req []byte, rsp []byte, url string, remoteAddr string, reqSource string, runtimeId string, fromPlugin string, hiddenIndex string, payloads []string) {
			if rsp == nil || len(rsp) == 0 {
				return
			}
			db := consts.GetGormProjectDatabase()
			flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(https, req, rsp, "scan", url, remoteAddr)
			if err != nil {
				log.Errorf("create httpflow from lowhttp failed: %s", err)
				return
			}
			if reqSource != "" {
				flow.AddTag(reqSource)
			} else {
				if reqSource := os.Getenv(consts.YAKIT_PLUGIN_ID); reqSource != "" {
					flow.AddTag(reqSource)
				}
			}
			switch ret := strings.ToLower(reqSource); ret {
			case "mitm":
				flow.SourceType = "mitm"
			case "basic-crawler", "crawler", "crawlerx":
				flow.SourceType = "basic-crawler"
			case "scan", "port-scan", "plugin":
				flow.SourceType = "scan"

			}
			flow.FromPlugin = fromPlugin
			flow.RuntimeId = runtimeId
			flow.HiddenIndex = hiddenIndex
			flow.Payload = strings.Join(payloads, ",")
			err = InsertHTTPFlow(db, flow)
			if err != nil {
				log.Errorf("save httpflow failed: %s", err)
			}
		})
		return nil
	})
}

type HTTPFlow struct {
	gorm.Model

	HiddenIndex        string
	NoFixContentLength bool   `json:"no_fix_content_length"`
	Hash               string `gorm:"unique_index"`
	IsHTTPS            bool
	Url                string `gorm:"index"`
	Path               string
	Method             string
	BodyLength         int64
	ContentType        string
	StatusCode         int64
	SourceType         string
	Request            string
	Response           string
	GetParamsTotal     int
	PostParamsTotal    int
	CookieParamsTotal  int
	IPAddress          string
	RemoteAddr         string
	IPInteger          int
	Tags               string // 用来打标！
	Payload            string

	// Websocket 相关字段
	IsWebsocket bool
	// 用来计算 websocket hash, 每次连接都不一样，一般来说，内部对象 req 指针足够了
	WebsocketHash string

	RuntimeId  string
	FromPlugin string

	// friendly for gorm build instance, not for store
	// 这两个字段不参与数据库存储，但是在序列化的时候，会被覆盖
	// 主要用来标记用户的 Request 和 Response 是否超大
	IsRequestOversize  bool `gorm:"-"`
	IsResponseOversize bool `gorm:"-"`

	IsTooLargeResponse         bool
	TooLargeResponseHeaderFile string
	TooLargeResponseBodyFile   string
}

type TagAndStatusCode struct {
	Value string
	Count int
}

type CreateHTTPFlowConfig struct {
	isHttps     bool
	reqRaw      []byte
	rspRaw      []byte
	fixRspRaw   []byte // 如果设置了，则不会再修复rspRaw
	source      string
	url         string
	remoteAddr  string
	reqIns      *http.Request // 如果设置了，则不会再解析reqRaw
	hiddenIndex string
}

type CreateHTTPFlowOptions func(c *CreateHTTPFlowConfig)

func CreateHTTPFlowWithHTTPS(isHttps bool) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.isHttps = isHttps
	}
}

func CreateHTTPFlowWithRequestRaw(reqRaw []byte) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.reqRaw = reqRaw
	}
}

func CreateHTTPFlowWithResponseRaw(rspRaw []byte) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.rspRaw = rspRaw
	}
}

// 如果传入了fixRspRaw，则不会再修复
func CreateHTTPFlowWithFixResponseRaw(fixRspRaw []byte) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.fixRspRaw = fixRspRaw
	}
}

func CreateHTTPFlowWithSource(source string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.source = source
	}
}

func CreateHTTPFlowWithURL(url string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.url = url
	}
}

func CreateHTTPFlowWithRemoteAddr(remoteAddr string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.remoteAddr = remoteAddr
	}
}

// 如果传入了RequestIns，则优先使用这个作为NewFuzzRequest的参数
func CreateHTTPFlowWithRequestIns(reqIns *http.Request) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.reqIns = reqIns
	}
}

func (f *HTTPFlow) FixHTTPRequest(req *http.Request) {
	if req == nil {
		return
	}
	if httpctx.GetResponseTooLarge(req) {
		f.IsTooLargeResponse = true
		f.TooLargeResponseHeaderFile = httpctx.GetResponseTooLargeHeaderFile(req)
		f.TooLargeResponseBodyFile = httpctx.GetResponseTooLargeBodyFile(req)
		f.BodyLength = httpctx.GetResponseTooLargeSize(req)
	}
}

// 颜色与 Tag API
func (f *HTTPFlow) AddTag(appendTags ...string) {
	existed := utils.PrettifyListFromStringSplited(f.Tags, "|")
	existedCount := len(existed)
	extLen := len(appendTags)
	tags := make([]string, existedCount+extLen)
	copy(tags, existed)
	for i := 0; i < extLen; i++ {
		tags[i+existedCount] = appendTags[i]
	}
	f.Tags = strings.Join(utils.RemoveRepeatStringSlice(tags), "|")
}

func (f *HTTPFlow) AddTagToFirst(appendTags ...string) {
	existed := utils.PrettifyListFromStringSplited(f.Tags, "|")
	f.Tags = strings.Join(utils.RemoveRepeatStringSlice(append(appendTags, existed...)), "|")
}

func (f *HTTPFlow) RemoveColor() {
	f.Tags = strings.Join(lo.Filter(utils.PrettifyListFromStringSplited(f.Tags, "|"), func(i string, _ int) bool {
		return !strings.HasPrefix(i, COLORPREFIX)
	}), "|")
}

func (f *HTTPFlow) Red() {
	f.RemoveColor()
	f.AddTag(yakitColor("RED"))
}

func (f *HTTPFlow) Green() {
	f.RemoveColor()
	f.AddTag(yakitColor("GREEN"))
}

func (f *HTTPFlow) Blue() {
	f.RemoveColor()
	f.AddTag(yakitColor("BLUE"))
}

func (f *HTTPFlow) Yellow() {
	f.RemoveColor()
	f.AddTag(yakitColor("YELLOW"))
}

func (f *HTTPFlow) Orange() {
	f.RemoveColor()
	f.AddTag(yakitColor("ORANGE"))
}

func (f *HTTPFlow) Purple() {
	f.RemoveColor()
	f.AddTag(yakitColor("PURPLE"))
}

func (f *HTTPFlow) Cyan() {
	f.RemoveColor()
	f.AddTag(yakitColor("CYAN"))
}

func (f *HTTPFlow) Grey() {
	f.RemoveColor()
	f.AddTag(yakitColor("GREY"))
}

func (f *HTTPFlow) ColorSharp(rgbHex string) {
	f.RemoveColor()
	f.AddTag(yakitColor(rgbHex))
}

func FuzzerResponseToHTTPFlow(db *gorm.DB, rsp *ypb.FuzzerResponse) (*HTTPFlow, error) {
	return SaveFromHTTPFromRaw(db, rsp.IsHTTPS, rsp.RequestRaw, rsp.GetResponseRaw(), "fuzzer", rsp.GetUrl(), rsp.Host)
}

func FuzzParamsToGRPCFuzzableParam(r *mutate.FuzzHTTPRequestParam, isHttps bool) *ypb.FuzzableParam {
	p := &ypb.FuzzableParam{
		Position:  r.PositionVerbose(),
		ParamName: r.Name(),
		IsHTTPS:   isHttps,
	}

	switch ret := r.Value().(type) {
	case []string:
		if len(ret) == 1 {
			p.OriginValue = []byte(ret[0])
		} else {
			p.OriginValue = utils.InterfaceToBytes(r.Value())
		}
	default:
		p.OriginValue = utils.InterfaceToBytes(r.Value())
	}

	flag := utils.RandNumberStringBytes(6)
	res, err := r.Fuzz(flag).Results()
	if err != nil {
		return p
	}

	var raw []byte
	for _, r := range res {
		raw, err = utils.HttpDumpWithBody(r, true)
		if err != nil {
			continue
		}
	}
	if raw != nil {
		if bytes.Contains(raw, []byte(flag)) {
			p.AutoTemplate = bytes.ReplaceAll(raw, []byte(flag), []byte("{{randstr(10,10,1)}}"))
		} else if bytes.Contains(raw, []byte(codec.EncodeBase64(flag))) {
			p.AutoTemplate = bytes.ReplaceAll(raw, []byte(codec.EncodeBase64(flag)), []byte("{{base64({{randstr(10,10,1)}})}}"))
		}
	}
	return p
}

func (f *HTTPFlow) ToGRPCModel(full bool) (*ypb.HTTPFlow, error) {
	return f.toGRPCModel(full)
}

func (f *HTTPFlow) ToGRPCModelFull() (*ypb.HTTPFlow, error) {
	return f.toGRPCModel(true)
}

func utf8safe(i string) string {
	return utils.EscapeInvalidUTF8Byte([]byte(i))
}

func (f *HTTPFlow) getCacheGRPCModel(full bool) *ypb.HTTPFlow {
	if f == nil {
		return nil
	}
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if v, ok := globalHTTPFlowCache.Get(f.CalcCacheHash(full)); ok {
		return v
	}
	return nil
}

func (f *HTTPFlow) SetCacheGRPCModel(full bool, m *ypb.HTTPFlow) {
	if f == nil {
		return
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	globalHTTPFlowCache.Set(f.CalcCacheHash(full), m)
}

func (f *HTTPFlow) toGRPCModel(full bool) (*ypb.HTTPFlow, error) {
	if flow := f.getCacheGRPCModel(full); flow != nil {
		return flow, nil
	}
	flow := &ypb.HTTPFlow{
		Id:                         uint64(f.ID),
		IsHTTPS:                    f.IsHTTPS,
		Url:                        utf8safe(f.Url),
		SourceType:                 f.SourceType,
		Path:                       utf8safe(f.Path),
		Method:                     utf8safe(f.Method),
		BodyLength:                 f.BodyLength,
		ContentType:                utf8safe(f.ContentType),
		StatusCode:                 f.StatusCode,
		GetParamsTotal:             int64(f.GetParamsTotal),
		PostParamsTotal:            int64(f.PostParamsTotal),
		UpdatedAt:                  f.UpdatedAt.Unix(),
		CreatedAt:                  f.CreatedAt.Unix(),
		HostPort:                   utf8safe(f.RemoteAddr),
		IPAddress:                  utf8safe(f.IPAddress),
		HtmlTitle:                  "",
		Tags:                       f.Tags,
		NoFixContentLength:         f.NoFixContentLength,
		IsWebsocket:                f.IsWebsocket,
		WebsocketHash:              f.WebsocketHash,
		IsTooLargeResponse:         f.IsTooLargeResponse,
		TooLargeResponseBodyFile:   f.TooLargeResponseBodyFile,
		TooLargeResponseHeaderFile: f.TooLargeResponseHeaderFile,
		Payloads:                   strings.Split(f.Payload, ","),
	}
	// 设置 title
	var (
		unquotedResponse string
		unquotedRequest  string
		err              error
	)
	if f.Response != "" {
		unquotedResponse, err = strconv.Unquote(f.Response)
		if err != nil {
			log.Errorf("unquoted response failed: %s", err)
			fmt.Println(f.Response)
			unquotedResponse = f.Response
		}
	}
	flow.HtmlTitle = utf8safe(strings.TrimSpace(utils.ExtractTitleFromHTMLTitle(unquotedResponse, "")))

	if f.UpdatedAt.IsZero() {
		flow.UpdatedAt = time.Now().Unix()
		flow.CreatedAt = time.Now().Unix()
	}
	if f.Hash == "" {
		flow.Hash = f.CalcHash()
	} else {
		flow.Hash = f.Hash
	}
	host, port, _ := utils.ParseStringToHostPort(flow.Url)
	flow.HostPort = utils.HostPort(host, port)

	flow.BodySizeVerbose = utils.ByteSize(uint64(flow.BodyLength))

	if f.Request != "" {
		unquotedRequest, err = strconv.Unquote(f.Request)
		if err != nil {
			unquotedRequest = f.Request
			log.Errorf("unquoted request failed: %s", err)
			fmt.Println(f.Request)
		}
	}

	flow.RequestLength = int64(len(unquotedRequest))
	flow.RequestSizeVerbose = utils.ByteSize(uint64(len(unquotedRequest)))

	requireRequest := full || !f.IsRequestOversize
	requireResponse := full || !f.IsResponseOversize
	isStandardRequest := !flow.NoFixContentLength

	haveRequest := funk.NotEmpty(unquotedRequest)
	var requestBody []byte
	if requireRequest {
		// return request:
		// 1. request is not required (!full)
		// 2. not a large request
		// 3. request is not oversize
		flow.Request = []byte(unquotedRequest)
		if isStandardRequest && haveRequest {
			// unfix request:
			var requestURI string
			_, requestBody = lowhttp.SplitHTTPPacket(flow.Request, func(method string, requestUri string, proto string) error {
				requestURI = requestUri
				return nil
			}, nil, func(line string) string {
				k, v := lowhttp.SplitHTTPHeader(line)
				flow.RequestHeader = append(flow.RequestHeader, &ypb.HTTPHeader{
					Header: utf8safe(utils.ParseStringToVisible(k)),
					Value:  utf8safe(utils.ParseStringToVisible(v)),
				})
				return line
			})
			if flow.Path == "" {
				flow.Path = utf8safe(requestURI)
			}

			fReq, _ := mutate.NewFuzzHTTPRequest(flow.Request, mutate.OptHTTPS(flow.IsHTTPS))
			if fReq != nil {
				for _, r := range fReq.GetCommonParams() {
					fReq := &ypb.FuzzableParam{
						Position:  r.PositionVerbose(),
						ParamName: utf8safe(utils.ParseStringToVisible(r.Name())),
						IsHTTPS:   flow.IsHTTPS,
					}

					if full {
						// 详情模式，这个很耗时。
						fReq = FuzzParamsToGRPCFuzzableParam(r, flow.IsHTTPS)
					}
					fReq.ParamName = utils.EscapeInvalidUTF8Byte([]byte(fReq.ParamName))
					if r.IsGetParams() {
						flow.GetParams = append(flow.GetParams, fReq)
					}
					if r.IsPostParams() {
						flow.PostParams = append(flow.PostParams, fReq)
					}
					if r.IsCookieParams() {
						flow.CookieParams = append(flow.CookieParams, fReq)
					}
				}

				flow.GetParamsTotal = int64(len(flow.GetParams))
				flow.PostParamsTotal = int64(len(flow.PostParams))
				flow.CookieParamsTotal = int64(len(flow.CookieParams))
			}
		}
	}

	haveResponse := funk.NotEmpty(unquotedResponse)
	var responseBody []byte
	if requireResponse {
		flow.Response = []byte(unquotedResponse)
		flow.DisableRenderStyles = len(flow.Response) > 2*1000*1000
		if isStandardRequest && haveResponse {
			_, responseBody = lowhttp.SplitHTTPPacket(flow.Response, nil, nil, func(line string) string {
				k, v := lowhttp.SplitHTTPHeader(line)
				flow.ResponseHeader = append(flow.ResponseHeader, &ypb.HTTPHeader{
					Header: utf8safe(utils.ParseStringToVisible(k)),
					Value:  utf8safe(utils.ParseStringToVisible(v)),
				})
				return line
			})
		}
	}

	// 这里用来标记一下，UTF8 支持情况，要根据情况提供给用户合理 body 建议处理方案
	if requireRequest {
		flow.InvalidForUTF8Request = !utf8.ValidString(unquotedRequest)
		if flow.InvalidForUTF8Request {
			flow.RawRequestBodyBase64 = codec.EncodeBase64(requestBody)
			// 这里需要修复请求，这是为了解决Content-Type与body中boundary不一致的问题
			fixReq := lowhttp.FixHTTPRequest(flow.Request)
			flow.SafeHTTPRequest = codec.EscapeInvalidUTF8Byte(lowhttp.ConvertHTTPRequestToFuzzTag(fixReq))
		}
	}

	if requireResponse {
		flow.InvalidForUTF8Response = !utf8.ValidString(unquotedResponse)
		if flow.InvalidForUTF8Response {
			flow.RawResponseBodyBase64 = codec.EncodeBase64(responseBody)
		}
	}

	// 提取数据 - 完整详细的数据的时候，才应该提取
	if full {
		domains, rootDomains := domainextractor.ExtractDomainsEx(string(flow.Response))
		var jsonObjects []string
		if !utils.MatchAnyOfSubString(strings.ToLower(f.ContentType), "json") {
			if len(flow.Response) > 200*1000 {
				jsonObjects = jsonextractor.ExtractStandardJSON(string(flow.Response[:200*1000]))
			} else {
				jsonObjects = jsonextractor.ExtractStandardJSON(string(flow.Response))
			}
		}
		flow.Domains = make([]string, len(domains))
		for i, d := range domains {
			flow.Domains[i] = utils.EscapeInvalidUTF8Byte([]byte(d))
		}
		flow.RootDomains = make([]string, len(rootDomains))
		for i, d := range rootDomains {
			flow.RootDomains[i] = utils.EscapeInvalidUTF8Byte([]byte(d))
		}
		for _, j := range jsonObjects {
			if j == "" || j == "&&" || j == "{}" {
				continue
			}

			if !utf8.ValidString(j) {
				continue
			}
			if strings.HasPrefix(j, `{"":`) {
				continue
			}
			flow.JsonObjects = append(flow.JsonObjects, utf8safe(j))
		}
	}
	f.SetCacheGRPCModel(full, flow)
	return flow, nil
}

func (f *HTTPFlow) CalcHash() string {
	return utils.CalcSha1(f.IsHTTPS, f.Url, f.Request, f.HiddenIndex, f.RuntimeId, f.FromPlugin)
}

func (f *HTTPFlow) CalcCacheHash(full bool) string {
	return utils.CalcSha1(f.ID, f.IsHTTPS, f.Url, f.Request, f.HiddenIndex, f.RuntimeId, f.FromPlugin, f.Response, full)
}

func (f *HTTPFlow) BeforeSave() error {
	f.fixURL()
	f.Hash = f.CalcHash()
	return nil
}

func (f *HTTPFlow) fixURL() {
	urlIns := utils.ParseStringToUrl(f.Url)
	if f.IsHTTPS {
		urlIns.Scheme = "https"
	}
	if urlIns != nil {
		host, port, _ := utils.ParseStringToHostPort(urlIns.Host)
		if (port == 443 && urlIns.Scheme == "https") || (port == 80 && urlIns.Scheme == "http") {
			urlIns.Host = host
			f.Url = urlIns.String()
		}
	}
}

func (f *HTTPFlow) AfterCreate(tx *gorm.DB) (err error) {
	BroadcastData("httpflow", "create")
	return nil
}

func (f *HTTPFlow) AfterSave(tx *gorm.DB) (err error) {
	BroadcastData("httpflow", "save")
	return nil
}

func (f *HTTPFlow) AfterUpdate(tx *gorm.DB) (err error) {
	BroadcastData("httpflow", "update")
	return nil
}

func (f *HTTPFlow) AfterDelete(tx *gorm.DB) (err error) {
	BroadcastData("httpflow", "delete")
	return nil
}

func SaveFromHTTP(db *gorm.DB, isHttps bool, req *http.Request, rsp *http.Response, source string, url string, remoteAddr string) (*HTTPFlow, error) {
	return SaveFromHTTPWithBodySaved(db, isHttps, req, rsp, source, url, remoteAddr)
}

func SaveFromHTTPFromRaw(db *gorm.DB, isHttps bool, req []byte, rsp []byte, source string, url string, remoteAddr string) (*HTTPFlow, error) {
	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(isHttps, req, rsp, source, url, remoteAddr)
	if err != nil {
		return nil, utils.Errorf("create httpflow failed: %s", err)
	}
	err = CreateOrUpdateHTTPFlow(db, flow.CalcHash(), flow)
	if err != nil {
		return nil, err
	}
	return flow, nil
}

func SaveFromHTTPWithBodySaved(db *gorm.DB, isHttps bool, req *http.Request, rsp *http.Response, source string, url string, remoteAddr string) (*HTTPFlow, error) {
	flow, err := CreateHTTPFlowFromHTTPWithBodySaved(isHttps, req, rsp, source, url, remoteAddr)
	if err != nil {
		return nil, utils.Errorf("create httpflow failed: %s", err)
	}
	err = CreateOrUpdateHTTPFlow(db, flow.CalcHash(), flow)
	if err != nil {
		return nil, err
	}
	return flow, nil
}

const maxBodyLength = 4 * 1024 * 1024

func CreateHTTPFlow(opts ...CreateHTTPFlowOptions) (*HTTPFlow, error) {
	c := &CreateHTTPFlowConfig{}
	for _, opt := range opts {
		opt(c)
	}

	var (
		isHttps    = c.isHttps
		reqRaw     = c.reqRaw
		rspRaw     = c.rspRaw
		fixRspRaw  = c.fixRspRaw
		source     = c.source
		url        = c.url
		remoteAddr = c.remoteAddr
		reqIns     = c.reqIns
	)

	var (
		method     string
		requestUri string
		fReq       *mutate.FuzzHTTPRequest
	)

	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketEx(reqRaw, func(m string, r string, proto string) error {
		method = m
		requestUri = r
		return nil
	})

	if false && len(body) > maxBodyLength {
		// Truncated by saver
		reqRaw = lowhttp.ReplaceHTTPPacketBody([]byte(header), body[:maxBodyLength], false)
	}
	requestRaw := strconv.Quote(string(reqRaw))
	if strings.HasPrefix(requestRaw, `"HTTP/1.`) {
		log.Errorf("[BUG] requestRaw is invalid: %s", requestRaw)
		log.Errorf("[BUG] requestRaw is invalid: %s", requestRaw)
		log.Errorf("[BUG] requestRaw is invalid: %s", requestRaw)
	}

	// 如果已经修复过响应，则不会再修复
	if len(fixRspRaw) == 0 {
		rawNoGzip, _, _ := lowhttp.FixHTTPResponse(rspRaw)
		if len(rawNoGzip) > 0 {
			rspRaw = rawNoGzip
		}
	} else {
		rspRaw = fixRspRaw
	}
	if rspRaw == nil {
		rspRaw = make([]byte, 0)
	}

	var rspContentType string
	header, body = lowhttp.SplitHTTPHeadersAndBodyFromPacket(rspRaw, func(line string) {
		k, v := lowhttp.SplitHTTPHeader(line)
		if strings.ToLower(k) == "content-type" {
			rspContentType = v
		}
	})
	responseRaw := strconv.Quote(string(rspRaw))
	// 如果设置了 reqIns，则不会再解析 reqRaw
	if reqIns != nil {
		fReq, _ = mutate.NewFuzzHTTPRequest(reqIns)
	} else {
		fReq, _ = mutate.NewFuzzHTTPRequest(reqRaw)
	}

	flow := &HTTPFlow{
		IsHTTPS:     isHttps,
		Url:         url,
		Path:        requestUri,
		Method:      method,
		BodyLength:  int64(len(body)),
		ContentType: rspContentType,
		StatusCode:  int64(lowhttp.ExtractStatusCodeFromResponse(rspRaw)),
		SourceType:  source,
		Request:     requestRaw,
		Response:    responseRaw,
		RemoteAddr:  remoteAddr,
		HiddenIndex: uuid.NewString(),
	}
	ip, _, _ := utils.ParseStringToHostPort(remoteAddr)
	if ip != "" {
		flow.IPAddress = ip
		ipInt, _ := utils.IPv4ToUint64(ip)
		if ipInt > 0 {
			flow.IPInteger = int(ipInt)
		}
	}

	if fReq != nil {
		flow.GetParamsTotal = len(fReq.GetGetQueryParams())

		postParams := fReq.GetPostJsonParams()
		if len(postParams) <= 0 {
			postParams = fReq.GetPostXMLParams()
		}
		if len(postParams) <= 0 {
			postParams = fReq.GetPostParams()
		}
		flow.PostParamsTotal = len(postParams)

		flow.CookieParamsTotal = len(fReq.GetCookieParams())
	}

	flow.Hash = flow.CalcHash()
	return flow, nil
}

func CreateHTTPFlowFromHTTPWithBodySavedFromRaw(isHttps bool, reqRaw []byte, rspRaw []byte, source string, url string, remoteAddr string, opts ...CreateHTTPFlowOptions) (*HTTPFlow, error) {
	extOpts := []CreateHTTPFlowOptions{
		CreateHTTPFlowWithHTTPS(isHttps), CreateHTTPFlowWithRequestRaw(reqRaw), CreateHTTPFlowWithResponseRaw(rspRaw), CreateHTTPFlowWithSource(source), CreateHTTPFlowWithURL(url), CreateHTTPFlowWithRemoteAddr(remoteAddr),
	}
	extOpts = append(extOpts, opts...)
	flow, err := CreateHTTPFlow(extOpts...)
	if err != nil {
		return nil, err
	}
	return flow, nil
}

func CreateHTTPFlowFromHTTPWithNoRspSaved(isHttps bool, req *http.Request, source string, url string, remoteAddr string, opts ...CreateHTTPFlowOptions) (*HTTPFlow, error) {
	urlRaw := url
	if urlRaw == "" {
		u, err := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
		if err != nil {
			log.Warnf("extract url from request failed: %s", err)
		}
		if u != nil {
			urlRaw = u.String()
		} else {
			if isHttps {
				urlRaw = "https://" + remoteAddr
			} else {
				urlRaw = "http://" + remoteAddr
			}
		}
	}

	reqRaw := httpctx.GetRequestBytes(req)
	if reqRaw == nil {
		var err error
		reqRaw, err = utils.HttpDumpWithBody(req, true)
		if err != nil {
			reqRaw, err = utils.HttpDumpWithBody(req, false)
			if err != nil {
				log.Errorf("dump request failed: %s", err)
			}
		}
	}

	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(isHttps, reqRaw, make([]byte, 0), source, urlRaw, remoteAddr)
	if err != nil {
		return nil, err
	}
	return flow, nil
}

func CreateHTTPFlowFromHTTPWithBodySaved(isHttps bool, req *http.Request, rsp *http.Response, source string, url string, remoteAddr string, opts ...CreateHTTPFlowOptions) (*HTTPFlow, error) {
	urlRaw := url
	if urlRaw == "" {
		u, err := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
		if err != nil {
			log.Warnf("extract url from request failed: %s", err)
		}
		if u != nil {
			urlRaw = u.String()
		} else {
			if isHttps {
				urlRaw = "https://" + remoteAddr
			} else {
				urlRaw = "http://" + remoteAddr
			}
		}
	}

	reqRaw := httpctx.GetRequestBytes(req)
	if reqRaw == nil {
		var err error
		reqRaw, err = utils.HttpDumpWithBody(req, true)
		if err != nil {
			reqRaw, err = utils.HttpDumpWithBody(req, false)
			if err != nil {
				log.Errorf("dump request failed: %s", err)
			}
		}
	}

	rspRaw, err := utils.HttpDumpWithBody(rsp, true)
	if err != nil {
		log.Errorf("dump response failed: %s", err)
	}
	return CreateHTTPFlowFromHTTPWithBodySavedFromRaw(isHttps, reqRaw, rspRaw, source, urlRaw, remoteAddr, opts...)
}

func UpdateHTTPFlowTags(db *gorm.DB, i *HTTPFlow) error {
	if i == nil {
		return nil
	}
	db = db.Model(&HTTPFlow{})

	if i.ID > 0 {
		if db = db.Where("id = ?", i.ID).Update("tags", i.Tags); db.Error != nil {
			log.Errorf("update tags(by id) failed: %s", db.Error)
			return db.Error
		}
	} else if i.HiddenIndex != "" {
		if db = db.Where("hidden_index = ?", i.HiddenIndex).Update("tags", i.Tags); db.Error != nil {
			log.Errorf("update tags(by hidden_index) failed: %s", db.Error)
			return db.Error
		}
	} else if i.Hash != "" {
		if db = db.Where("hash = ?", i.HiddenIndex).Update("tags", i.Tags); db.Error != nil {
			log.Errorf("update tags(by hash) failed: %s", db.Error)
			return db.Error
		}
	}
	return nil
}

func InsertHTTPFlow(db *gorm.DB, i *HTTPFlow) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
			debug.PrintStack()
		}
	}()

	i.ID = 0
	if db = db.Model(&HTTPFlow{}).Save(i); db.Error != nil {
		return utils.Errorf("insert HTTPFlow failed: %s", db.Error)
	}

	return nil
}

func CreateOrUpdateHTTPFlow(db *gorm.DB, hash string, i *HTTPFlow) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
		}
	}()

	db = db.Model(&HTTPFlow{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(i); db.Error != nil {
		return utils.Errorf("create/update HTTPFlow failed: %s", db.Error)
	}
	return nil
}

func GetHTTPFlow(db *gorm.DB, id int64) (*HTTPFlow, error) {
	var req HTTPFlow
	if db := db.Model(&HTTPFlow{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get HTTPFlow failed: %s", db.Error)
	}

	return &req, nil
}

func GetHTTPFlowByIDOrHash(db *gorm.DB, id int64, hash string) (*HTTPFlow, error) {
	var req HTTPFlow
	if db := db.Model(&HTTPFlow{}).Where("id = ? OR hash = ?", id, hash).First(&req); db.Error != nil {
		return nil, utils.Errorf("get HTTPFlow failed: %s", db.Error)
	}

	return &req, nil
}

func GetHTTPFlowByHash(db *gorm.DB, hash string) (*HTTPFlow, error) {
	var req HTTPFlow
	if db := db.Model(&HTTPFlow{}).Where("hash = ?", hash).First(&req); db.Error != nil {
		return nil, utils.Errorf("get HTTPFlow failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteHTTPFlowByID(db *gorm.DB, id int64) error {
	if db := db.Model(&HTTPFlow{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&HTTPFlow{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteHTTPFlow(db *gorm.DB, req *ypb.DeleteHTTPFlowRequest) error {
	if req.GetDeleteAll() {
		db.DropTableIfExists(&HTTPFlow{})
		if db := db.Exec(`UPDATE SQLITE_SEQUENCE SET SEQ=0 WHERE NAME='http_flows';`); db.Error != nil {
			log.Errorf("update sqlite sequence failed: %s", db.Error)
		}
		db.AutoMigrate(&HTTPFlow{})
		DeleteProjectKeyBareRequestAndResponse(db)
		return nil
	}

	if len(req.GetId()) > 0 {
		db = db.Or("false")
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", req.GetId())
		// for _, id := range req.GetId() {
		// 	db = db.Or("id = ?", id)
		// }
		db.Unscoped().Delete(&HTTPFlow{})
		return nil
	}

	if req.GetFilter() != nil {
		db = FilterHTTPFlow(db, req.GetFilter())
		db.Unscoped().Delete(&HTTPFlow{})
		return nil
	}

	if req.GetURLPrefix() != "" {
		db = db.Model(&HTTPFlow{})
		db = bizhelper.FuzzQueryLike(db, "url", req.GetURLPrefix()).Unscoped().Delete(&HTTPFlow{})
		if db.Error != nil {
			return nil
		}
		return nil
	}

	if req.GetItemHash() != nil {
		db = db.Model(&HTTPFlow{})
		db = bizhelper.ExactQueryStringArrayOr(db, "hash", req.GetItemHash())
		if db := db.Where("true").Unscoped().Delete(&HTTPFlow{}); db.Error != nil {
			return db.Error
		}
		return nil
	}

	if len(req.GetURLPrefixBatch()) > 0 {
		db = db.Model(&HTTPFlow{})
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "url", req.GetURLPrefixBatch())
		db = db.Unscoped().Delete(&HTTPFlow{})
		if db.Error != nil {
			return db.Error
		}
		return nil
	}
	return nil
}

func FilterHTTPFlow(db *gorm.DB, params *ypb.QueryHTTPFlowRequest) *gorm.DB {
	db = db.Model(&HTTPFlow{}) //.Debug()
	if params == nil {
		params = &ypb.QueryHTTPFlowRequest{}
	}

	db = bizhelper.ExactQueryStringArrayOr(db, "source_type", utils.PrettifyListFromStringSplited(params.SourceType, ","))
	// 过滤 Methods
	if ms := utils.StringArrayFilterEmpty(utils.PrettifyListFromStringSplited(params.GetMethods(), ",")); ms != nil {
		db = bizhelper.ExactQueryStringArrayOr(db, "method", ms)
	}
	// 搜索 URL
	db = bizhelper.FuzzQueryLike(db, "url", params.GetSearchURL())
	// status code 这里可以支持范围搜索
	db = bizhelper.QueryBySpecificPorts(db, "status_code", params.GetStatusCode())
	if params.GetHaveBody() {
		db = db.Where("body_length > 0")
	}
	db = bizhelper.ExactQueryString(db, "runtime_id", params.GetRuntimeId())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "runtime_id", params.GetRuntimeIDs())
	db = bizhelper.ExactQueryString(db, "from_plugin", params.GetFromPlugin())

	// 搜索是否有对应的参数
	if params.GetHaveCommonParams() {
		db = db.Where("((get_params_total > 0) OR (post_params_total > 0)) OR (cookie_params_total > 0)")
	}

	if params.GetHaveParamsTotal() == "true" {
		db = db.Where("((get_params_total > 0) OR (post_params_total > 0))")
	} else if params.GetHaveParamsTotal() == "false" {
		db = db.Where("((get_params_total = 0) and (post_params_total = 0))")
	}

	if len(params.GetTags()) > 0 {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"tags"}, params.GetTags(), false)
	}

	if len(params.GetColor()) > 0 {
		db = bizhelper.FuzzSearchWithStringArrayOrAf(db, []string{"tags"}, params.GetColor(), false)
	}

	// 搜索 Content-Type
	db = bizhelper.FuzzQueryStringArrayOrLike(db, "content_type",
		utils.StringArrayFilterEmpty(utils.PrettifyListFromStringSplited(params.GetSearchContentType(), ",")))

	if len(params.GetIncludeInUrl()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "url", params.GetIncludeInUrl())
	}

	if len(params.GetIncludeInIP()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "ip_address", params.GetIncludeInIP())
	}

	if len(params.GetIncludeId()) > 0 {
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", params.GetIncludeId())
	}

	if len(params.GetExcludeInUrl()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "url", params.GetExcludeInUrl())
	}

	if len(params.GetExcludeInIP()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "ip_address", params.GetExcludeInIP())
	}

	if len(params.GetIncludePath()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "path", params.GetIncludePath())
	}

	if len(params.GetExcludePath()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "path", params.GetExcludePath())
	}

	if len(params.GetIncludeSuffix()) > 0 {
		var suffixes []string
		for _, suffix := range params.GetIncludeSuffix() {
			if !strings.HasPrefix(suffix, ".") {
				suffix = "." + suffix
			}
			suffixes = append(suffixes, suffix)
		}
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "path", suffixes)
	}
	if len(params.GetExcludeSuffix()) > 0 {
		var suffixes []string
		for _, suffix := range params.GetIncludeSuffix() {
			if !strings.HasPrefix(suffix, ".") {
				suffix = "." + suffix
			}
			suffixes = append(suffixes, suffix)
		}
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "path", suffixes)
	}
	if len(params.GetExcludeContentType()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "content_type", params.GetExcludeContentType())
	}

	if len(params.GetExcludeId()) > 0 {
		db = bizhelper.ExactExcludeQueryInt64Array(db, "id", params.GetExcludeId())
	}

	if len(params.GetIncludeHash()) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "hash", params.GetIncludeHash())
	}

	if params.AfterBodyLength > 0 {
		db = db.Where("body_length >= ?", params.AfterBodyLength)
	}
	if params.BeforeBodyLength > 0 {
		db = db.Where("body_length <= ?", params.BeforeBodyLength)
	}

	return db
}

func QuickSearchHTTPFlowCount(token string) int {
	db := consts.GetGormProjectDatabase()
	var count int
	db.Model(&HTTPFlow{}).Where(
		"(request like ?) OR (response like ?) OR (url like ?)",
		"%"+token+"%",
		"%"+token+"%",
		"%"+token+"%",
	).Count(&count)
	return count
}

func QuickSearchMITMHTTPFlowCount(token string) int {
	db := consts.GetGormProjectDatabase()
	var count int
	db.Model(&HTTPFlow{}).Where(
		"(request like ?) OR (response like ?) OR (url like ?)",
		"%"+token+"%",
		"%"+token+"%",
		"%"+token+"%",
	).Where("source_type = ?", "mitm").Count(&count)
	return count
}

// BuildHTTPFlowQuery 构建带有过滤条件的查询
func BuildHTTPFlowQuery(db *gorm.DB, params *ypb.QueryHTTPFlowRequest) *gorm.DB {
	// 应用所有过滤条件
	if params == nil {
		params = &ypb.QueryHTTPFlowRequest{}
	}

	if !params.GetFull() {
		extraSelectField := ""
		if params.GetWithPayload() {
			extraSelectField = "payload,"
		}
		// 只查询部分字段，主要是为了处理大的 response 和 request 的情况，同时告诉用户
		// max request size is 200K -> 200 * 1024 -> 204800
		// max response size is 500K -> 500 * 1024 -> 512000
		db = db.Select(fmt.Sprintf(`id,created_at,updated_at,hidden_index,%s -- basic gorm fields
body_length, -- handle body length should be careful, if it's big, no return response

-- metainfo
is_http_s, -- legacy
no_fix_content_length, hash,url, path, method,
content_type, status_code, source_type,
get_params_total, post_params_total, cookie_params_total,
ip_address, remote_addr, ip_integer,
tags, is_websocket, websocket_hash, runtime_id, from_plugin,

-- request is larger than 200K, return empty string
LENGTH(request) > 204800 as is_request_oversize,
CASE WHEN LENGTH(request) > 204800 THEN '' ELSE request END as request,

-- response is larger than 500K, return empty string
LENGTH(response) > 512000 as is_response_oversize,
CASE WHEN LENGTH(response) > 512000 THEN '' ELSE response END as response,

-- is response too large
is_too_large_response, 
too_large_response_header_file, too_large_response_body_file
`, extraSelectField))
	}

	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	p := params.Pagination
	if p.OrderBy == "" {
		p.OrderBy = "id" // 如果 没有设置 orderby 则以ID排序
	}

	if params.GetAfterUpdatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", params.GetAfterUpdatedAt(), time.Now().Add(10*time.Minute).Unix())
	}
	db = bizhelper.FuzzSearchEx(db, []string{
		"tags", "url", "path", "request",
		"response", "remote_addr",
	}, params.GetKeyword(), false)
	if params.GetAfterId() > 0 {
		db = db.Where("id > ?", params.GetAfterId())
	}
	if params.GetBeforeUpdatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", 0, params.GetBeforeUpdatedAt())
	}
	if params.GetBeforeId() > 0 {
		db = db.Where("id < ?", params.GetBeforeId())
	}

	if params.GetOnlyWebsocket() {
		// log.Info("query websocket request flow")
		db = db.Where("(is_websocket = ?) AND (url LIKE 'ws%')", params.OnlyWebsocket)
	}
	switch params.GetIsWebsocket() {
	case "http/https":
		db = db.Where("is_websocket = false")
	case "websocket":
		db = db.Where("(is_websocket = true) AND (url LIKE 'ws%')")
	}

	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)

	db = FilterHTTPFlow(db, params)

	return db
}

func QueryHTTPFlow(db *gorm.DB, params *ypb.QueryHTTPFlowRequest) (paging *bizhelper.Paginator, httpflows []*HTTPFlow, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	// todo: remove this
	db = db.Debug()
	queryDB := BuildHTTPFlowQuery(db.Model(&HTTPFlow{}), params)

	return SelectHTTPFlowFromDB(queryDB, params)
}

func SelectHTTPFlowFromDB(queryDB *gorm.DB, params *ypb.QueryHTTPFlowRequest) (paging *bizhelper.Paginator, httpflows []*HTTPFlow, err error) {
	var limitFlows, fullFlows []*HTTPFlow

	if params.OffsetId > 0 {
		offsetDB := queryDB
		if params.Pagination.Order == "desc" {
			offsetDB = offsetDB.Where("id < ?", params.OffsetId)
		} else {
			offsetDB = offsetDB.Where("id > ?", params.OffsetId)
		}
		offsetDB.Limit(int(params.Pagination.Limit)).Offset(0).Scan(&limitFlows)
		paging, queryDB = bizhelper.Paging(queryDB, int(params.Pagination.Page), int(params.Pagination.Limit), &fullFlows)
	} else {
		paging, queryDB = bizhelper.Paging(queryDB, int(params.Pagination.Page), int(params.Pagination.Limit), &limitFlows)
	}

	if queryDB.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", queryDB.Error)
	}

	return paging, limitFlows, nil
}

type HTTPFlowUrl struct {
	Url string `json:"url"`
}

func YieldHTTPUrl(db *gorm.DB, ctx context.Context) chan *HTTPFlowUrl {
	outC := make(chan *HTTPFlowUrl)
	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*HTTPFlowUrl
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

func YieldHTTPFlows(db *gorm.DB, ctx context.Context) chan *HTTPFlow {
	outC := make(chan *HTTPFlow)
	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*HTTPFlow
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

const (
	HTTPFLOW_TAG        = "HTTPFLOW_TAG"
	HTTPFLOW_STATUSCODE = "HTTPFLOW_STATUSCODE"
)

/*func HTTPFlowStatusCode(refreshRequest bool) (req []*TagAndStatusCode, err error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("cannot found database config")
		return nil, utils.Error("empty database")
	}
	if !refreshRequest {
		value := GetKey(db, HTTPFLOW_STATUSCODE)
		if value != "" {
			var statusCode []*TagAndStatusCode
			_ = json.Unmarshal([]byte(value), &statusCode)
			if len(statusCode) > 0 {
				return statusCode, nil
			}
		}
	}

	// log.Info("start to execute updating tags")
	db = db.Raw(`SELECT count(*) as count, status_code as value FROM http_flows GROUP BY status_code order by count desc;`)
	rows, err := db.Rows()
	if err != nil {
		return nil, utils.Errorf("rows failed: %s", err)
	}

	var statusCode = make([]*TagAndStatusCode, 0)
	for rows.Next() {
		var codeName string
		var count int
		err = rows.Scan(&count, &codeName)
		if err != nil {
			log.Errorf("scan code stats failed: %s", err)
			continue
		}
		statusCode = append(statusCode, &TagAndStatusCode{
			Value: codeName,
			Count: count,
		})
	}

	raw, _ := json.Marshal(statusCode)
	if len(raw) > 0 {
		log.Infof("start to cache statusCode[%v]", len(raw))
		SetKey(consts.GetGormProfileDatabase(), HTTPFLOW_STATUSCODE, string(raw))
	}
	return statusCode, nil
}*/

func HTTPFlowTags(refreshRequest bool) ([]*TagAndStatusCode, error) {
	tagCounts := make(map[string]int)
	for _, v := range globalHTTPFlowCache.GetAll() {
		for _, tag := range strings.Split(v.Tags, "|") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tagCounts[tag]++
			}
		}
	}
	tags := make([]*TagAndStatusCode, 0)
	for k, v := range tagCounts {
		tags = append(tags, &TagAndStatusCode{
			Value: k,
			Count: v,
		})
	}
	return tags, nil
}

func QueryWebsocketFlowsByHTTPFlowHash(db *gorm.DB, req *ypb.DeleteHTTPFlowRequest) *gorm.DB {
	db = db.Model(&HTTPFlow{})

	if len(req.GetId()) > 0 {
		db = db.Or("false")
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", req.GetId())
	}

	if req.GetFilter() != nil {
		db = FilterHTTPFlow(db, req.GetFilter())
	}

	if req.GetURLPrefix() != "" {
		db = bizhelper.FuzzQueryLike(db, "url", req.GetURLPrefix())
	}
	if len(req.GetURLPrefixBatch()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "url", req.GetURLPrefixBatch())
	}
	if req.GetItemHash() != nil {
		db = bizhelper.ExactQueryStringArrayOr(db, "hash", req.GetItemHash())
		db = db.Where("true")
	}
	return db
}

func ExportHTTPFlow(db *gorm.DB, params *ypb.ExportHTTPFlowsRequest) (paging *bizhelper.Paginator, ret []*HTTPFlow, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	queryParams := params.ExportWhere
	queryParams.IncludeId = params.Ids

	db = db.Model(&HTTPFlow{})
	// overwrite Select Field, fix payloads
	for i, field := range params.FieldName {
		if field != "payloads" {
			continue
		}
		queryParams.WithPayload = true
		params.FieldName[i] = "payload"
	}

	queryDB := BuildHTTPFlowQuery(db, queryParams).Select(params.FieldName)
	return SelectHTTPFlowFromDB(queryDB, queryParams)
}
