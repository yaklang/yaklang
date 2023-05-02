package yaklib

import (
	"context"
	"reflect"
	"strings"
	"time"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"

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
}

func newDefaultPoCConfig() *_pocConfig {
	config := &_pocConfig{
		Host:                 "",
		Port:                 0,
		ForceHttps:           false,
		ForceHttp2:           false,
		Timeout:              15 * time.Second,
		RetryTimes:           0,
		RetryInStatusCode:    []int{},
		RetryNotInStatusCode: []int{},
		RetryWaitTime:        defaultWaitTime,
		RetryMaxWaitTime:     defaultMaxWaitTime,
		RedirectTimes:        5,
		NoRedirect:           false,
		Proxy:                nil,
		NoFixContentLength:   false,
		JsRedirect:           false,
		SaveHTTPFlow:         consts.GetDefaultSaveHTTPFlowFromEnv(),
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

func _pocOptWIthSource(i string) PocConfig {
	return func(c *_pocConfig) {
		c.Source = i
	}
}

func pocHTTP(raw interface{}, opts ...PocConfig) ([]byte, []byte, error) {
	var packet []byte
	switch raw.(type) {
	case string:
		packet = []byte(raw.(string))
	case []byte:
		packet = raw.([]byte)
	default:
		return nil, nil, utils.Errorf("poc.HTTP cannot support: %s", reflect.TypeOf(raw))
	}

	// poc 模块收 proxy 影响
	proxy := _cliStringSlice("proxy")
	config := newDefaultPoCConfig()
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
			return nil, nil, utils.Errorf("fuzz.Mutate With Params failed: %v\n\nParams: \n%v", err, spew.Sdump(config.FuzzParams))
		}
		if len(packets) <= 0 {
			return nil, nil, utils.Error("fuzzed packets empty!")
		}

		packet = []byte(packets[0])
	}

	u, err := lowhttp.ExtractURLFromHTTPRequestRaw(packet, config.ForceHttps)
	if err != nil {
		return nil, nil, utils.Errorf("extract url failed: %s", err)
	}

	host, port, err := utils.ParseStringToHostPort(u.String())
	if err != nil {
		return nil, nil, utils.Errorf("parse url failed: %s", err)
	}

	if port == 443 {
		config.ForceHttps = true
	}

	if config.Host != "" {
		host = config.Host
	}

	if config.Port > 0 {
		port = config.Port
	}

	if config.NoRedirect {
		config.RedirectTimes = 0
	}

	if config.RetryTimes < 0 {
		config.RetryTimes = 0
	}

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
			return nil, nil, errors.Wrap(err, "lowhttp.Websocket handshake failed")
		}
		return c.Response, c.Request, nil
	}

	response, err := lowhttp.SendHTTPRequestWithRawPacketWithRedirectWithStateWithOptFullEx(
		lowhttp.WithHttps(config.ForceHttps),
		lowhttp.WithHost(host),
		lowhttp.WithPort(port),
		lowhttp.WithPacket(packet),
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
	)
	return response.RawPacket, lowhttp.FixHTTPPacketCRLF(packet, config.NoFixContentLength), err
}

var PoCExports = map[string]interface{}{
	"HTTP": pocHTTP,

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

	// websocket，可以直接复用 HTTP 参数
	"Websocket": func(raw interface{}, opts ...PocConfig) ([]byte, []byte, error) {
		opts = append(opts, _pocOptWebsocket(true))
		return pocHTTP(raw, opts...)
	},
	"websocket":           _pocOptWebsocket,
	"websocketFromServer": _pocOptWebsocketHandler,
	"websocketOnClient":   _pocOptWebsocketClientHandler,

	// split
	"Split":          lowhttp.SplitHTTPHeadersAndBodyFromPacket,
	"FixHTTPRequest": lowhttp.FixHTTPRequestOut,
	"FixHTTPResponse": func(r []byte) []byte {
		rsp, _, _ := lowhttp.FixHTTPResponse(r)
		return rsp
	},
	"ReplaceBody":              lowhttp.ReplaceHTTPPacketBody,
	"FixHTTPPacketCRLF":        lowhttp.FixHTTPPacketCRLF,
	"HTTPPacketForceChunked":   lowhttp.HTTPPacketForceChunked,
	"ParseBytesToHTTPRequest":  lowhttp.ParseBytesToHttpRequest,
	"ParseBytesToHTTPResponse": lowhttp.ParseBytesToHTTPResponse,
	"ParseUrlToHTTPRequestRaw": lowhttp.ParseUrlToHttpRequestRaw,
}
