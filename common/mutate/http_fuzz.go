package mutate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"
)

type FuzzHTTPRequest struct {
	Opts                   []BuildFuzzHTTPRequestOption
	isHttps                bool
	source                 string
	runtimeId              string
	proxy                  string
	originRequest          []byte
	_originRequestInstance *http.Request
	chunked                bool
}

type FuzzHTTPRequestIf interface {
	Repeat(i int) FuzzHTTPRequestIf

	// 模糊测试 http.Request 的 method 字段
	FuzzMethod(method ...string) FuzzHTTPRequestIf

	// 模糊测试 Path 字段
	FuzzPath(...string) FuzzHTTPRequestIf

	// FuzzPathAppend 模糊测试 Path，追加 Path
	FuzzPathAppend(...string) FuzzHTTPRequestIf

	// 模糊测试 HTTPHeader 字段
	FuzzHTTPHeader(interface{}, interface{}) FuzzHTTPRequestIf

	// 模糊测试 Query
	FuzzGetParamsRaw(queryRaw ...string) FuzzHTTPRequestIf

	// 模糊测试 Query 中的字段
	FuzzGetParams(interface{}, interface{}) FuzzHTTPRequestIf

	// 模糊测试被Base64编码后Query中的字段
	FuzzGetBase64Params(interface{}, interface{}) FuzzHTTPRequestIf

	// FuzzGetParamJson
	FuzzGetJsonPathParams(any, string, any) FuzzHTTPRequestIf

	// 模糊测试 Post
	FuzzPostRaw(...string) FuzzHTTPRequestIf

	// 模糊测试 PostParam
	FuzzPostParams(k, v interface{}) FuzzHTTPRequestIf

	// 模糊测试被Base64编码后Post中的字段
	FuzzPostBase64Params(k, v interface{}) FuzzHTTPRequestIf

	// 测试 PostJson 中的数据
	FuzzPostJsonParams(k, v interface{}) FuzzHTTPRequestIf

	// 测试 PostJsonPath 中的数据
	FuzzPostJsonPathParams(k any, jp string, v any) FuzzHTTPRequestIf

	// 测试 Cookie 中的数据
	FuzzCookieRaw(value interface{}) FuzzHTTPRequestIf

	// 按键值对测试 Cookie 中的数据
	FuzzCookie(k, v interface{}) FuzzHTTPRequestIf

	// 模糊测试被Base64编码后Cookie中的字段
	FuzzCookieBase64(k, v interface{}) FuzzHTTPRequestIf

	// 测试 multipart 携带字段
	FuzzFormEncoded(k, v interface{}) FuzzHTTPRequestIf

	// 测试上传文件的文件名
	FuzzUploadFileName(k, v interface{}) FuzzHTTPRequestIf

	// 测试上传文件的文件内容
	FuzzUploadFile(k, v interface{}, content []byte) FuzzHTTPRequestIf

	// 测试文件上传内容
	FuzzUploadKVPair(k, v interface{}) FuzzHTTPRequestIf

	// CookieJsonPath
	FuzzCookieJsonPath(any, string, any) FuzzHTTPRequestIf
	FuzzCookieBase64JsonPath(any, string, any) FuzzHTTPRequestIf

	// 测试被 Base64 编码后的 Get Post 参数
	FuzzGetBase64JsonPath(any, string, any) FuzzHTTPRequestIf
	FuzzPostBase64JsonPath(any, string, any) FuzzHTTPRequestIf

	Results() ([]*http.Request, error)

	Exec(...HttpPoolConfigOption) (chan *_httpResult, error)

	Show() FuzzHTTPRequestIf

	ExecFirst(...HttpPoolConfigOption) (*_httpResult, error)

	FirstFuzzHTTPRequest() *FuzzHTTPRequest
	FirstHTTPRequestBytes() []byte

	GetFirstFuzzHTTPRequest() (*FuzzHTTPRequest, error)
}

func rebuildHTTPRequest(req *http.Request, contentLength int64) (*http.Request, error) {
	raw, err := utils.DumpHTTPRequest(req, true)
	if err != nil {
		return nil, utils.Errorf("parse request to bytes failed: %s", err)
	}
	reqCopied, err := lowhttp.ParseBytesToHttpRequest(raw)
	if err != nil {
		return nil, utils.Errorf("restore bytes to request failed: %s", err)
	}
	if contentLength > 0 {
		reqCopied.ContentLength = contentLength
		reqCopied.Header.Set("Content-Length", fmt.Sprint(contentLength))
	}
	return reqCopied, nil
}

func (f *FuzzHTTPRequest) getBody() ([]byte, error) {
	req, err := lowhttp.ParseBytesToHttpRequest(f.originRequest)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(req.Body)
}

func (f *FuzzHTTPRequest) IsEmptyBody() bool {
	body, _ := f.getBody()
	return len(body) == 0
}

func (f *FuzzHTTPRequest) IsBodyJsonEncoded() bool {
	var i interface{} = nil

	body, _ := f.getBody()
	if body == nil {
		return false
	}
	err := json.Unmarshal(bytes.TrimSpace(body), &i)
	if err != nil {
		return false
	}

	return i != nil
}

func (f *FuzzHTTPRequest) IsBodyUrlEncoded() bool {
	body, _ := f.getBody()
	if body == nil {
		return false
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return false
	}
	_ = vals
	return false
}

func (f *FuzzHTTPRequest) IsBodyFormEncoded() bool {
	return lowhttp.IsMultipartFormDataRequest(f.GetBytes())
}

type buildFuzzHTTPRequestConfig struct {
	IsHttps   bool
	Source    string
	RuntimeId string
	Proxy     string
}

type BuildFuzzHTTPRequestOption func(config *buildFuzzHTTPRequestConfig)

func OptProxy(i string) BuildFuzzHTTPRequestOption {
	return func(config *buildFuzzHTTPRequestConfig) {
		config.Proxy = i
	}
}

func OptHTTPS(i bool) BuildFuzzHTTPRequestOption {
	return func(config *buildFuzzHTTPRequestConfig) {
		config.IsHttps = i
	}
}

func OptRuntimeId(r string) BuildFuzzHTTPRequestOption {
	return func(config *buildFuzzHTTPRequestConfig) {
		config.RuntimeId = r
	}
}

func OptSource(i string) BuildFuzzHTTPRequestOption {
	return func(config *buildFuzzHTTPRequestConfig) {
		config.Source = i
	}
}

func RawRequestsToFuzzHTTPRequests(targets []string, onError func(string, error), opts ...BuildFuzzHTTPRequestOption) (*FuzzHTTPRequestBatch, error) {
	var freqs []FuzzHTTPRequestIf
	var firstReq *FuzzHTTPRequest
	for _, req := range targets {
		f, err := NewFuzzHTTPRequest(req, opts...)
		if err != nil {
			log.Errorf("build request failed: %s", err)
			if onError != nil {
				onError(req, err)
			}
			continue
		}
		if firstReq == nil {
			firstReq = f
		}
		freqs = append(freqs, f)
	}
	if len(freqs) <= 0 {
		return nil, utils.Errorf("fuzz http requests EMPTY!")
	}
	batch := &FuzzHTTPRequestBatch{
		fallback:         nil,
		nextFuzzRequests: freqs,
		originRequest:    firstReq,
	}
	return batch, nil
}

func UrlsToHTTPRequests(target ...interface{}) (*FuzzHTTPRequestBatch, error) {
	var reqs []*http.Request
	for _, urlBase := range InterfaceToFuzzResults(target) {
		for _, u := range utils.ParseStringToUrlsWith3W(urlBase) {
			_req, err := http.NewRequest("GET", u, nil)
			if err != nil {
				log.Error(err)
				continue
			}
			reqs = append(reqs, _req)
		}
	}

	var reqIf []FuzzHTTPRequestIf
	var firstReq *FuzzHTTPRequest
	for _, req := range reqs {
		var isHttps = false
		if req.URL.Scheme == "https" {
			isHttps = true
		}
		fuzzReq, err := NewFuzzHTTPRequest(req, OptHTTPS(isHttps))
		if err != nil {
			log.Errorf("build fuzz http request failed: %s", err)
			continue
		}

		if firstReq == nil {
			firstReq = fuzzReq
		}
		reqIf = append(reqIf, fuzzReq)

	}

	if len(reqIf) <= 0 {
		return nil, utils.Errorf("fuzz http requests EMPTY!")
	}
	batch := &FuzzHTTPRequestBatch{
		fallback:         nil,
		nextFuzzRequests: reqIf,
		originRequest:    firstReq,
	}
	return batch, nil
}

func _fixHttpsPorts(r *http.Request) {
	if utils.ToLowerAndStrip(r.URL.Scheme) == "https" {
		var host string
		if r.Host != "" {
			host = r.Host
		}

		if host == "" {
			host = r.Header.Get("Host")
		}

		if host == "" {
			host = r.URL.Host
		}

		if host != "" && r.URL.Host == "" {
			r.URL.Host = host
		}

		host, port, err := utils.ParseStringToHostPort(r.URL.String())
		if err != nil {
			return
		}
		r.Host = utils.HostPort(host, port)
		r.Header.Set("Host", r.Host)
		r.URL.Host = r.Host
	}
}

func getPostJsonFuzzParams(jsonPathPrefix string, params map[string]interface{}, origin *FuzzHTTPRequest) []*FuzzHTTPRequestParam {
	var fuzzParams []*FuzzHTTPRequestParam
	for key, value := range params {
		var prefix string
		if jsonPathPrefix == "" {
			prefix = key
		} else {
			prefix = fmt.Sprintf("%s.%s", jsonPathPrefix, key)
		}

		if !strVisible(key) {
			continue
		}
		switch ret := value.(type) {
		case map[string]interface{}:
			fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
				typePosition:     posPostJson,
				param:            key,
				paramOriginValue: value,
				origin:           origin,
				jsonPath:         prefix,
			})
			fuzzParams = append(fuzzParams, getPostJsonFuzzParams(prefix, ret, origin)...)
		default:
			fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
				typePosition:     posPostJson,
				param:            key,
				paramOriginValue: value,
				origin:           origin,
				jsonPath:         prefix,
			})
		}
	}
	return fuzzParams
}

func NewMustFuzzHTTPRequest(i interface{}, opts ...BuildFuzzHTTPRequestOption) *FuzzHTTPRequest {
	req, err := NewFuzzHTTPRequest(i, opts...)
	if err != nil {
		log.Errorf("create fuzzRequest failed: %s", err)
	}
	return req
}

func NewFuzzHTTPRequest(i interface{}, opts ...BuildFuzzHTTPRequestOption) (*FuzzHTTPRequest, error) {
	var originHttpRequest []byte
	switch ret := i.(type) {
	case []byte:
		_, err := lowhttp.ParseBytesToHttpRequest(ret)
		if err != nil {
			return nil, utils.Errorf("parse bytes to http.Request failed: %s", err)
		}
		originHttpRequest = ret
	case string:
		_, err := lowhttp.ParseStringToHttpRequest(ret)
		if err != nil {
			return nil, utils.Errorf("parse string to http.Request failed: %s", err)
		}
		originHttpRequest = []byte(ret)
	case http.Request:
		r := &ret
		_fixHttpsPorts(r)
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			return nil, utils.Errorf("dump request out failed: %s", err)
		}
		originHttpRequest = raw
	case *http.Request:
		_fixHttpsPorts(ret)
		raw, err := utils.DumpHTTPRequest(ret, true)
		if err != nil {
			return nil, utils.Errorf("dump request out failed: %s", err)
		}
		originHttpRequest = raw
	case *yakhttp.YakHttpRequest:
		return NewFuzzHTTPRequest(ret.Request, opts...)
	default:
		return nil, utils.Errorf("unsupported type[%v] to FuzzHTTPRequest", reflect.TypeOf(i))
	}

	config := &buildFuzzHTTPRequestConfig{}
	for _, opt := range opts {
		opt(config)
	}

	var req = &FuzzHTTPRequest{}
	req.originRequest = originHttpRequest
	req.isHttps = config.IsHttps
	req.source = config.Source
	req.runtimeId = config.RuntimeId
	req.proxy = config.Proxy
	req.Opts = opts

	return req, nil
}

func (f *FuzzHTTPRequest) GetOriginHTTPRequest() (*http.Request, error) {
	if f._originRequestInstance != nil {
		return f._originRequestInstance, nil
	}
	req, err := lowhttp.ParseBytesToHttpRequest(f.originRequest)
	if err != nil {
		return nil, utils.Errorf("init fuzz origin request failed: %s", err)
	}
	f._originRequestInstance = req
	return req, nil
}

func (f *FuzzHTTPRequest) GetGetQueryParams() []*FuzzHTTPRequestParam {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil
	}

	var params []*FuzzHTTPRequestParam
	for key, param := range req.URL.Query() {
		if !strVisible(key) {
			continue
		}

		if val, ok := utils.IsJSON(param[0]); ok {
			for _, j := range jsonpath.RecursiveDeepJsonPath(val) {
				params = append(params, &FuzzHTTPRequestParam{
					typePosition:     posGetQueryJson,
					param:            key,
					paramOriginValue: param,
					jsonPath:         j,
					origin:           f,
				})
			}
		}

		if val, ok := isBase64(param[0]); ok && govalidator.IsPrintableASCII(val) {
			if val, ok := utils.IsJSON(val); ok {
				for _, j := range jsonpath.RecursiveDeepJsonPath(val) {
					params = append(params, &FuzzHTTPRequestParam{
						typePosition:     posGetQueryBase64Json,
						param:            key,
						paramOriginValue: param,
						jsonPath:         j,
						origin:           f,
					})
				}
			} else {
				params = append(params, &FuzzHTTPRequestParam{
					typePosition:     posGetQueryBase64,
					param:            key,
					paramOriginValue: param,
					origin:           f,
				})
			}
		}

		param := &FuzzHTTPRequestParam{
			typePosition:     posGetQuery,
			param:            key,
			paramOriginValue: param,
			origin:           f,
		}
		params = append(params, param)
	}
	return params
}

func httpRequestReadBody(r *http.Request) []byte {
	if r.Body == nil {
		return nil
	}
	var buf bytes.Buffer
	_, err := io.Copy(&buf, r.Body)
	r.Body = ioutil.NopCloser(&buf)
	if err != nil {
		return nil
	}
	return buf.Bytes()
}

func (f *FuzzHTTPRequest) GetPostJsonParams() []*FuzzHTTPRequestParam {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil
	}
	bodyRaw := httpRequestReadBody(req)

	var params any
	err = json.Unmarshal(bytes.TrimSpace(bodyRaw), &params)
	if err != nil {
		return nil
	}

	var fuzzParams []*FuzzHTTPRequestParam
	for _, jsonPath := range jsonpath.RecursiveDeepJsonPath(params) {
		lastPointIndex := strings.LastIndex(jsonPath, ".")
		fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
			typePosition:     posPostJson,
			param:            jsonPath[lastPointIndex+1:],
			paramOriginValue: string(bytes.TrimSpace(bodyRaw)),
			jsonPath:         jsonPath,
			origin:           f,
		})
	}
	//param := &FuzzHTTPRequestParam{
	//	typePosition:     posBody,
	//	param:            "[JSON]",
	//	paramOriginValue: string(bytes.TrimSpace(bodyRaw)),
	//	origin:           f,
	//}
	//fuzzParams = append(fuzzParams, param)
	return fuzzParams
}

func (f *FuzzHTTPRequest) GetPostParams() []*FuzzHTTPRequestParam {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil
	}

	queryRaw := httpRequestReadBody(req)
	r, err := url.ParseQuery(string(queryRaw))
	if err != nil {
		return nil
	}

	var params []*FuzzHTTPRequestParam
	for key, param := range r {
		if !strVisible(key) {
			continue
		}

		if len(param) <= 0 {
			continue
		}

		if val, ok := utils.IsJSON(param[0]); ok {
			for _, j := range jsonpath.RecursiveDeepJsonPath(val) {
				params = append(params, &FuzzHTTPRequestParam{
					typePosition:     posPostQueryJson,
					param:            key,
					paramOriginValue: param,
					jsonPath:         j,
					origin:           f,
				})
			}
		}

		if val, ok := isBase64(param[0]); ok && govalidator.IsPrintableASCII(val) {
			if val, ok := utils.IsJSON(val); ok {
				for _, j := range jsonpath.RecursiveDeepJsonPath(val) {
					params = append(params, &FuzzHTTPRequestParam{
						typePosition:     posPostQueryBase64Json,
						param:            key,
						paramOriginValue: param,
						jsonPath:         j,
						origin:           f,
					})
				}
			} else {
				params = append(params, &FuzzHTTPRequestParam{
					typePosition:     posPostQueryBase64,
					param:            key,
					paramOriginValue: param,
					origin:           f,
				})
			}
		}

		param := &FuzzHTTPRequestParam{
			typePosition:     posPostQuery,
			param:            key,
			paramOriginValue: param,
			origin:           f,
		}
		params = append(params, param)
	}
	return params
}

func (f *FuzzHTTPRequest) GetCookieParams() []*FuzzHTTPRequestParam {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil
	}

	var params []*FuzzHTTPRequestParam
	for _, k := range req.Cookies() {
		if shouldIgnoreCookie(k.Name) {
			continue
		}

		if val, ok := utils.IsJSON(k.Value); ok {
			for _, j := range jsonpath.RecursiveDeepJsonPath(val) {
				params = append(params, &FuzzHTTPRequestParam{
					typePosition:     posCookieJson,
					param:            k.Name,
					paramOriginValue: []string{k.Value},
					jsonPath:         j,
					origin:           f,
				})
			}
		}

		if val, ok := isBase64(k.Value); ok && govalidator.IsPrintableASCII(val) {
			if val, ok := utils.IsJSON(val); ok {
				for _, j := range jsonpath.RecursiveDeepJsonPath(val) {
					params = append(params, &FuzzHTTPRequestParam{
						typePosition:     posCookieBase64Json,
						param:            k.Name,
						paramOriginValue: []string{k.Value},
						jsonPath:         j,
						origin:           f,
					})
				}
			} else {
				params = append(params, &FuzzHTTPRequestParam{
					typePosition:     posCookieBase64,
					param:            k.Name,
					paramOriginValue: []string{k.Value},
					origin:           f,
				})
			}
		}

		params = append(params, &FuzzHTTPRequestParam{
			typePosition:     posCookie,
			param:            k.Name,
			paramOriginValue: []string{k.Value},
			origin:           f,
		})
	}
	return params
}

func (f *FuzzHTTPRequest) GetPathAppendParams() []*FuzzHTTPRequestParam {
	return []*FuzzHTTPRequestParam{{
		typePosition:     posPathAppend,
		paramOriginValue: f.GetPath(),
		origin:           f,
	}}
}

func (f *FuzzHTTPRequest) GetPathRawParams() []*FuzzHTTPRequestParam {
	return []*FuzzHTTPRequestParam{{
		typePosition:     posPath,
		paramOriginValue: f.GetPath(),
		origin:           f,
	}}
}

func (f *FuzzHTTPRequest) GetPathBlockParams() []*FuzzHTTPRequestParam {
	return []*FuzzHTTPRequestParam{{
		typePosition:     posPathBlock,
		paramOriginValue: f.GetPath(),
		origin:           f,
	}}
}

func (f *FuzzHTTPRequest) GetPathParams() []*FuzzHTTPRequestParam {
	var params []*FuzzHTTPRequestParam
	params = append(params, &FuzzHTTPRequestParam{
		typePosition:     posPath,
		paramOriginValue: f.GetPath(),
		origin:           f,
	})
	params = append(params, &FuzzHTTPRequestParam{
		typePosition:     posPathAppend,
		paramOriginValue: f.GetPath(),
		origin:           f,
	})
	params = append(params, &FuzzHTTPRequestParam{
		typePosition:     posPathBlock,
		paramOriginValue: f.GetPath(),
		origin:           f,
	})
	return params
}

func (f *FuzzHTTPRequest) GetCommonParams() []*FuzzHTTPRequestParam {
	postParams := f.GetPostJsonParams()
	if len(postParams) <= 0 {
		postParams = f.GetPostParams()
	}
	ret := append(f.GetGetQueryParams(), postParams...)
	ret = append(ret, f.GetCookieParams()...)

	return ret
}

func (f *FuzzHTTPRequest) GetAllParams() []*FuzzHTTPRequestParam {
	var params []*FuzzHTTPRequestParam
	params = append(params, f.GetGetQueryParams()...)
	if ret := f.GetPostParams(); len(ret) <= 0 {
		ret = f.GetPostJsonParams()
		params = append(params, ret...)
	} else {
		params = append(params, ret...)
	}
	params = append(params, f.GetCookieParams()...)
	params = append(params, f.GetHeaderParams()...)
	params = append(params, f.GetPathParams()...)
	params = append(params, &FuzzHTTPRequestParam{
		typePosition:     posMethod,
		paramOriginValue: f.GetMethod(),
	})
	params = append(params, &FuzzHTTPRequestParam{
		typePosition:     posBody,
		paramOriginValue: f.GetBody(),
	})
	f.IsBodyFormEncoded()
	return params
}

func (f *FuzzHTTPRequest) GetHeaderParams() []*FuzzHTTPRequestParam {
	var keys = f.GetHeaderKeys()
	var params = make([]*FuzzHTTPRequestParam, len(keys))
	for i, k := range keys {
		value := f.GetHeader(k)
		params[i] = &FuzzHTTPRequestParam{
			typePosition:     posHeader,
			param:            k,
			paramOriginValue: value,
			origin:           f,
		}
	}
	return params
}

func (f *FuzzHTTPRequest) GetHeaderParamByName(k string) *FuzzHTTPRequestParam {
	return &FuzzHTTPRequestParam{
		typePosition:     posHeader,
		param:            k,
		paramOriginValue: "",
		origin:           f,
	}
}

func (f *FuzzHTTPRequest) ParamsHash() (string, error) {
	var commonHashElement []string
	params := f.GetCommonParams()
	for _, param := range params {
		commonHashElement = append(commonHashElement, fmt.Sprintf("position:[%v]-name:[%v]", param.Name(), param.Position()))
	}

	if commonHashElement == nil {
		return "", utils.Errorf("no params no test hash")
	}
	sort.Strings(commonHashElement)
	return codec.Sha256(strings.Join(commonHashElement, "|")), nil
}

func (f *FuzzHTTPRequest) Exec(opts ...HttpPoolConfigOption) (chan *_httpResult, error) {
	var originOpts = make([]HttpPoolConfigOption, 4, len(opts)+4)
	originOpts[0] = WithPoolOpt_Https(f.isHttps)
	originOpts[1] = WithPoolOpt_Source(f.source)
	originOpts[2] = WithPoolOpt_RuntimeId(f.runtimeId)
	originOpts[3] = WithPoolOpt_Proxy(f.proxy)
	originOpts = append(originOpts, opts...)
	return _httpPool(f, originOpts...)
}

func (f *FuzzHTTPRequestBatch) Exec(opts ...HttpPoolConfigOption) (chan *_httpResult, error) {
	req := f.GetOriginRequest()
	if req == nil {
		return _httpPool(f, opts...)
	}

	var originOpts []HttpPoolConfigOption
	originOpts = append(originOpts,
		WithPoolOpt_Https(req.isHttps), WithPoolOpt_Source(req.source),
		WithPoolOpt_RuntimeId(req.runtimeId), WithPoolOpt_Proxy(req.proxy),
	)
	return _httpPool(f, append(originOpts, opts...)...)
}

func (f *FuzzHTTPRequest) FirstHTTPRequestBytes() []byte {
	return f.GetBytes()[:]
}

func (f *FuzzHTTPRequestBatch) FirstHTTPRequestBytes() []byte {
	results, err := f.Results()
	if err != nil {
		log.Errorf("cannot get request bytes: %s", err)
	}
	if len(results) > 0 {
		req, _ := utils.HttpDumpWithBody(results[0], true)
		return req
	}
	return nil
}

func (f *FuzzHTTPRequest) FirstFuzzHTTPRequest() *FuzzHTTPRequest {
	return f
}

func (f *FuzzHTTPRequestBatch) FirstFuzzHTTPRequest() *FuzzHTTPRequest {
	results, err := f.Results()
	if err != nil {
		log.Errorf("cannot get request bytes: %s", err)
	}
	if len(results) > 0 {
		req, _ := utils.HttpDumpWithBody(results[0], true)
		return NewMustFuzzHTTPRequest(req)
	}
	return nil
}
