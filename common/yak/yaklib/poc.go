package yaklib

import (
	"context"
	"net/http"
	"net/http/httputil"
	"reflect"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"

	"github.com/pkg/errors"

	"github.com/davecgh/go-spew/spew"
)

const (
	defaultWaitTime    = time.Duration(100) * time.Millisecond
	defaultMaxWaitTime = time.Duration(2000) * time.Millisecond
)

type _pocConfig struct {
	Host                 string
	Port                 int
	ForceHttps           bool
	ForceHttp2           bool
	Timeout              time.Duration
	RetryTimes           int
	RetryInStatusCode    []int
	RetryNotInStatusCode []int
	RetryWaitTime        time.Duration
	RetryMaxWaitTime     time.Duration
	RedirectTimes        int
	NoRedirect           bool
	Proxy                []string
	FuzzParams           map[string][]string
	NoFixContentLength   bool
	JsRedirect           bool
	RedirectHandler      func(bool, []byte, []byte) bool
	Session              interface{} // session的标识符，可以用任意对象
	SaveHTTPFlow         bool
	Source               string

	// packetHandler
	PacketHandler []func([]byte) []byte

	// websocket opt
	// 标注是否开启 Websocket 连接？
	Websocket bool

	// 这是用来出来 Websocket 数据的
	// 参数一为数据的 bytes
	// 参数二为取消函数，调用将会强制断开 websocket
	WebsocketHandler func(i []byte, cancel func())
	// 获取 Websocket 客户端的手段，如果连接成功，Websocket 客户端在这里
	// 可以直接 c.WriteText 即可写入数据
	WebsocketClientHandler func(c *lowhttp.WebsocketClient)

	FromPlugin string
	RuntimeId  string
}

func (c *_pocConfig) ToLowhttpOptions() []lowhttp.LowhttpOpt {
	var opts []lowhttp.LowhttpOpt
	if c.Host != "" {
		opts = append(opts, lowhttp.WithHost(c.Host))
	}
	if c.Port != 0 {
		opts = append(opts, lowhttp.WithPort(c.Port))
	}
	opts = append(opts, lowhttp.WithHttps(c.ForceHttps))
	opts = append(opts, lowhttp.WithHttp2(c.ForceHttp2))
	if c.Timeout > 0 {
		opts = append(opts, lowhttp.WithTimeout(c.Timeout))
	}
	if c.RetryTimes > 0 {
		opts = append(opts, lowhttp.WithRetryTimes(c.RetryTimes))
	}
	if len(c.RetryInStatusCode) > 0 {
		opts = append(opts, lowhttp.WithRetryInStatusCode(c.RetryInStatusCode))
	}
	if len(c.RetryNotInStatusCode) > 0 {
		opts = append(opts, lowhttp.WithRetryNotInStatusCode(c.RetryNotInStatusCode))
	}
	if c.RetryWaitTime > 0 {
		opts = append(opts, lowhttp.WithRetryWaitTime(c.RetryWaitTime))
	}
	if c.RetryMaxWaitTime > 0 {
		opts = append(opts, lowhttp.WithRetryMaxWaitTime(c.RetryMaxWaitTime))
	}
	if c.RedirectTimes > 0 {
		opts = append(opts, lowhttp.WithRedirectTimes(c.RedirectTimes))
	}
	if c.NoRedirect {
		opts = append(opts, lowhttp.WithRedirectTimes(0))
	}

	if c.Proxy != nil {
		opts = append(opts, lowhttp.WithProxy(c.Proxy...))
	}
	if c.FuzzParams != nil {
		log.Warnf("fuzz params is not nil, but not support now")
	}

	if c.NoFixContentLength {
		opts = append(opts, lowhttp.WithNoFixContentLength(c.NoFixContentLength))
	}
	if c.JsRedirect {
		opts = append(opts, lowhttp.WithJsRedirect(c.JsRedirect))
	}
	if c.RedirectHandler != nil {
		opts = append(opts, lowhttp.WithRedirectHandler(c.RedirectHandler))
	}
	if c.Session != nil {
		opts = append(opts, lowhttp.WithSession(c.Session))
	}
	if c.SaveHTTPFlow {
		opts = append(opts, lowhttp.WithSaveHTTPFlow(c.SaveHTTPFlow))
	}
	if c.Source != "" {
		opts = append(opts, lowhttp.WithSource(c.Source))
	}
	return opts
}

func NewDefaultPoCConfig() *_pocConfig {
	config := &_pocConfig{
		Host:                   "",
		Port:                   0,
		ForceHttps:             false,
		ForceHttp2:             false,
		Timeout:                15 * time.Second,
		RetryTimes:             0,
		RetryInStatusCode:      []int{},
		RetryNotInStatusCode:   []int{},
		RetryWaitTime:          defaultWaitTime,
		RetryMaxWaitTime:       defaultMaxWaitTime,
		RedirectTimes:          5,
		NoRedirect:             false,
		Proxy:                  nil,
		FuzzParams:             nil,
		NoFixContentLength:     false,
		JsRedirect:             false,
		RedirectHandler:        nil,
		Session:                nil,
		SaveHTTPFlow:           consts.GetDefaultSaveHTTPFlowFromEnv(),
		Source:                 "",
		Websocket:              false,
		WebsocketHandler:       nil,
		WebsocketClientHandler: nil,
		PacketHandler:          make([]func([]byte) []byte, 0),
	}
	return config
}

type PocConfig func(c *_pocConfig)

func _pocOptWithParams(i interface{}) PocConfig {
	return func(c *_pocConfig) {
		c.FuzzParams = utils.InterfaceToMap(i)
	}
}

func _pocOptWithRedirectHandler(i func(isHttps bool, req, rsp []byte) bool) PocConfig {
	return func(c *_pocConfig) {
		c.RedirectHandler = i
	}
}

func _pocOptWithRetryTimes(t int) PocConfig {
	return func(c *_pocConfig) {
		c.RetryTimes = t
	}
}

func _pocOptWithRetryInStausCode(codes ...int) PocConfig {
	return func(c *_pocConfig) {
		c.RetryInStatusCode = codes
	}
}

func _pocOptWithRetryNotInStausCode(codes ...int) PocConfig {
	return func(c *_pocConfig) {
		c.RetryNotInStatusCode = codes
	}
}

func _pocOptWithRetryWaitTime(t int) PocConfig {
	return func(c *_pocConfig) {
		c.RetryWaitTime = time.Duration(t) * time.Second
	}
}

func _pocOptWithRetryMaxWaitTime(t int) PocConfig {
	return func(c *_pocConfig) {
		c.RetryMaxWaitTime = time.Duration(t) * time.Second
	}
}

func _pocOptWithRedirectTimes(t int) PocConfig {
	return func(c *_pocConfig) {
		c.RedirectTimes = t
	}
}

func _pocOptWithNoFixContentLength(b bool) PocConfig {
	return func(c *_pocConfig) {
		c.NoFixContentLength = b
	}
}

func _pocOptWithNoRedirect(b bool) PocConfig {
	return func(c *_pocConfig) {
		c.NoRedirect = b
	}
}

func _pocOptWithProxy(proxies ...string) PocConfig {
	return func(c *_pocConfig) {
		c.Proxy = proxies
	}
}

func _pocOptWithForceHTTPS(isHttps bool) PocConfig {
	return func(c *_pocConfig) {
		c.ForceHttps = isHttps
	}
}

func _pocOptWithForceHTTP2(isHttp2 bool) PocConfig {
	return func(c *_pocConfig) {
		c.ForceHttp2 = isHttp2
	}
}

func _pocOptWithTimeout(f float64) PocConfig {
	return func(c *_pocConfig) {
		c.Timeout = utils.FloatSecondDuration(f)
	}
}

func _pocOptWithHost(h string) PocConfig {
	return func(c *_pocConfig) {
		c.Host = h
	}
}

var PoCOptWithSource = _pocOptWIthSource
var PoCOptWithRuntimeId = _pocOptWithRuntimeId
var PoCOptWithFromPlugin = _pocOptWithFromPlugin

func _pocOptWithRuntimeId(r string) PocConfig {
	return func(c *_pocConfig) {
		c.RuntimeId = r
	}
}

func _pocOptWithFromPlugin(b string) PocConfig {
	return func(c *_pocConfig) {
		c.FromPlugin = b
	}
}

func _pocOptWebsocket(w bool) PocConfig {
	return func(c *_pocConfig) {
		c.Websocket = w
	}
}

func _pocOptWebsocketHandler(w func(i []byte, cancel func())) PocConfig {
	return func(c *_pocConfig) {
		c.WebsocketHandler = w
	}
}

func _pocOptWebsocketClientHandler(w func(c *lowhttp.WebsocketClient)) PocConfig {
	return func(c *_pocConfig) {
		c.WebsocketClientHandler = w
	}
}

func _pocOptWithPort(port int) PocConfig {
	return func(c *_pocConfig) {
		c.Port = port
	}
}

func _pocOptWithJSRedirect(b bool) PocConfig {
	return func(c *_pocConfig) {
		c.JsRedirect = b
	}
}

func _pocOptWithSession(i interface{}) PocConfig {
	return func(c *_pocConfig) {
		c.Session = i
	}
}

func _pocOptWithSave(i bool) PocConfig {
	return func(c *_pocConfig) {
		c.SaveHTTPFlow = i
	}
}

var PoCOptWithSaveHTTPFlow = _pocOptWithSave

func _pocOptWIthSource(i string) PocConfig {
	return func(c *_pocConfig) {
		c.Source = i
	}
}

func _pocOptReplaceHttpPacketFirstLine(firstLine string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketFirstLine(packet, firstLine)
		},
		)
	}
}

func _pocOptReplaceHttpPacketHeader(key, value string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketHeader(packet, key, value)
		},
		)
	}
}

func _pocOptReplaceHttpPacketCookie(key, value string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketCookie(packet, key, value)
		},
		)
	}
}

func _pocOptReplaceHttpPacketBody(body []byte, chunk bool) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketBody(packet, body, chunk)
		},
		)
	}
}

func _pocOptReplaceHttpPacketPath(path string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketPath(packet, path)
		},
		)
	}
}

func _pocOptReplaceHttpPacketQueryParam(key, value string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketQueryParam(packet, key, value)
		},
		)
	}
}

func _pocOptReplaceHttpPacketPostParam(key, value string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketPostParam(packet, key, value)
		},
		)
	}
}

func _pocOptAppendHeader(key, value string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketHeader(packet, key, value)
		},
		)
	}
}

func _pocOptAppendCookie(key, value string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketCookie(packet, key, value)
		},
		)
	}
}

func _pocOptAppendQueryParam(key, value string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketQueryParam(packet, key, value)
		},
		)
	}
}

func _pocOptAppendPostParam(key, value string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketPostParam(packet, key, value)
		},
		)
	}
}

func _pocOptAppendHttpPacketPath(path string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketPath(packet, path)
		},
		)
	}
}

func _pocOptDeleteHeader(key string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.DeleteHTTPPacketHeader(packet, key)
		},
		)
	}
}

func _pocOptDeleteCookie(key string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.DeleteHTTPPacketCookie(packet, key)
		},
		)
	}
}

func _pocOptDeleteQueryParam(key string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.DeleteHTTPPacketQueryParam(packet, key)
		},
		)
	}
}

func _pocOptDeletePostParam(key string) PocConfig {
	return func(c *_pocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.DeleteHTTPPacketPostParam(packet, key)
		},
		)
	}
}

func fixPacketByConfig(packet []byte, config *_pocConfig) []byte {
	for _, fixFunc := range config.PacketHandler {
		packet = fixFunc(packet)
	}
	return packet
}

func handleUrlAndConfig(urlStr string, opts ...PocConfig) (*_pocConfig, error) {
	// poc 模块收 proxy 影响
	proxy := _cliStringSlice("proxy")
	config := NewDefaultPoCConfig()
	config.Proxy = proxy
	for _, opt := range opts {
		opt(config)
	}

	if len(config.Proxy) <= 0 && utils.GetProxyFromEnv() != "" {
		config.Proxy = append(config.Proxy, utils.GetProxyFromEnv())
	}

	host, port, err := utils.ParseStringToHostPort(urlStr)
	if err != nil {
		return config, utils.Errorf("parse url failed: %s", err)
	}

	if port == 443 {
		config.ForceHttps = true
	}

	if config.Host == "" {
		config.Host = host
	}

	if config.Port == 0 {
		config.Port = port
	}

	if config.NoRedirect {
		config.RedirectTimes = 0
	}

	if config.RetryTimes < 0 {
		config.RetryTimes = 0
	}
	return config, nil
}

func handleRawPacketAndConfig(i interface{}, opts ...PocConfig) ([]byte, *_pocConfig, error) {
	var packet []byte
	switch ret := i.(type) {
	case string:
		packet = []byte(ret)
	case []byte:
		packet = ret
	case http.Request:
		r := &ret
		lowhttp.FixRequestHostAndPort(r)
		raw, err := httputil.DumpRequest(r, true)
		if err != nil {
			return nil, nil, utils.Errorf("dump request out failed: %s", err)
		}
		packet = raw
	case *http.Request:
		lowhttp.FixRequestHostAndPort(ret)
		raw, err := httputil.DumpRequest(ret, true)
		if err != nil {
			return nil, nil, utils.Errorf("dump request out failed: %s", err)
		}
		packet = raw
	case *yakhttp.YakHttpRequest:
		raw, err := httputil.DumpRequest(ret.Request, true)
		if err != nil {
			return nil, nil, utils.Errorf("dump request out failed: %s", err)
		}
		packet = raw
	default:
		return nil, nil, utils.Errorf("cannot support: %s", reflect.TypeOf(i))
	}

	// poc 模块收 proxy 影响
	proxy := _cliStringSlice("proxy")
	config := NewDefaultPoCConfig()
	config.Proxy = proxy
	for _, opt := range opts {
		opt(config)
	}

	if len(config.Proxy) <= 0 && utils.GetProxyFromEnv() != "" {
		config.Proxy = append(config.Proxy, utils.GetProxyFromEnv())
	}

	// 最先应该修复数据包
	if config.FuzzParams != nil && len(config.FuzzParams) > 0 {
		packets, err := mutate.QuickMutate(string(packet), consts.GetGormProfileDatabase(), mutate.MutateWithExtraParams(config.FuzzParams))
		if err != nil {
			return nil, config, utils.Errorf("fuzz parameters failed: %v\n\nParams: \n%v", err, spew.Sdump(config.FuzzParams))
		}
		if len(packets) <= 0 {
			return nil, config, utils.Error("fuzzed packets empty!")
		}

		packet = []byte(packets[0])
	}

	u, err := lowhttp.ExtractURLFromHTTPRequestRaw(packet, config.ForceHttps)
	if err != nil {
		return nil, config, utils.Errorf("extract url failed: %s", err)
	}

	host, port, err := utils.ParseStringToHostPort(u.String())
	if err != nil {
		return nil, config, utils.Errorf("parse url failed: %s", err)
	}

	if port == 443 {
		config.ForceHttps = true
	}

	if config.Host == "" {
		config.Host = host
	}

	if config.Port == 0 {
		config.Port = port
	}

	if config.NoRedirect {
		config.RedirectTimes = 0
	}

	if config.RetryTimes < 0 {
		config.RetryTimes = 0
	}

	packet = fixPacketByConfig(packet, config)
	return packet, config, nil
}

func pochttp(packet []byte, config *_pocConfig) (*lowhttp.LowhttpResponse, error) {
	if config.Websocket {
		if config.Timeout <= 0 {
			config.Timeout = 15 * time.Second
		}
		wsCtx, cancel := context.WithTimeout(context.Background(), config.Timeout)
		defer cancel()

		c, err := lowhttp.NewWebsocketClient(
			packet,
			lowhttp.WithWebsocketTLS(config.ForceHttps),
			lowhttp.WithWebsocketProxy(strings.Join(config.Proxy, ",")),
			lowhttp.WithWebsocketWithContext(wsCtx),
			lowhttp.WithWebsocketFromServerHandler(func(bytes []byte) {
				if config.WebsocketHandler != nil {
					config.WebsocketHandler(bytes, cancel)
				} else {
					spew.Dump(bytes)
				}
			}),
			lowhttp.WithWebsocketHost(config.Host),
			lowhttp.WithWebsocketPort(config.Port),
		)
		c.StartFromServer()
		if config.WebsocketClientHandler != nil {
			config.WebsocketClientHandler(c)
			c.Wait()
		}
		if err != nil {
			return nil, errors.Wrap(err, "websocket handshake failed")
		}
		return &lowhttp.LowhttpResponse{
			RawPacket: c.Response,
		}, nil
	}

	response, err := lowhttp.HTTP(
		lowhttp.WithHttps(config.ForceHttps),
		lowhttp.WithHost(config.Host),
		lowhttp.WithPort(config.Port),
		lowhttp.WithPacketBytes(packet),
		lowhttp.WithTimeout(config.Timeout),
		lowhttp.WithRetryTimes(config.RetryTimes),
		lowhttp.WithRetryInStatusCode(config.RetryInStatusCode),
		lowhttp.WithRetryNotInStatusCode(config.RetryNotInStatusCode),
		lowhttp.WithRetryWaitTime(config.RetryWaitTime),
		lowhttp.WithRetryMaxWaitTime(config.RetryMaxWaitTime),
		lowhttp.WithRedirectTimes(config.RedirectTimes),
		lowhttp.WithJsRedirect(config.JsRedirect),
		lowhttp.WithSession(config.Session),
		lowhttp.WithRedirectHandler(func(isHttps bool, req []byte, rsp []byte) bool {
			if config.RedirectHandler == nil {
				return true
			}
			return config.RedirectHandler(isHttps, req, rsp)
		}),
		lowhttp.WithNoFixContentLength(config.NoFixContentLength),
		lowhttp.WithHttp2(config.ForceHttp2),
		lowhttp.WithProxy(config.Proxy...),
		lowhttp.WithSaveHTTPFlow(config.SaveHTTPFlow),
		lowhttp.WithSource(config.Source),
		lowhttp.WithRuntimeId(config.RuntimeId),
		lowhttp.WithFromPlugin(config.FromPlugin),
	)
	return response, err
}

func pocHTTPEx(i interface{}, opts ...PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error) {
	packet, config, err := handleRawPacketAndConfig(i, opts...)
	if err != nil {
		return nil, nil, err
	}
	response, err := pochttp(packet, config)
	if err != nil {
		return nil, nil, err
	}
	request, err := lowhttp.ParseBytesToHttpRequest(packet)
	if err != nil {
		return nil, nil, err
	}
	return response, request, nil
}

func pocHTTP(i interface{}, opts ...PocConfig) ([]byte, []byte, error) {
	packet, config, err := handleRawPacketAndConfig(i, opts...)
	if err != nil {
		return nil, nil, err
	}
	response, err := pochttp(packet, config)
	return response.RawPacket, lowhttp.FixHTTPPacketCRLF(packet, config.NoFixContentLength), err
}

func do(method string, urlStr string, opts ...PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error) {
	config, err := handleUrlAndConfig(urlStr, opts...)
	if err != nil {
		return nil, nil, err
	}

	packet := lowhttp.UrlToRequestPacket(method, urlStr, nil, config.ForceHttps)
	packet = fixPacketByConfig(packet, config)

	response, err := pochttp(packet, config)
	if err != nil {
		return nil, nil, err
	}
	request, err := lowhttp.ParseBytesToHttpRequest(packet)
	if err != nil {
		return nil, nil, err
	}
	return response, request, nil
}

func get(urlStr string, opts ...PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error) {
	return do("GET", urlStr, opts...)
}

func post(urlStr string, opts ...PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error) {
	return do("POST", urlStr, opts...)
}

func head(urlStr string, opts ...PocConfig) (*lowhttp.LowhttpResponse, *http.Request, error) {
	return do("HEAD", urlStr, opts...)
}

var PoCExports = map[string]interface{}{
	"HTTP":   pocHTTP,
	"HTTPEx": pocHTTPEx,
	"Get":    get,
	"Post":   post,
	"Head":   head,
	"Do":     do,
	// websocket，可以直接复用 HTTP 参数
	"Websocket": func(raw interface{}, opts ...PocConfig) ([]byte, []byte, error) {
		opts = append(opts, _pocOptWebsocket(true))
		return pocHTTP(raw, opts...)
	},

	// options
	"host":                 _pocOptWithHost,
	"port":                 _pocOptWithPort,
	"retryTimes":           _pocOptWithRetryTimes,
	"retryInStatusCode":    _pocOptWithRetryInStausCode,
	"retryNotInStatusCode": _pocOptWithRetryNotInStausCode,
	"redirectTimes":        _pocOptWithRedirectTimes,
	"noRedirect":           _pocOptWithNoRedirect,
	"jsRedirect":           _pocOptWithJSRedirect,
	"redirectHandler":      _pocOptWithRedirectHandler,
	"https":                _pocOptWithForceHTTPS,
	"http2":                _pocOptWithForceHTTP2,
	"params":               _pocOptWithParams,
	"proxy":                _pocOptWithProxy,
	"timeout":              _pocOptWithTimeout,
	"noFixContentLength":   _pocOptWithNoFixContentLength,
	"session":              _pocOptWithSession,
	"save":                 _pocOptWithSave,
	"source":               _pocOptWIthSource,
	"websocket":            _pocOptWebsocket,
	"websocketFromServer":  _pocOptWebsocketHandler,
	"websocketOnClient":    _pocOptWebsocketClientHandler,
	"replaceFirstLine":     _pocOptReplaceHttpPacketFirstLine,
	"replaceHeader":        _pocOptReplaceHttpPacketHeader,
	"replaceCookie":        _pocOptReplaceHttpPacketCookie,
	"replaceBody":          _pocOptReplaceHttpPacketBody,
	"replaceQueryParam":    _pocOptReplaceHttpPacketQueryParam,
	"replacePostParam":     _pocOptReplaceHttpPacketPostParam,
	"replacePath":          _pocOptReplaceHttpPacketPath,
	"appendHeader":         _pocOptAppendHeader,
	"appendCookie":         _pocOptAppendCookie,
	"appendQueryParam":     _pocOptAppendQueryParam,
	"appendPostParam":      _pocOptAppendPostParam,
	"appendPath":           _pocOptAppendHttpPacketPath,
	"deleteHeader":         _pocOptDeleteHeader,
	"deleteCookie":         _pocOptDeleteCookie,
	"deleteQueryParam":     _pocOptDeleteQueryParam,
	"deletePostParam":      _pocOptDeletePostParam,

	// split
	"Split":          lowhttp.SplitHTTPHeadersAndBodyFromPacket,
	"FixHTTPRequest": lowhttp.FixHTTPRequestOut,
	"FixHTTPResponse": func(r []byte) []byte {
		rsp, _, _ := lowhttp.FixHTTPResponse(r)
		return rsp
	},

	// packet helper
	"ReplaceBody":              lowhttp.ReplaceHTTPPacketBody,
	"FixHTTPPacketCRLF":        lowhttp.FixHTTPPacketCRLF,
	"HTTPPacketForceChunked":   lowhttp.HTTPPacketForceChunked,
	"ParseBytesToHTTPRequest":  lowhttp.ParseBytesToHttpRequest,
	"ParseBytesToHTTPResponse": lowhttp.ParseBytesToHTTPResponse,
	"ParseUrlToHTTPRequestRaw": lowhttp.ParseUrlToHttpRequestRaw,

	"ReplaceHTTPPacketFirstLine":  lowhttp.ReplaceHTTPPacketFirstLine,
	"ReplaceHTTPPacketHeader":     lowhttp.ReplaceHTTPPacketHeader,
	"ReplaceHTTPPacketBody":       lowhttp.ReplaceHTTPPacketBodyFast,
	"ReplaceHTTPPacketCookie":     lowhttp.ReplaceHTTPPacketCookie,
	"ReplaceHTTPPacketQueryParam": lowhttp.ReplaceHTTPPacketQueryParam,
	"ReplaceHTTPPacketPostParam":  lowhttp.ReplaceHTTPPacketPostParam,
	"ReplaceHTTPPacketPath":       lowhttp.ReplaceHTTPPacketPath,
	"AppendHTTPPacketHeader":      lowhttp.AppendHTTPPacketHeader,
	"AppendHTTPPacketCookie":      lowhttp.AppendHTTPPacketCookie,
	"AppendHTTPPacketQueryParam":  lowhttp.AppendHTTPPacketQueryParam,
	"AppendHTTPPacketPostParam":   lowhttp.AppendHTTPPacketPostParam,
	"AppendHTTPPacketPath":        lowhttp.AppendHTTPPacketPath,
	"DeleteHTTPPacketHeader":      lowhttp.DeleteHTTPPacketHeader,
	"DeleteHTTPPacketCookie":      lowhttp.DeleteHTTPPacketCookie,
	"DeleteHTTPPacketQueryParam":  lowhttp.DeleteHTTPPacketQueryParam,
	"DeleteHTTPPacketPostParam":   lowhttp.DeleteHTTPPacketPostParam,

	"GetAllHTTPPacketQueryParams": lowhttp.GetAllHTTPRequestQueryParams,
	"GetAllHTTPPacketPostParams":  lowhttp.GetAllHTTPRequestPostParams,
	"GetHTTPPacketQueryParam":     lowhttp.GetHTTPRequestQueryParam,
	"GetHTTPPacketPostParam":      lowhttp.GetHTTPRequestPostParam,
	"GetHTTPPacketCookieValues":   lowhttp.GetHTTPPacketCookieValues,
	"GetHTTPPacketCookieFirst":    lowhttp.GetHTTPPacketCookieFirst,
	"GetHTTPPacketCookie":         lowhttp.GetHTTPPacketCookie,
	"GetHTTPPacketContentType":    lowhttp.GetHTTPPacketContentType,
	"GetHTTPPacketCookies":        lowhttp.GetHTTPPacketCookies,
	"GetHTTPPacketCookiesFull":    lowhttp.GetHTTPPacketCookiesFull,
	"GetHTTPPacketHeaders":        lowhttp.GetHTTPPacketHeaders,
	"GetHTTPPacketHeadersFull":    lowhttp.GetHTTPPacketHeadersFull,
	"GetHTTPPacketHeader":         lowhttp.GetHTTPPacketHeader,
	"GetStatusCodeFromResponse":   lowhttp.GetStatusCodeFromResponse,

	"CurlToHTTPRequest": func(c string) []byte {
		raw, err := lowhttp.CurlToHTTPRequest(c)
		if err != nil {
			log.Errorf(`CurlToHTTPRequest failed: %s`, err)
		}
		return raw
	},
	"HTTPRequestToCurl": func(https bool, i any) string {
		cmd, err := lowhttp.GetCurlCommand(https, utils.InterfaceToBytes(i))
		if err != nil {
			log.Errorf(`http2curl.GetCurlCommand(req): %v`, err)
			return ""
		}
		return cmd.String()
	},
}
