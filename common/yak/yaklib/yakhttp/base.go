package yakhttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"github.com/yaklang/yaklang/common/crawler"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"

	"github.com/corpix/uarand"
	"github.com/davecgh/go-spew/spew"
)

var getDefaultHTTPClient = utils.NewDefaultHTTPClient

var ClientPool sync.Map

func GetClient(session interface{}) *http.Client {
	var client *http.Client

	if iClient, ok := ClientPool.Load(session); !ok {
		client = getDefaultHTTPClient()
		ClientPool.Store(session, client)
	} else {
		client = iClient.(*http.Client)
	}

	return client
}

func dump(i interface{}) ([]byte, error) {
	return dumpWithBody(i, true)
}

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
	switch ret := i.(type) {
	case *http.Request:
		raw, err := httputil.DumpRequest(ret, body)
		return true, raw, err
	case http.Request:
		return _dumpWithBody(&ret, body)
	case *http.Response:
		raw, err := httputil.DumpResponse(ret, body)
		return false, raw, err
	case http.Response:
		return _dumpWithBody(&ret, body)
	case YakHttpResponse:
		return _dumpWithBody(ret.Response, body)
	case *YakHttpResponse:
		return _dumpWithBody(ret.Response, body)
	case YakHttpRequest:
		return _dumpWithBody(ret.Request, body)
	case *YakHttpRequest:
		return _dumpWithBody(ret.Request, body)
	default:
		return false, nil, utils.Errorf("error type for http.dump, Type: [%v]", reflect.TypeOf(i))
	}
}

func httpShow(i interface{}) {
	rsp, err := dumpWithBody(i, true)
	if err != nil {
		log.Errorf("show failed: %s", err)
		return
	}
	fmt.Println(string(rsp))
}

type YakHttpRequest struct {
	*http.Request

	timeout    time.Duration
	proxies    func(req *http.Request) (*url.URL, error)
	redirector func(i *http.Request, reqs []*http.Request) bool
	session    interface{}
}

type HttpOption func(req *YakHttpRequest)

func yakHttpConfig_Timeout(f float64) HttpOption {
	return func(req *YakHttpRequest) {
		req.timeout = utils.FloatSecondDuration(f)
	}
}

/*
*
http 扩展包
*/
var rawRequest = func(i interface{}) (*http.Request, error) {

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
	case *YakHttpRequest:
		return ret.Request, nil
	case YakHttpRequest:
		return ret.Request, nil
	default:
		return nil, utils.Errorf("not a valid type: %v for req: %v", reflect.TypeOf(i), spew.Sdump(i))
	}

	return lowhttp.ParseStringToHttpRequest(rawReq)
}

func yakHttpConfig_Proxy(values ...string) HttpOption {
	return func(req *YakHttpRequest) {
		values = utils.StringArrayFilterEmpty(values)
		if len(values) <= 0 {
			return
		}
		swticher, err := crawler.RoundRobinProxySwitcher(values...)
		if err != nil {
			log.Errorf("set http proxy[%v] failed: %s", strings.Join(values, ","), err)
			return
		}
		req.proxies = swticher
	}
}

var NewHttpNewRequest = func(method, url string, opts ...HttpOption) (*YakHttpRequest, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	rawReq := &YakHttpRequest{
		Request: req,
	}
	for _, op := range opts {
		op(rawReq)
	}
	return rawReq, nil
}

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
	case *YakHttpResponse:
		return GetAllBody(r.Response)
	default:
		log.Errorf("unsupported GetAllBody for %v", reflect.TypeOf(raw))
		return nil
	}
}

var HttpExports = map[string]interface{}{
	// 获取原生 Raw 请求包
	"Raw": rawRequest,

	// 快捷方式
	"Get": func(url string, opts ...HttpOption) (*YakHttpResponse, error) {
		return httpRequest("GET", url, opts...)
	},
	"Post": func(url string, opts ...HttpOption) (*YakHttpResponse, error) {
		return httpRequest("POST", url, opts...)
	},
	"Request": httpRequest,

	// Do 和 Request 组合发起请求
	"Do":         Do,
	"NewRequest": NewHttpNewRequest,

	// 获取响应内容的 response
	"GetAllBody": GetAllBody,

	//

	// 调试信息
	"dump":     dump,
	"show":     httpShow,
	"dumphead": dumphead,
	"showhead": func(i interface{}) {
		rsp, err := dumphead(i)
		if err != nil {
			log.Errorf("show failed: %s", err)
			return
		}
		fmt.Println(string(rsp))
	},

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
	"proxy": yakHttpConfig_Proxy,

	// timeout
	"timeout": yakHttpConfig_Timeout,

	// redirect
	"redirect":   RedirectHandler,
	"noredirect": NoRedirect,

	// session
	"session": Session,

	"uarand": _getuarand,
}

// GetParams set query params
func GetParams(i interface{}) HttpOption {
	return func(req *YakHttpRequest) {
		req.URL.RawQuery = utils.UrlJoinParams(req.URL.RawQuery, i)
	}
}

// PostParams set post params
func PostParams(i interface{}) HttpOption {
	return Body(utils.UrlJoinParams("", i))
}

func Do(req *YakHttpRequest) (*http.Response, error) {
	if req.proxies != nil && utils.GetProxyFromEnv() != "" {
		yakHttpConfig_Proxy(utils.GetProxyFromEnv())(req)
	}

	var client *http.Client
	if req.session != nil {
		client = GetClient(req.session)
	} else {
		client = getDefaultHTTPClient()
	}

	httpTr := client.Transport.(*http.Transport)
	httpTr.Proxy = req.proxies
	if req.timeout > 0 {
		client.Timeout = req.timeout
	}
	client.Transport = httpTr
	if req.redirector != nil {
		client.CheckRedirect = func(i *http.Request, via []*http.Request) error {
			if !req.redirector(i, via) {
				return utils.Errorf("user aborted...")
			} else {
				return nil
			}
		}
	}
	return client.Do(req.Request)
}

func _getuarand() string {
	return uarand.GetRandom()
}

func Header(key, value interface{}) HttpOption {
	return func(req *YakHttpRequest) {
		req.Header.Set(fmt.Sprint(key), fmt.Sprint(value))
	}
}

func UserAgent(value interface{}) HttpOption {
	return func(req *YakHttpRequest) {
		req.Header.Set("User-Agent", fmt.Sprint(value))
	}
}

func FakeUserAgent() HttpOption {
	return func(req *YakHttpRequest) {
		req.Header.Set("User-Agent", _getuarand())
	}
}

func RedirectHandler(c func(r *http.Request, vias []*http.Request) bool) HttpOption {
	return func(req *YakHttpRequest) {
		req.redirector = c
	}
}

func NoRedirect() HttpOption {
	return func(req *YakHttpRequest) {
		req.redirector = func(r *http.Request, vias []*http.Request) bool {
			return false
		}
	}
}

func Cookie(value interface{}) HttpOption {
	return func(req *YakHttpRequest) {
		req.Header.Set("Cookie", fmt.Sprint(value))
	}
}

func JsonBody(value interface{}) HttpOption {
	return func(req *YakHttpRequest) {
		body, err := json.Marshal(value)
		if err != nil {
			log.Errorf("yak http.json cannot marshal json: %v\n  ORIGIN: %v\n", err, string(spew.Sdump(value)))
			return
		}
		Body(body)(req)
		Header("Content-Type", "application/json")(req)
	}
}

func Body(value interface{}) HttpOption {
	return func(req *YakHttpRequest) {
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
			req.ContentLength = int64(len(rc.Bytes()))
			buf := rc.Bytes()
			req.GetBody = func() (io.ReadCloser, error) {
				r := bytes.NewReader(buf)
				return ioutil.NopCloser(r), nil
			}
			req.Body, _ = req.GetBody()
		}
	}
}

func Session(value interface{}) HttpOption {
	return func(req *YakHttpRequest) {
		req.session = value
	}
}

func httpRequest(method, url string, options ...HttpOption) (*YakHttpResponse, error) {
	reqRaw, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	req := &YakHttpRequest{Request: reqRaw}
	for _, opt := range options {
		opt(req)
	}
	if req.proxies == nil && utils.GetProxyFromEnv() != "" {
		yakHttpConfig_Proxy(utils.GetProxyFromEnv())(req)
	}

	// client复用，实现会话管理
	var client *http.Client
	if req.session != nil {
		client = GetClient(req.session)
	} else {
		client = getDefaultHTTPClient()
	}

	httpTr := client.Transport.(*http.Transport)
	httpTr.Proxy = req.proxies
	if req.timeout > 0 {
		client.Timeout = req.timeout
	}
	client.Transport = httpTr
	rspRaw, err := client.Do(req.Request)
	if err != nil {
		return nil, err
	}
	return &YakHttpResponse{Response: rspRaw}, nil
}

type YakHttpResponse struct {
	*http.Response
}

func (y *YakHttpResponse) Json() interface{} {
	var data = y.Data()
	if data == "" {
		return nil
	}
	var i interface{}
	err := json.Unmarshal([]byte(data), i)
	if err != nil {
		log.Errorf("parse %v to json failed: %s", strconv.Quote(data), err)
		return ""
	}
	return i
}

func (y *YakHttpResponse) Data() string {
	if y.Response == nil {
		log.Error("response empty")
		return ""
	}

	if y.Response.Body == nil {
		return ""
	}

	body, _ := ioutil.ReadAll(y.Response.Body)
	y.Response.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	return string(body)
}

func (y *YakHttpResponse) GetHeader(key string) string {
	return y.Response.Header.Get(key)
}

func (y *YakHttpResponse) Raw() []byte {
	raw, _ := dumpWithBody(y, true)
	return raw
}
