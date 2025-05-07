package mutate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/antchfx/xmlquery"
	"github.com/asaskevich/govalidator"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/http_struct"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const (
	packetFuzz = iota
	// param.Fuzz(any) 的情况不应该保留上一次 fuzz 的params
	paramsFuzz
)

type FuzzHTTPRequest struct {
	opts                   []BuildFuzzHTTPRequestOption
	isHttps                bool
	source                 string
	runtimeId              string
	noAutoEncode           bool
	friendlyDisplay        bool
	proxy                  string
	originRequest          []byte
	_originRequestInstance *http.Request
	chunked                bool
	ctx                    context.Context
	queryParams            *lowhttp.QueryParams
	position               lowhttp.HttpParamPositionType
	mode                   int
	mutex                  sync.Mutex
	fromPlugin             string
}

func (f *FuzzHTTPRequest) NoAutoEncode() bool {
	if f == nil {
		return false
	}
	return f.noAutoEncode
}

func (f *FuzzHTTPRequest) DisableAutoEncode(b bool) FuzzHTTPRequestIf {
	if f != nil {
		f.noAutoEncode = b
	}
	return f
}

func (f *FuzzHTTPRequest) FriendlyDisplay() FuzzHTTPRequestIf {
	if f != nil {
		f.friendlyDisplay = true
	}
	return f
}

type FuzzHTTPRequestIf interface {
	// Repeat 重复数据包
	// Example:
	// ```
	//
	// ```
	Repeat(i int) FuzzHTTPRequestIf

	// 模糊测试参数时不进行自动编码
	DisableAutoEncode(bool) FuzzHTTPRequestIf

	FriendlyDisplay() FuzzHTTPRequestIf

	// 标注是否进行自动编码
	NoAutoEncode() bool

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

	// 测试 PostXML 中的数据
	FuzzPostXMLParams(k, v interface{}) FuzzHTTPRequestIf

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
	RequestMap(func([]byte)) FuzzHTTPRequestIf

	Exec(...HttpPoolConfigOption) (chan *HttpResult, error)

	Show() FuzzHTTPRequestIf

	ExecFirst(...HttpPoolConfigOption) (*HttpResult, error)

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
	IsHttps         bool
	Source          string
	RuntimeId       string
	NoAutoEncode    bool
	FriendlyDisplay bool
	QueryParams     *lowhttp.QueryParams
	Proxy           string
	Ctx             context.Context
	FromPlugin      string
}

type BuildFuzzHTTPRequestOption func(config *buildFuzzHTTPRequestConfig)

func OptProxy(i string) BuildFuzzHTTPRequestOption {
	return func(config *buildFuzzHTTPRequestConfig) {
		config.Proxy = i
	}
}

func OptDisableAutoEncode(i bool) BuildFuzzHTTPRequestOption {
	return func(config *buildFuzzHTTPRequestConfig) {
		config.NoAutoEncode = i
	}
}

func OptFriendlyDisplay() BuildFuzzHTTPRequestOption {
	return func(config *buildFuzzHTTPRequestConfig) {
		config.FriendlyDisplay = true
	}
}

func OptQueryParams(i *lowhttp.QueryParams) BuildFuzzHTTPRequestOption {
	return func(config *buildFuzzHTTPRequestConfig) {
		config.QueryParams = i
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

func OptFromPlugin(i string) BuildFuzzHTTPRequestOption {
	return func(config *buildFuzzHTTPRequestConfig) {
		config.FromPlugin = i
	}
}

func OptContext(ctx context.Context) BuildFuzzHTTPRequestOption {
	return func(config *buildFuzzHTTPRequestConfig) {
		config.Ctx = ctx
	}
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
		isHttps := false
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
				position: lowhttp.PosPostJson,
				param:    key,
				raw:      value,
				origin:   origin,
				path:     prefix,
			})
			fuzzParams = append(fuzzParams, getPostJsonFuzzParams(prefix, ret, origin)...)
		default:
			fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
				position: lowhttp.PosPostJson,
				param:    key,
				raw:      value,
				origin:   origin,
				path:     prefix,
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
	case *http_struct.YakHttpRequest:
		return NewFuzzHTTPRequest(ret.Request, opts...)
	case *FuzzHTTPRequest:
		opts = ret.MergeOption(opts...)
	default:
		return nil, utils.Errorf("unsupported type[%v] to FuzzHTTPRequest", reflect.TypeOf(i))
	}

	config := &buildFuzzHTTPRequestConfig{}
	for _, opt := range opts {
		opt(config)
	}

	req := &FuzzHTTPRequest{
		originRequest:   originHttpRequest,
		isHttps:         config.IsHttps,
		source:          config.Source,
		runtimeId:       config.RuntimeId,
		proxy:           config.Proxy,
		noAutoEncode:    config.NoAutoEncode,
		friendlyDisplay: config.FriendlyDisplay,
		queryParams:     config.QueryParams,
		ctx:             config.Ctx,
		opts:            opts,
		mode:            packetFuzz,
		fromPlugin:      config.FromPlugin,
	}

	return req, nil
}

func (f *FuzzHTTPRequest) GetCurrentOptions() []BuildFuzzHTTPRequestOption {
	result := make([]BuildFuzzHTTPRequestOption, 0, len(f.opts))
	if f.runtimeId != "" {
		result = append(result, OptRuntimeId(f.runtimeId))
	}
	if f.source != "" {
		result = append(result, OptSource(f.source))
	}
	if f.fromPlugin != "" {
		result = append(result, OptFromPlugin(f.fromPlugin))
	}
	if f.isHttps {
		result = append(result, OptHTTPS(f.isHttps))
	}
	if f.proxy != "" {
		result = append(result, OptProxy(f.proxy))
	}
	if f.noAutoEncode {
		result = append(result, OptDisableAutoEncode(f.noAutoEncode))
	}

	if f.friendlyDisplay {
		result = append(result, OptFriendlyDisplay())
	}

	if f.queryParams != nil {
		result = append(result, OptQueryParams(f.queryParams))
	}

	return result
}

func (f *FuzzHTTPRequest) MergeOption(opts ...BuildFuzzHTTPRequestOption) []BuildFuzzHTTPRequestOption {
	result := make([]BuildFuzzHTTPRequestOption, len(f.opts)+len(opts))
	copy(result, f.opts)
	copy(result[len(f.opts):], opts)
	return result
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

	fuzzParams := make([]*FuzzHTTPRequestParam, 0)
	vals := lowhttp.ParseQueryParams(req.URL.RawQuery)
	filtered := make(map[string]struct{})

	for _, item := range vals.Items {
		key, value, valueRaw := item.Key, item.Value, item.ValueRaw
		if _, ok := filtered[key]; ok {
			// 重复的参数不再处理
			continue
		} else {
			filtered[key] = struct{}{}
		}
		if key == "" {
			key = item.Raw
		}
		if !strVisible(key) {
			continue
		}

		if raw, ok := utils.IsJSON(value); ok {
			fixRaw := strings.TrimSpace(raw)
			call := func(jk, jv gjson.Result, gPath, jPath string) {
				fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
					position:   lowhttp.PosGetQueryJson,
					param:      key,
					raw:        fixRaw,
					paramValue: jv.String(),
					path:       jPath,
					gpath:      gPath,
					origin:     f,
				})
			}
			walk(gjson.ParseBytes([]byte(raw)), "", "$", call)
		}

		if bs64Raw, ok := IsStrictBase64(value); ok && govalidator.IsPrintableASCII(bs64Raw) {
			if raw, ok := utils.IsJSON(bs64Raw); ok {
				fixRaw := strings.TrimSpace(raw)
				call := func(jk, jv gjson.Result, gPath, jPath string) {
					fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
						position:   lowhttp.PosGetQueryBase64Json,
						param:      key,
						param2nd:   jk.String(),
						paramValue: jv.String(),
						raw:        fixRaw,
						path:       jPath,
						gpath:      gPath,
						origin:     f,
					})
				}
				walk(gjson.ParseBytes([]byte(raw)), "", "$", call)

			}
			// 优化显示效果
			fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
				position:   lowhttp.PosGetQueryBase64,
				param:      key,
				paramValue: bs64Raw,
				raw:        valueRaw,
				origin:     f,
			})

		}

		param := &FuzzHTTPRequestParam{
			position:   lowhttp.PosGetQuery,
			param:      key,
			raw:        valueRaw,
			paramValue: value,
			origin:     f,
		}
		fuzzParams = append(fuzzParams, param)
	}
	return fuzzParams
}

func (f *FuzzHTTPRequest) GetPostCommonParams() []*FuzzHTTPRequestParam {
	postParams := f.GetPostJsonParams()
	if len(postParams) <= 0 {
		postParams = f.GetPostXMLParams()
	}
	if len(postParams) <= 0 {
		postParams = f.GetPostParams()
	}
	return postParams
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

// HasSpecialJSONPathChars 检查 JSONPath 字符串是否包含特殊意义的符号。
func HasSpecialJSONPathChars(jsonPath string) bool {
	// 定义一个包含 JSONPath 特殊字符的列表。
	specialChars := []string{"$", "@", ".", "..", "*", "[", "]", "?", "(", ":", ")"}

	// 遍历特殊字符列表，检查它们是否在 JSONPath 字符串中。
	for _, char := range specialChars {
		if strings.Contains(jsonPath, char) {
			return true // 发现特殊字符，返回 true。
		}
	}
	return false
}

func walk(value gjson.Result, gPrefix string, jPrefix string, call func(key, val gjson.Result, gPath, jPath string)) {
	// 遍历当前层级的所有键
	value.ForEach(func(key, val gjson.Result) bool {
		var jPath string
		// json path syntax
		if key.Type == gjson.Number {
			jPath = fmt.Sprintf("%s[%d]", jPrefix, key.Int())
		} else {
			jPath = fmt.Sprintf("%s.%s", jPrefix, key.String())
		}
		if key.Type == gjson.String && HasSpecialJSONPathChars(key.String()) {
			jPath = fmt.Sprintf(`%s["%s"]`, jPrefix, key.String())
		}
		// gjson path syntax
		gPath := key.String()
		if gPrefix != "" {
			curr := key.String()
			if HasSpecialJSONPathChars(key.String()) {
				curr = "\\" + key.String()
			}
			gPath = gPrefix + "." + curr
		}

		call(key, val, gPath, jPath)

		// 如果当前值是对象或数组，递归遍历
		if val.IsObject() || val.IsArray() {
			walk(val, gPath, jPath, call)
		}

		return true
	})
}

func (f *FuzzHTTPRequest) GetPostJsonParams() []*FuzzHTTPRequestParam {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil
	}

	fuzzParams := make([]*FuzzHTTPRequestParam, 0)

	bodyRaw := httpRequestReadBody(req)

	if bodyRaw == nil || len(bodyRaw) == 0 {
		return fuzzParams
	}
	bodyStr := string(bytes.TrimSpace(bodyRaw))
	if _, ok := utils.IsJSON(bodyStr); !ok {
		return fuzzParams
	}
	call := func(key, val gjson.Result, gPath, jPath string) {
		var paramValue interface{}
		if val.IsObject() || val.IsArray() {
			paramValue = val.String()
		} else {
			paramValue = val.Value()
		}

		fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
			position:   lowhttp.PosPostJson,
			param:      key.String(),
			raw:        bodyStr,
			paramValue: paramValue,
			path:       jPath,
			gpath:      gPath,
			origin:     f,
		})
	}
	values := gjson.ParseBytes(bodyRaw)
	// 从根对象开始遍历
	walk(values, "", "$", call)
	return fuzzParams
}

func (f *FuzzHTTPRequest) GetPostXMLParams() []*FuzzHTTPRequestParam {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil
	}
	bodyRaw := httpRequestReadBody(req)

	rootNode, err := xmlquery.Parse(bytes.NewReader(bodyRaw))
	if err != nil {
		return nil
	}

	var fuzzParams []*FuzzHTTPRequestParam
	RecursiveXMLNode(rootNode, func(node *xmlquery.Node) {
		fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
			position: lowhttp.PosPostXML,
			param:    node.Data,
			raw:      node,
			path:     GetXpathFromNode(node),
			origin:   f,
		})
	})
	return fuzzParams
}

func (f *FuzzHTTPRequest) GetPostParams() []*FuzzHTTPRequestParam {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil
	}

	body := httpRequestReadBody(req)
	bodyStr := string(body)
	fuzzParams := make([]*FuzzHTTPRequestParam, 0)

	vals := lowhttp.ParseQueryParams(bodyStr)
	filtered := make(map[string]struct{})

	for _, item := range vals.Items {
		key, value, valueRaw := item.Key, item.Value, item.ValueRaw
		if _, ok := filtered[key]; ok {
			// 重复的参数不再处理
			continue
		} else {
			filtered[key] = struct{}{}
		}
		if key == "" {
			key = item.Raw
		}
		if !strVisible(key) {
			continue
		}

		if raw, ok := utils.IsJSON(value); ok {
			fixRaw := strings.TrimSpace(raw)
			call := func(jk, jv gjson.Result, gPath, jPath string) {
				fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
					position:   lowhttp.PosPostQueryJson,
					param:      key,
					param2nd:   jk.String(),
					paramValue: jv.String(),
					raw:        fixRaw,
					path:       jPath,
					gpath:      gPath,
					origin:     f,
				})
			}
			walk(gjson.Parse(raw), "", "$", call)
		}

		if bs64Raw, ok := IsStrictBase64(value); ok && govalidator.IsPrintableASCII(bs64Raw) {
			if raw, ok := utils.IsJSON(bs64Raw); ok {
				fixRaw := strings.TrimSpace(raw)
				call := func(jk, jv gjson.Result, gPath, jPath string) {
					fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
						position:   lowhttp.PosPostQueryBase64Json,
						param:      key,
						param2nd:   jk.String(),
						paramValue: jv.String(),
						raw:        fixRaw,
						path:       jPath,
						gpath:      gPath,
						origin:     f,
					})
				}
				walk(gjson.Parse(raw), "", "$", call)
			}
			fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
				position:   lowhttp.PosPostQueryBase64,
				param:      key,
				paramValue: bs64Raw,
				raw:        valueRaw,
				origin:     f,
			})

		}

		param := &FuzzHTTPRequestParam{
			position:   lowhttp.PosPostQuery,
			param:      key,
			paramValue: value,
			raw:        valueRaw,
			origin:     f,
		}
		fuzzParams = append(fuzzParams, param)
	}
	return fuzzParams
}

func (f *FuzzHTTPRequest) GetCookieParams() []*FuzzHTTPRequestParam {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil
	}

	fuzzParams := make([]*FuzzHTTPRequestParam, 0)
	for _, k := range req.Cookies() {
		if ShouldIgnoreCookie(k.Name) {
			continue
		}

		if raw, ok := utils.IsJSON(k.Value); ok {
			fixRaw := strings.TrimSpace(raw)
			call := func(jk, jv gjson.Result, gPath, jPath string) {
				fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
					position:   lowhttp.PosCookieJson,
					param:      k.Name,
					param2nd:   jk.String(),
					paramValue: jv.String(),
					raw:        fixRaw,
					path:       jPath,
					gpath:      gPath,
					origin:     f,
				})
			}
			walk(gjson.ParseBytes([]byte(raw)), "", "$", call)
		}

		if bs64Raw, ok := IsStrictBase64(k.Value); ok && govalidator.IsPrintableASCII(bs64Raw) {
			if raw, ok := utils.IsJSON(bs64Raw); ok {
				fixRaw := strings.TrimSpace(raw)
				call := func(jk, jv gjson.Result, gPath, jPath string) {
					fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
						position:   lowhttp.PosCookieBase64Json,
						param:      k.Name,
						param2nd:   jk.String(),
						paramValue: jv.String(),
						raw:        fixRaw,
						path:       jPath,
						gpath:      gPath,
						origin:     f,
					})
				}
				walk(gjson.ParseBytes([]byte(raw)), "", "$", call)

			}
			fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
				position:   lowhttp.PosCookieBase64,
				param:      k.Name,
				paramValue: bs64Raw,
				raw:        k.Value,
				origin:     f,
			})
		}

		fuzzParams = append(fuzzParams, &FuzzHTTPRequestParam{
			position:   lowhttp.PosCookie,
			param:      k.Name,
			paramValue: k.Value,
			raw:        []string{k.Value},
			origin:     f,
		})
	}
	return fuzzParams
}

func (f *FuzzHTTPRequest) GetCookieParamsByName(name string) *FuzzHTTPRequestParam {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil
	}

	for _, k := range req.Cookies() {
		if k.Name == name {
			return &FuzzHTTPRequestParam{
				position: lowhttp.PosCookie,
				param:    k.Name,
				raw:      []string{k.Value},
				origin:   f,
			}
		}
	}
	return nil
}

func (f *FuzzHTTPRequest) GetPathAppendParams() []*FuzzHTTPRequestParam {
	return []*FuzzHTTPRequestParam{{
		position: lowhttp.PosPathAppend,
		raw:      f.GetPath(),
		origin:   f,
	}}
}

func (f *FuzzHTTPRequest) GetPathRawParams() []*FuzzHTTPRequestParam {
	return []*FuzzHTTPRequestParam{{
		position: lowhttp.PosPath,
		raw:      f.GetPath(),
		origin:   f,
	}}
}

func (f *FuzzHTTPRequest) GetPathBlockParams() []*FuzzHTTPRequestParam {
	return []*FuzzHTTPRequestParam{{
		position: lowhttp.PosPathBlock,
		raw:      f.GetPath(),
		origin:   f,
	}}
}

func (f *FuzzHTTPRequest) GetPathParams() []*FuzzHTTPRequestParam {
	var params []*FuzzHTTPRequestParam
	params = append(params, &FuzzHTTPRequestParam{
		position: lowhttp.PosPath,
		raw:      f.GetPath(),
		origin:   f,
	})
	params = append(params, &FuzzHTTPRequestParam{
		position: lowhttp.PosPathAppend,
		raw:      f.GetPath(),
		origin:   f,
	})
	params = append(params, &FuzzHTTPRequestParam{
		position: lowhttp.PosPathBlock,
		raw:      f.GetPath(),
		origin:   f,
	})
	return params
}

func (f *FuzzHTTPRequest) GetCommonParams() []*FuzzHTTPRequestParam {
	var params []*FuzzHTTPRequestParam
	params = append(params, f.GetGetQueryParams()...)
	params = append(params, f.GetPostCommonParams()...)
	params = append(params, f.GetCookieParams()...)
	return params
}

func (f *FuzzHTTPRequest) GetPostCommonParamsByName(name string) *FuzzHTTPRequestParam {
	postParams := f.GetPostCommonParams()
	for _, param := range postParams {
		if param.Name() == name {
			return param
		}
	}
	return nil
}

func (f *FuzzHTTPRequest) GetAllParams() []*FuzzHTTPRequestParam {
	var params []*FuzzHTTPRequestParam
	params = append(params, f.GetGetQueryParams()...)
	params = append(params, f.GetPostCommonParams()...)
	params = append(params, f.GetCookieParams()...)
	params = append(params, f.GetHeaderParams()...)
	params = append(params, f.GetPathParams()...)
	params = append(params, &FuzzHTTPRequestParam{
		position: lowhttp.PosMethod,
		raw:      f.GetMethod(),
		origin:   f,
	})
	params = append(params, &FuzzHTTPRequestParam{
		position: lowhttp.PosBody,
		raw:      f.GetBody(),
		origin:   f,
	})
	return params
}

func (f *FuzzHTTPRequest) GetHeaderParams() []*FuzzHTTPRequestParam {
	keys := f.GetHeaderKeys()
	params := make([]*FuzzHTTPRequestParam, len(keys))
	for i, k := range keys {
		value := f.GetHeader(k)
		params[i] = &FuzzHTTPRequestParam{
			position:   lowhttp.PosHeader,
			param:      k,
			paramValue: value,
			origin:     f,
		}
	}
	return params
}

func (f *FuzzHTTPRequest) GetHeaderParamByName(k string) *FuzzHTTPRequestParam {
	value := f.GetHeader(k)
	return &FuzzHTTPRequestParam{
		position:   lowhttp.PosHeader,
		param:      k,
		paramValue: value,
		origin:     f,
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

func (f *FuzzHTTPRequest) Exec(opts ...HttpPoolConfigOption) (chan *HttpResult, error) {
	originOpts := []HttpPoolConfigOption{
		WithPoolOpt_Https(f.isHttps),
		WithPoolOpt_Source(f.source),
		WithPoolOpt_RuntimeId(f.runtimeId),
		WithPoolOpt_Proxy(f.proxy),
		WithPoolOpt_FromPlugin(f.fromPlugin),
	}
	if f.ctx != nil {
		originOpts = append(originOpts, WithPoolOpt_Context(f.ctx))
	}
	originOpts = append(originOpts, opts...)
	return _httpPool(f, originOpts...)
}

func (f *FuzzHTTPRequestBatch) Exec(opts ...HttpPoolConfigOption) (chan *HttpResult, error) {
	req := f.GetOriginRequest()
	if req == nil {
		return _httpPool(f, opts...)
	}

	var originOpts []HttpPoolConfigOption
	originOpts = append(originOpts,
		WithPoolOpt_Https(req.isHttps), WithPoolOpt_Source(req.source),
		WithPoolOpt_RuntimeId(req.runtimeId), WithPoolOpt_Proxy(strings.Split(req.proxy, ",")...),
		WithPoolOpt_FromPlugin(req.fromPlugin),
	)
	return _httpPool(f, append(originOpts, opts...)...)
}

func (f *FuzzHTTPRequest) FirstHTTPRequestBytes() []byte {
	return f.GetBytes()[:]
}

func RequestMap(f FuzzHTTPRequestIf, h func([]byte)) FuzzHTTPRequestIf {
	results, err := f.Results()
	if err != nil {
		log.Errorf("cannot get request bytes: %s", err)
		return f
	}
	for _, req := range results {
		raw, err := utils.DumpHTTPRequest(req, true)
		if err != nil {
			log.Warnf("RequestMap ... utils.DumpHTTPRequest failed: %s", err)
			continue
		}
		h(raw)
	}
	return f
}

func (f *FuzzHTTPRequest) RequestMap(h func([]byte)) FuzzHTTPRequestIf {
	return RequestMap(f, h)
}

func (f *FuzzHTTPRequestBatch) RequestMap(h func([]byte)) FuzzHTTPRequestIf {
	return RequestMap(f, h)
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
