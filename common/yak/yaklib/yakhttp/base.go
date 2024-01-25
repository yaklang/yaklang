package yakhttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/http_struct"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"

	"github.com/corpix/uarand"
	"github.com/davecgh/go-spew/spew"
)

// dump 获取指定请求结构体引用或响应结构体引用的原始报文，返回原始报文与错误
// Example:
// ```
// req, err = http.NewRequest("GET", "http://www.yaklang.com", http.timeout(10))
// reqRaw, err = http.dump(req)
// rsp, err = http.Do(req)
// rspRaw, err = http.dump(rsp)
// ```
func dump(i interface{}) ([]byte, error) {
	return dumpWithBody(i, true)
}

// dumphead 获取指定请求结构体引用或响应结构体引用的原始报文头部，返回原始报文头部与错误
// Example:
// ```
// req, err = http.NewRequest("GET", "http://www.yaklang.com", http.timeout(10))
// reqHeadRaw, err = http.dumphead(req)
// rsp, err = http.Do(req)
// rspHeadRaw, err = http.dumphead(rsp)
// ```
func dumphead(i interface{}) ([]byte, error) {
	return dumpWithBody(i, false)
}

func dumpWithBody(i interface{}, body bool) ([]byte, error) {
	if body {
		isReq, raw, err := _dumpWithBody(i, body)
		if err != nil {
			return nil, err
		}
		if isReq {
			return lowhttp.FixHTTPPacketCRLF(raw, false), nil
		} else {
			raw, _, err := lowhttp.FixHTTPResponse(raw)
			if err != nil {
				return nil, err
			}
			return raw, nil
		}
	}
	_, raw, err := _dumpWithBody(i, body)
	return raw, err
}

func _dumpWithBody(i interface{}, body bool) (isReq bool, _ []byte, _ error) {
	if i == nil {
		return false, nil, utils.Error("nil interface for http.dump")
	}

	switch ret := i.(type) {
	case *http.Request:
		raw, err := utils.DumpHTTPRequest(ret, body)
		return true, raw, err
	case http.Request:
		return _dumpWithBody(&ret, body)
	case *http.Response:
		raw, err := utils.DumpHTTPResponse(ret, body)
		return false, raw, err
	case http.Response:
		return _dumpWithBody(&ret, body)
	case http_struct.YakHttpResponse:
		return _dumpWithBody(ret.Response, body)
	case *http_struct.YakHttpResponse:
		if ret == nil {
			return false, nil, utils.Error("nil http_struct.YakHttpResponse for http.dump")
		}
		return _dumpWithBody(ret.Response, body)
	case http_struct.YakHttpRequest:
		return _dumpWithBody(ret.Request, body)
	case *http_struct.YakHttpRequest:
		return _dumpWithBody(ret.Request, body)
	default:
		return false, nil, utils.Errorf("error type for http.dump, Type: [%v]", reflect.TypeOf(i))
	}
}

// show 获取指定请求结构体引用或响应结构体引用的原始报文并输出在标准输出
// Example:
// ```
// req, err = http.NewRequest("GET", "http://www.yaklang.com", http.timeout(10))
// http.show(req)
// rsp, err = http.Do(req)
// http.show(rsp)
// ```
func httpShow(i interface{}) {
	rsp, err := dumpWithBody(i, true)
	if err != nil {
		log.Errorf("show failed: %s", err)
		return
	}
	fmt.Println(string(rsp))
}

// showhead 获取指定请求结构体引用或响应结构体引用的原始报文头部并输出在标准输出
// Example:
// ```
// req, err = http.NewRequest("GET", "http://www.yaklang.com", http.timeout(10))
// http.showhead(req)
// rsp, err = http.Do(req)
// http.showhead(rsp)
// ```
func showhead(i interface{}) {
	rsp, err := dumphead(i)
	if err != nil {
		log.Errorf("show failed: %s", err)
		return
	}
	fmt.Println(string(rsp))
}

// timeout 是一个请求选项参数，用于设置请求超时时间，单位是秒
// Example:
// ```
// rsp, err = http.Get("http://www.yaklang.com", http.timeout(10))
// ```
func timeout(f float64) http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		config.Timeout = f
	}
}

// Raw 根据原始请求报文生成请求结构体引用，返回请求结构体引用与错误
// 注意，此函数只会生成请求结构体引用，不会发起请求
// ! 已弃用，使用 poc.HTTP 或 poc.HTTPEx 代替
// Example:
// ```
// req, err = http.Raw("GET / HTTP/1.1\r\nHost: www.yaklang.com\r\n\r\n")
// ```
func rawRequest(i interface{}) (*http.Request, error) {
	var rawReq string
	switch ret := i.(type) {
	case []byte:
		rawReq = string(ret)
	case string:
		rawReq = ret
	case *http.Request:
		return ret, nil
	case http.Request:
		return &ret, nil
	case *http_struct.YakHttpRequest:
		return ret.Request, nil
	case http_struct.YakHttpRequest:
		return ret.Request, nil
	default:
		return nil, utils.Errorf("not a valid type: %v for req: %v", reflect.TypeOf(i), spew.Sdump(i))
	}

	return lowhttp.ParseStringToHttpRequest(rawReq)
}

// proxy 是一个请求选项参数，用于设置一个或多个请求的代理，请求时会根据顺序找到一个可用的代理使用
// Example:
// ```
// rsp, err = http.Get("http://www.yaklang.com", http.proxy("http://127.0.0.1:7890", "http://127.0.0.1:8083"))
// ```
func WithProxy(values ...string) http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		values = utils.StringArrayFilterEmpty(values)
		if len(values) <= 0 {
			return
		}
		if len(config.Proxies) == 0 {
			config.Proxies = make([]string, 0)
		}
		config.Proxies = append(config.Proxies, values...)
	}
}

// NewRequest 根据指定的 method 和 URL 生成请求结构体引用，返回请求结构体引用与错误，它的第一个参数是 URL ，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如设置超时时间等
// 注意，此函数只会生成请求结构体引用，不会发起请求
// ! 已弃用
// Example:
// ```
// req, err = http.NewRequest("GET", "http://www.yaklang.com", http.timeout(10))
// ```
func NewHttpNewRequest(method, url string, opts ...http_struct.HttpOption) (*http_struct.YakHttpRequest, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	config := http_struct.NewHTTPConfig()
	for _, op := range opts {
		op(config)
	}
	rawReq := &http_struct.YakHttpRequest{Request: req, Config: config}
	return rawReq, nil
}

// GetAllBody 获取响应结构体引用的原始响应报文
// Example:
// ```
// rsp, err = http.Get("http://www.yaklang.com")
// raw = http.GetAllBody(rsp)
// ```
func GetAllBody(raw interface{}) []byte {
	switch r := raw.(type) {
	case *http.Response:
		if r == nil {
			return nil
		}

		if r.Body == nil {
			return nil
		}

		rspRaw, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return nil
		}
		return rspRaw
	case *http_struct.YakHttpResponse:
		return GetAllBody(r.Response)
	default:
		log.Errorf("unsupported GetAllBody for %v", reflect.TypeOf(raw))
		return nil
	}
}

// params 是一个请求选项参数，用于添加/指定 GET 参数，这会将参数进行 URL 编码
// Example:
// ```
// rsp, err = http.Get("http://www.yaklang.com", http.params("a=b"), http.params("c=d"))
// ```
func GetParams(i interface{}) http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		params := utils.InterfaceToString(i)
		values, _ := url.ParseQuery(params)

		for k, v := range values {
			if len(v) > 0 {
				config.GetParams[k] = v[len(v)-1]
			}
		}
	}
}

// postparams 是一个请求选项参数，用于添加/指定 POST 参数，这会将参数进行 URL 编码
// Example:
// ```
// rsp, err = http.Post("http://www.yaklang.com", http.postparams("a=b"), http.postparams("c=d"))
// ```
func PostParams(i interface{}) http_struct.HttpOption {
	return Body(utils.UrlJoinParams("", i))
}

// Do 根据构造好的请求结构体引用发送请求，返回响应结构体引用与错误
// ! 已弃用
// Example:
// ```
// req, err = http.Raw("GET / HTTP/1.1\r\nHost: www.yaklang.com\r\n\r\n")
// rsp, err = http.Do(req)
// ```
func Do(i interface{}) (*http.Response, error) {
	switch ret := i.(type) {
	case *http.Request:
		return Do(&http_struct.YakHttpRequest{Request: ret})
	case http.Request:
		return Do(&http_struct.YakHttpRequest{Request: &ret})
	case http_struct.YakHttpRequest:
		return Do(&ret)
	case *http_struct.YakHttpRequest:
	default:
		return nil, utils.Errorf("not a valid type: %v for req: %v", reflect.TypeOf(i), spew.Sdump(i))
	}
	yakHTTPRequest, _ := i.(*http_struct.YakHttpRequest)

	// 修复 HTTPS
	scheme := strings.ToLower(yakHTTPRequest.URL.Scheme)
	isHttps := false
	if scheme == "https" || scheme == "wss" {
		isHttps = true
	}
	config := yakHTTPRequest.Config
	if config == nil {
		config = http_struct.NewHTTPConfig()
	}
	rawRequest, err := utils.DumpHTTPRequest(yakHTTPRequest.Request, true)
	if err != nil {
		return nil, err
	}

	rsp, _, err := poc.HTTP(rawRequest,
		poc.WithForceHTTPS(isHttps),
		poc.WithTimeout(config.Timeout),
		poc.WithProxy(config.Proxies...),
		poc.WithRedirectHandler(func(isHttps bool, req, rsp []byte) bool {
			if config.Redirector != nil {
				reqInstance, err := lowhttp.ParseBytesToHttpRequest(req)
				if err != nil {
					return config.Redirector(reqInstance, []*http.Request{reqInstance})
				}
			}
			return true
		}),
		poc.WithSession(config.Session),
		poc.WithReplaceAllHttpPacketQueryParams(config.GetParams),
		poc.WithReplaceHttpPacketBody(config.Body, false),
		poc.WithReplaceAllHttpPacketHeaders(config.Headers),
	)
	if err != nil {
		return nil, err
	}
	rspInstance, err := lowhttp.ParseBytesToHTTPResponse(rsp)
	if err != nil {
		return nil, err
	}
	return rspInstance, nil
}

// uarand 返回一个随机的 User-Agent
// Example:
// ```
// ua = http.uarand()
// ```
func _getuarand() string {
	return uarand.GetRandom()
}

// header 是一个请求选项参数，用于添加/指定请求头
// Example:
// ```
// rsp, err = http.Get("http://www.yaklang.com", http.header("AAA", "BBB"))
// ```
func Header(key, value interface{}) http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		config.Headers[utils.InterfaceToString(key)] = utils.InterfaceToString(value)
	}
}

// useragent 是一个请求选项参数，用于指定请求的 User-Agent
// Example:
// ```
// rsp, err = http.Get("http://www.yaklang.com", http.ua("yaklang-http"))
// ```
func UserAgent(value interface{}) http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		config.Headers["UserAgent"] = utils.InterfaceToString(value)
	}
}

// fakeua 是一个请求选项参数，用于随机指定请求的 User-Agent
// Example:
// ```
// rsp, err = http.Get("http://www.yaklang.com", http.fakeua())
// ```
func FakeUserAgent() http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		config.Headers["UserAgent"] = _getuarand()
	}
}

// redirect 是一个请求选项参数，它接收重定向处理函数，用于自定义重定向处理逻辑，返回 true 代表继续重定向，返回 false 代表终止重定向
// 重定向处理函数中第一个参数是当前的请求结构体引用，第二个参数是之前的请求结构体引用
// Example:
// ```
// rsp, err = http.Get("http://pie.dev/redirect/3", http.redirect(func(r, vias) bool { return true })
// ```
func RedirectHandler(c func(r *http.Request, vias []*http.Request) bool) http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		config.Redirector = c
	}
}

// noredirect 是一个请求选项参数，用于禁止重定向
// Example:
// ```
// rsp, err = http.Get("http://pie.dev/redirect/3", http.noredirect())
// ```
func NoRedirect() http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		config.Redirector = func(r *http.Request, vias []*http.Request) bool {
			return false
		}
	}
}

// header 是一个请求选项参数，用于设置 Cookie
// Example:
// ```
// rsp, err = http.Get("http://www.yaklang.com", http.Cookie("a=b; c=d"))
// ```
func Cookie(value interface{}) http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		config.Headers["Cookie"] = utils.InterfaceToString(value)
	}
}

// body 是一个请求选项参数，用于指定 JSON 格式的请求体
// 它会将传入的值进行 JSON 序列化，然后设置序列化后的值为请求体
// Example:
// ```
// rsp, err = http.Post("https://pie.dev/post", http.header("Content-Type", "application/json"), http.json({"a": "b", "c": "d"}))
// ```
func JsonBody(value interface{}) http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		body, err := json.Marshal(value)
		if err != nil {
			log.Errorf("yak http.json cannot marshal json: %v\n  ORIGIN: %v\n", err, string(spew.Sdump(value)))
			return
		}
		config.Body = body
		config.Headers["Content-Type"] = "application/json"
	}
}

// body 是一个请求选项参数，用于指定请求体
// Example:
// ```
// rsp, err = http.Post("https://pie.dev/post", http.body("a=b&c=d"))
// ```
func Body(value interface{}) http_struct.HttpOption {
	return func(config *http_struct.HTTPConfig) {
		var rc *bytes.Buffer
		switch ret := value.(type) {
		case string:
			rc = bytes.NewBufferString(ret)
		case []byte:
			rc = bytes.NewBuffer(ret)
		case io.Reader:
			all, err := ioutil.ReadAll(ret)
			if err != nil {
				return
			}
			rc = bytes.NewBuffer(all)
		default:
			rc = bytes.NewBufferString(fmt.Sprint(ret))
		}
		if rc != nil {
			config.Body = rc.Bytes()
		}
	}
}

// session 是一个请求选项参数，用于根据传入的值指定会话，使用相同的值会使用同一个会话，同一个会话会自动复用 Cookie
// Example:
// ```
// rsp, err = http.Get("http://www.yaklang.com", http.session("request1"))
// ```
func Session(value interface{}) http_struct.HttpOption {
	return func(session *http_struct.HTTPConfig) {
		session.Session = value
	}
}

// Request 根据指定的 URL 发起请求，它的第一个参数是 URL ，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如设置请求体，设置超时时间等
// 返回响应结构体引用与错误
// ! 已弃用，使用 poc.Do 代替
// Example:
// ```
// rsp, err = http.Request("POST","http://pie.dev/post", http.body("a=b&c=d"), http.timeout(10))
// ```
func httpRequest(method, url string, options ...http_struct.HttpOption) (*http_struct.YakHttpResponse, error) {
	config := http_struct.NewHTTPConfig()
	for _, op := range options {
		op(config)
	}

	lowhttpRspInst, _, err := poc.Do(method, url,
		poc.WithTimeout(config.Timeout),
		poc.WithProxy(config.Proxies...),
		poc.WithRedirectHandler(func(isHttps bool, req, rsp []byte) bool {
			if config.Redirector != nil {
				reqInstance, err := lowhttp.ParseBytesToHttpRequest(req)
				if err != nil {
					return config.Redirector(reqInstance, []*http.Request{reqInstance})
				}
			}
			return true
		}),
		poc.WithSession(config.Session),
		poc.WithReplaceAllHttpPacketQueryParams(config.GetParams),
		poc.WithReplaceHttpPacketBody(config.Body, false),
		poc.WithReplaceAllHttpPacketHeaders(config.Headers),
	)
	if err != nil {
		return nil, err
	}
	rspInst, err := lowhttp.ParseBytesToHTTPResponse(lowhttpRspInst.RawPacket)
	if err != nil {
		return nil, err
	}
	return &http_struct.YakHttpResponse{Response: rspInst}, nil
}

// Get 根据指定的 URL 发起 GET 请求，它的第一个参数是 URL ，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如设置超时时间等
// 返回响应结构体引用与错误
// ! 已弃用，使用 poc.Get 代替
// Example:
// ```
// rsp, err = http.Get("http://www.yaklang.com", http.timeout(10))
// ```
func _get(url string, options ...http_struct.HttpOption) (*http_struct.YakHttpResponse, error) {
	return httpRequest("GET", url, options...)
}

// Post 根据指定的 URL 发起 POST 请求，它的第一个参数是 URL ，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如设置请求体，设置超时时间等
// 返回响应结构体引用与错误
// ! 已弃用，使用 poc.Post 代替
// Example:
// ```
// rsp, err = http.Post("http://pie.dev/post", http.body("a=b&c=d"), http.timeout(10))
// ```
func _post(url string, options ...http_struct.HttpOption) (*http_struct.YakHttpResponse, error) {
	return httpRequest("POST", url, options...)
}

var HttpExports = map[string]interface{}{
	// 获取原生 Raw 请求包
	"Raw": rawRequest,

	// 快捷方式
	"Get":     _get,
	"Post":    _post,
	"Request": httpRequest,

	// Do 和 Request 组合发起请求
	"Do":         Do,
	"NewRequest": NewHttpNewRequest,

	// 获取响应内容的 response
	"GetAllBody": GetAllBody,

	// 调试信息
	"dump":     dump,
	"show":     httpShow,
	"dumphead": dumphead,
	"showhead": showhead,

	// ua
	"ua":        UserAgent,
	"useragent": UserAgent,
	"fakeua":    FakeUserAgent,

	// header
	"header": Header,

	// cookie
	"cookie": Cookie,

	// body
	"body": Body,

	// json
	"json": JsonBody,

	// urlencode params 区别于 body，这个会编码
	// params 针对 get 请求
	// data 针对 post 请求
	"params":     GetParams,
	"postparams": PostParams,

	// proxy
	"proxy": WithProxy,

	// timeout
	"timeout": timeout,

	// redirect
	"redirect":   RedirectHandler,
	"noredirect": NoRedirect,

	// session
	"session": Session,

	"uarand": _getuarand,
}
