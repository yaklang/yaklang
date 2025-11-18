package poc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/corpix/uarand"
	utls "github.com/refraction-networking/utls"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/cli"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/http_struct"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/pkg/errors"

	"github.com/davecgh/go-spew/spew"
)

var (
	defaultWaitTime    = time.Duration(100) * time.Millisecond
	defaultMaxWaitTime = time.Duration(2000) * time.Millisecond
	defaultTimeout     = time.Duration(15000) * time.Millisecond
)

type PocConfig struct {
	Host                 string
	Port                 int
	ForceHttps           *bool
	ForceHttp2           *bool
	SNI                  *string
	Timeout              *time.Duration
	ConnectTimeout       *time.Duration
	RetryTimes           *int
	RetryInStatusCode    []int
	RetryNotInStatusCode []int
	RetryWaitTime        *time.Duration
	RetryMaxWaitTime     *time.Duration
	RedirectTimes        *int
	NoRedirect           *bool
	Proxy                []string
	FuzzParams           map[string][]string
	NoFixContentLength   *bool
	JsRedirect           *bool
	RedirectHandler      func(bool, []byte, []byte) bool
	Session              interface{} // session的标识符，可以用任意对象
	SaveHTTPFlow         *bool
	SaveHTTPFlowHandler  []func(*lowhttp.LowhttpResponse)
	UseMITMRule          *bool
	SaveHTTPFlowSync     *bool
	Source               string
	Username             *string
	Password             *string
	FixQueryEscape       *bool

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
	// strict mode
	// whether strictly follow RFC 6455
	WebsocketStrictMode bool

	FromPlugin string
	RuntimeId  string

	ConnectionPool bool

	Context context.Context

	DNSServers []string
	DNSNoCache bool

	BodyStreamHandler func([]byte, io.ReadCloser)
	NoBodyBuffer      *bool // 禁用响应体缓冲，用于下载大文件避免内存占用

	ClientHelloSpec *utls.ClientHelloSpec
	RandomJA3       bool

	GmTLS       bool
	GmTLSOnly   bool
	GmTLSPrefer bool

	// random chunked
	EnableRandomChunked bool
	MinChunkedLength    int
	MaxChunkedLength    int
	MinChunkDelay       time.Duration
	MaxChunkDelay       time.Duration
	ChunkedHandler      func(id int, chunkRaw []byte, totalTime time.Duration, chunkSendTime time.Duration)
}

func (c *PocConfig) IsHTTPS() bool {
	return c.ForceHttps != nil && *c.ForceHttps
}

func (c *PocConfig) ToLowhttpOptions() []lowhttp.LowhttpOpt {
	var opts []lowhttp.LowhttpOpt
	if c.Host != "" {
		opts = append(opts, lowhttp.WithHost(c.Host))
	}
	if c.RuntimeId != "" {
		opts = append(opts, lowhttp.WithRuntimeId(c.RuntimeId))
	}
	if c.Port != 0 {
		opts = append(opts, lowhttp.WithPort(c.Port))
	}
	if c.ForceHttps != nil {
		opts = append(opts, lowhttp.WithHttps(*c.ForceHttps))
	}
	if c.ForceHttp2 != nil {
		opts = append(opts, lowhttp.WithHttp2(*c.ForceHttp2))
	}
	if c.SNI != nil {
		opts = append(opts, lowhttp.WithSNI(*c.SNI))
	}
	if c.Timeout != nil {
		opts = append(opts, lowhttp.WithTimeout(*c.Timeout))
	}
	if c.RetryTimes != nil {
		opts = append(opts, lowhttp.WithRetryTimes(*c.RetryTimes))
	}
	if len(c.RetryInStatusCode) > 0 {
		opts = append(opts, lowhttp.WithRetryInStatusCode(c.RetryInStatusCode))
	}
	if len(c.RetryNotInStatusCode) > 0 {
		opts = append(opts, lowhttp.WithRetryNotInStatusCode(c.RetryNotInStatusCode))
	}
	if c.RetryWaitTime != nil {
		opts = append(opts, lowhttp.WithRetryWaitTime(*c.RetryWaitTime))
	}
	if c.RetryMaxWaitTime != nil {
		opts = append(opts, lowhttp.WithRetryMaxWaitTime(*c.RetryMaxWaitTime))
	}
	if c.RedirectTimes != nil {
		opts = append(opts, lowhttp.WithRedirectTimes(*c.RedirectTimes))
	}
	if c.NoRedirect != nil && *c.NoRedirect {
		opts = append(opts, lowhttp.WithRedirectTimes(0))
	}

	if len(c.Proxy) > 0 {
		opts = append(opts, lowhttp.WithProxy(c.Proxy...))
	}

	if c.NoFixContentLength != nil {
		opts = append(opts, lowhttp.WithNoFixContentLength(*c.NoFixContentLength))
	}
	if c.FixQueryEscape != nil {
		opts = append(opts, lowhttp.WithFixQueryEscape(*c.FixQueryEscape))
	}
	if c.JsRedirect != nil {
		opts = append(opts, lowhttp.WithJsRedirect(*c.JsRedirect))
	}
	if c.SaveHTTPFlow != nil {
		opts = append(opts, lowhttp.WithSaveHTTPFlow(*c.SaveHTTPFlow))
	}
	if c.SaveHTTPFlowHandler != nil {
		opts = append(opts, lowhttp.WithSaveHTTPFlowHandler(c.SaveHTTPFlowHandler...))
	}
	if c.UseMITMRule != nil {
		opts = append(opts, lowhttp.WithUseMITMRule(*c.UseMITMRule))
	}
	if c.SaveHTTPFlowSync != nil {
		opts = append(opts, lowhttp.WithSaveHTTPFlowSync(*c.SaveHTTPFlowSync))
	}
	opts = append(opts, lowhttp.WithRedirectHandler(func(isHttps bool, req []byte, rsp []byte) bool {
		if c.RedirectHandler == nil {
			return true
		}
		return c.RedirectHandler(isHttps, req, rsp)
	}))
	if c.Session != nil {
		opts = append(opts, lowhttp.WithSession(c.Session))
	}
	if c.Source != "" {
		opts = append(opts, lowhttp.WithSource(c.Source))
	}
	if c.FromPlugin != "" {
		opts = append(opts, lowhttp.WithFromPlugin(c.FromPlugin))
	}
	opts = append(opts, lowhttp.WithConnPool(c.ConnectionPool))
	if c.ConnectTimeout != nil {
		opts = append(opts, lowhttp.WithConnectTimeout(*c.ConnectTimeout))
	}
	if c.Context != nil {
		opts = append(opts, lowhttp.WithContext(c.Context))
	}
	if len(c.DNSServers) > 0 {
		opts = append(opts, lowhttp.WithDNSServers(c.DNSServers))
	}
	opts = append(opts, lowhttp.WithDNSNoCache(c.DNSNoCache))

	if c.BodyStreamHandler != nil {
		opts = append(opts, lowhttp.WithBodyStreamReaderHandler(c.BodyStreamHandler))
	}
	if c.NoBodyBuffer != nil {
		opts = append(opts, lowhttp.WithNoBodyBuffer(*c.NoBodyBuffer))
	}

	if c.Username != nil {
		opts = append(opts, lowhttp.WithUsername(*c.Username))
	}
	if c.Password != nil {
		opts = append(opts, lowhttp.WithPassword(*c.Password))
	}

	if c.ClientHelloSpec != nil {
		opts = append(opts, lowhttp.WithClientHelloSpec(c.ClientHelloSpec))
	}

	if c.RandomJA3 {
		opts = append(opts, lowhttp.WithRandomJA3FingerPrint(c.RandomJA3))
	}
	if c.GmTLSOnly {
		opts = append(opts, lowhttp.WithGmTLSOnly(c.GmTLSOnly))
	}
	if c.GmTLS {
		opts = append(opts, lowhttp.WithGmTLS(c.GmTLS))
	}
	if c.GmTLSPrefer {
		opts = append(opts, lowhttp.WithGmTLSPrefer(c.GmTLSPrefer))
	}
	if c.EnableRandomChunked {
		opts = append(opts, lowhttp.WithEnableRandomChunked(c.EnableRandomChunked))
		opts = append(opts, lowhttp.WithRandomChunkedLength(c.MinChunkedLength, c.MaxChunkedLength))
		opts = append(opts, lowhttp.WithRandomChunkedDelay(c.MinChunkDelay, c.MaxChunkDelay))
		opts = append(opts, lowhttp.WithRandomChunkedHandler(c.ChunkedHandler))
	}
	return opts
}

func NewDefaultPoCConfig() *PocConfig {
	config := &PocConfig{
		Host:                   "",
		Port:                   0,
		RetryTimes:             nil,
		RetryInStatusCode:      []int{},
		RetryNotInStatusCode:   []int{},
		RedirectTimes:          nil,
		Proxy:                  nil,
		FuzzParams:             nil,
		RedirectHandler:        nil,
		Session:                nil,
		Source:                 "",
		Websocket:              false,
		WebsocketHandler:       nil,
		WebsocketClientHandler: nil,
		PacketHandler:          make([]func([]byte) []byte, 0),
		Context:                nil,
		ConnectionPool:         false,
		DNSServers:             make([]string, 0),
		DNSNoCache:             false,
	}
	return config
}

type PocConfigOption func(c *PocConfig)

// params 是一个请求选项参数，用于在请求时使用传入的值，需要注意的是，它可以很方便地使用 `str.f()`或 f-string 代替
// Example:
// rsp, req, err = poc.HTTP(x`POST /post HTTP/1.1
// Content-Type: application/json
// Host: pie.dev
//
// {"key": "{{params(a)}}"}`, poc.params({"a":"bbb"})) // 实际上发送的POST参数为{"key": "bbb"}
func WithParams(i interface{}) PocConfigOption {
	return func(c *PocConfig) {
		c.FuzzParams = utils.InterfaceToMap(i)
	}
}

// redirectHandler 是一个请求选项参数，用于作为重定向处理函数，如果设置了该选项，则会在重定向时调用该函数，如果该函数返回 true，则会继续重定向，否则不会重定向。其第一个参数为是否使用 https 协议，第二个参数为原始请求报文，第三个参数为原始响应报文
// Example:
// ```
// count = 3
// poc.Get("https://pie.dev/redirect/5", poc.redirectHandler(func(https, req, rsp) {
// count--
// return count >= 0
// })) // 向 pie.edv 发起请求，使用自定义 redirectHandler 函数，使用 count 控制，进行最多3次重定向
// ```
func WithRedirectHandler(i func(isHttps bool, req, rsp []byte) bool) PocConfigOption {
	return func(c *PocConfig) {
		c.RedirectHandler = i
	}
}

// redirect 是一个请求选项参数，用于设置旧风格的 redirectHandler 函数，如果设置了该选项，则会在重定向时调用该函数，如果该函数返回 true，则会继续重定向，否则不会重定向。其第一个参数为当前的请求，第二个参数为既往的多个请求
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.redirect(func(current, vias) {
// return true
// })) // 向 example.com 发起请求，使用自定义 redirectHandler 函数，如果该函数返回 true，则会继续重定向，否则不会重定向
// ```
func WithRedirect(i func(current *http.Request, vias []*http.Request) bool) PocConfigOption {
	return func(c *PocConfig) {
		c.RedirectHandler = func(isHttps bool, req, rsp []byte) bool {
			reqInstance, err := lowhttp.ParseBytesToHttpRequest(req)
			if err != nil {
				return i(reqInstance, []*http.Request{reqInstance})
			}
			return i(reqInstance, []*http.Request{reqInstance})
		}
	}
}

// stream 是一个请求选项参数，可以设置一个回调函数，如果 body 读取了，将会复制一份给这个流，在这个流中处理 body 是不会影响最终结果的，一般用于处理较长的 chunk 数据
func WithBodyStreamReaderHandler(i func(r []byte, closer io.ReadCloser)) PocConfigOption {
	return func(c *PocConfig) {
		c.BodyStreamHandler = i
	}
}

// noBodyBuffer 是一个请求选项参数，用于指定是否禁用响应体缓冲，设置为 true 时可以避免大文件下载时的内存占用
// 通常与 WithBodyStreamReaderHandler 配合使用，用于流式处理大文件
// Example:
// ```
// poc.Get("https://example.com/large-file.zip",
//
//	poc.noBodyBuffer(true),
//	poc.bodyStreamHandler(func(header, body) {
//	    // 流式处理响应体，不会将整个文件读入内存
//	    io.Copy(file, body)
//	}))
//
// ```
func WithNoBodyBuffer(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.NoBodyBuffer = &b
	}
}

// retryTimes 是一个请求选项参数，用于指定请求失败时的重试次数，需要搭配 retryInStatusCode 或 retryNotInStatusCode 使用，来设置在什么响应码的情况下重试
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.retryTimes(5), poc.retryInStatusCode(500, 502)) // 向 example.com 发起请求，如果响应状态码500或502则进行重试，最多进行5次重试
// ```
func WithRetryTimes(t int) PocConfigOption {
	return func(c *PocConfig) {
		c.RetryTimes = &t
	}
}

// retryInStatusCode 是一个请求选项参数，用于指定在某些响应状态码的情况下重试，需要搭配 retryTimes 使用
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.retryTimes(5), poc.retryInStatusCode(500, 502)) // 向 example.com 发起请求，如果响应状态码500或502则进行重试，最多进行5次重试
// ```
func WithRetryInStatusCode(codes ...int) PocConfigOption {
	return func(c *PocConfig) {
		c.RetryInStatusCode = codes
	}
}

// retryNotInStatusCode 是一个请求选项参数，用于指定非某些响应状态码的情况下重试，需要搭配 retryTimes 使用
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.retryTimes(5), poc.retryNotInStatusCode(200)) // 向 example.com 发起请求，如果响应状态码不等于200则进行重试，最多进行5次重试
// ```
func WithRetryNotInStausCode(codes ...int) PocConfigOption {
	return func(c *PocConfig) {
		c.RetryNotInStatusCode = codes
	}
}

// retryWaitTime 是一个请求选项参数，用于指定重试时最小等待时间，需要搭配 retryTimes 使用，默认为0.1秒
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.retryTimes(5), poc.retryNotInStatusCode(200), poc.retryWaitTime(0.1)) // 向 example.com 发起请求，如果响应状态码不等于200则进行重试，最多进行5次重试，重试时最小等待0.1秒
// ```
func WithRetryWaitTime(f float64) PocConfigOption {
	return func(c *PocConfig) {
		t := utils.FloatSecondDuration(f)
		c.RetryWaitTime = &t
	}
}

// retryMaxWaitTime 是一个请求选项参数，用于指定重试时最大等待时间，需要搭配 retryTimes 使用，默认为2秒
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.retryTimes(5), poc.retryNotInStatusCode(200), poc.retryWaitTime(2)) // 向 example.com 发起请求，如果响应状态码不等于200则进行重试，最多进行5次重试，重试时最多等待2秒
// ```
func WithRetryMaxWaitTime(f float64) PocConfigOption {
	return func(c *PocConfig) {
		t := utils.FloatSecondDuration(f)
		c.RetryMaxWaitTime = &t
	}
}

// redirectTimes 是一个请求选项参数，用于指定最大重定向次数，默认为5次
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.redirectTimes(5)) // 向 example.com 发起请求，如果响应重定向到其他链接，则会自动跟踪重定向最多5次
// ```
func WithRedirectTimes(t int) PocConfigOption {
	return func(c *PocConfig) {
		c.RedirectTimes = &t
	}
}

// noFixContentLength 是一个请求选项参数，用于指定是否修复响应报文中的 Content-Length 字段，默认为 false 即会自动修复Content-Length字段
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.noFixContentLength()) // 向 example.com 发起请求，如果响应报文中的Content-Length字段不正确或不存在	也不会自动修复
// ```
func WithNoFixContentLength(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.NoFixContentLength = &b
	}
}

// noRedirect 是一个请求选项参数，用于指定是否跟踪重定向，默认为 false 即会自动跟踪重定向
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.noRedirect()) // 向 example.com 发起请求，如果响应重定向到其他链接也不会自动跟踪重定向
// ```
func WithNoRedirect(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.NoRedirect = &b
	}
}

// proxy 是一个请求选项参数，用于指定请求使用的代理，可以指定多个代理，默认会使用系统代理
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.proxy("http://127.0.0.1:7890")) // 向 example.com 发起请求，使用 http://127.0.0.1:7890 代理
// ```
func WithProxy(proxies ...string) PocConfigOption {
	return func(c *PocConfig) {
		data := utils.StringArrayFilterEmpty(proxies)
		if len(data) > 0 {
			c.Proxy = proxies
		}
	}
}

// https 是一个请求选项参数，用于指定是否使用 https 协议，默认为 false 即使用 http 协议
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.https(true)) // 向 example.com 发起请求，使用 https 协议
// ```
func WithForceHTTPS(isHttps bool) PocConfigOption {
	return func(c *PocConfig) {
		c.ForceHttps = &isHttps
	}
}

// http2 是一个请求选项参数，用于指定是否使用 http2 协议，默认为 false 即使用http1协议
// Example:
// ```
// poc.Get("https://www.example.com", poc.http2(true), poc.https(true)) // 向 www.example.com 发起请求，使用 http2 协议
// ```
func WithForceHTTP2(isHttp2 bool) PocConfigOption {
	return func(c *PocConfig) {
		c.ForceHttp2 = &isHttp2
	}
}

// sni 是一个请求选项参数，用于指定使用 tls(https) 协议时的 服务器名称指示(SNI)
// Example:
// ```
// poc.Get("https://www.example.com", poc.sni("google.com"))
// ```
func WithSNI(sni string) PocConfigOption {
	return func(c *PocConfig) {
		c.SNI = &sni
	}
}

// username 是一个请求选项参数，用于指定认证时的用户名
// Example:
// ```
// poc.Get("https://www.example.com", poc.username("admin"), poc.password("admin"))
// ```
func WithUsername(username string) PocConfigOption {
	return func(c *PocConfig) {
		c.Username = &username
	}
}

// password 是一个请求选项参数，用于指定认证时的密码
// Example:
// ```
// poc.Get("https://www.example.com", poc.username("admin"), poc.password("admin"))
// ```
func WithPassword(password string) PocConfigOption {
	return func(c *PocConfig) {
		c.Password = &password
	}
}

// timeout 是一个请求选项参数，用于指定读取超时时间，默认为15秒
// Example:
// ```
// poc.Get("https://www.example.com", poc.timeout(15)) // 向 www.baidu.com 发起请求，读取超时时间为15秒
// ```
func WithTimeout(f float64) PocConfigOption {
	return func(c *PocConfig) {
		t := utils.FloatSecondDuration(f)
		c.Timeout = &t
	}
}

// connectTimeout 是一个请求选项参数，用于指定连接超时时间，默认为15秒
// Example:
// ```
// poc.Get("https://www.example.com", poc.timeout(15)) // 向 www.baidu.com 发起请求，读取超时时间为15秒
// ```
func WithConnectTimeout(f float64) PocConfigOption {
	return func(c *PocConfig) {
		t := utils.FloatSecondDuration(f)
		c.ConnectTimeout = &t
	}
}

// host 是一个请求选项参数，用于指定实际请求的 host，如果没有设置该请求选项，则会依据原始请求报文中的Host字段来确定实际请求的host
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.host("yaklang.com")) // 实际上请求 yaklang.com
// ```
func WithHost(h string) PocConfigOption {
	return func(c *PocConfig) {
		c.Host = h
	}
}

func WithRuntimeId(r string) PocConfigOption {
	return func(c *PocConfig) {
		c.RuntimeId = r
	}
}

func WithFromPlugin(b string) PocConfigOption {
	return func(c *PocConfig) {
		c.FromPlugin = b
	}
}

// json 是一个请求选项参数，用于指定请求的 body 为 json 格式，需要传入一个任意类型的参数，会自动转换为 json 格式
// Example:
// ```
// poc.Post("https://www.example.com", poc.json({"a": "b"})) // 向 www.example.com 发起请求，请求的 body 为 {"a": "b"} 并自动设置 Content-Type 为 application/json
// ```
func WithJSON(i any) PocConfigOption {
	raw, err := json.Marshal(i)
	if err != nil {
		log.Errorf("poc WithJson Failed: %v", err)
		return func(c *PocConfig) {

		}
	}
	return func(c *PocConfig) {
		WithReplaceHttpPacketHeader("Content-Type", "application/json")(c)
		WithReplaceHttpPacketBody(raw, false)(c)
	}
}

// body 是一个请求选项参数，用于指定请求的 body，需要传入一个任意类型的参数，如果不是 string 或者 bytes 会抛出日志并忽略。
// Example:
// ```
// poc.Post("https://www.example.com", poc.body("a=b")) // 向 www.example.com 发起请求，请求的 body 为 a=b
// ```
func WithBody(i any) PocConfigOption {
	return func(c *PocConfig) {
		switch i.(type) {
		case string, []byte:
			WithReplaceHttpPacketBody(utils.InterfaceToBytes(i), false)(c)
		default:
			log.Warnf("poc WithBody Failed: %#v", i)
		}
	}
}

// query 是一个请求选项参数，用于指定请求的 query 参数，需要传入一个任意类型的参数，会自动转换为 query 参数
// 如果输入的是 map 类型，则会自动转换为 query 参数，例如：{"a": "b"} 转换为 a=b
// 如果输入的是其他，会把字符串结果直接作为 query 设置
// Example:
// ```
// poc.Get("https://www.example.com", poc.query({"a": "b"})) // 向 www.example.com 发起请求，请求的 query 为 a=b, url 为 https://www.example.com?a=b
// poc.Get("https://www.example.com", poc.query("a=b")) // 向 www.example.com 发起请求，请求的 query 为 a=b, url 为 https://www.example.com?a=b
// poc.Get("https://www.example.com", poc.query("abc")) // 向 www.example.com 发起请求，请求的 query 为 a=b, url 为 https://www.example.com?abc
// ```
func WithQuery(i any) PocConfigOption {
	return func(c *PocConfig) {
		if utils.IsMap(i) {
			for k, raw := range utils.InterfaceToMap(i) {
				if len(raw) > 0 {
					for _, v := range raw {
						WithAppendQueryParam(k, utils.InterfaceToString(v))(c)
					}
				} else {
					WithAppendQueryParam(k, "")(c)
				}
			}
		} else {
			WithReplaceHttpPacketQueryParamRaw(utils.InterfaceToString(i))(c)
		}
	}
}

// postData 是一个请求选项参数，用于指定请求的 body 为 post 数据，需要传入一个任意类型的参数，会自动转换为 post 数据
// 输入是原始字符串，不会修改 Content-Type
// Example:
// ```
// poc.Post("https://www.example.com", poc.postData("a=b")) // 向 www.example.com 发起请求，请求的 body 为 a=b
// ```
func WithPostData(i string) PocConfigOption {
	return func(c *PocConfig) {
		WithReplaceHttpPacketBody([]byte(i), false)(c)
	}
}

// postParams 是一个请求选项参数，用于指定请求的 body 为 post 数据，需要传入一个任意类型的参数，会自动转换为 post 数据
// 输入是 map 类型，会自动转换为 post 数据，同时会自动设置 Content-Type 为 application/x-www-form-urlencoded
// Example:
// ```
// poc.Post("https://www.example.com", poc.postParams({"a": "b"})) // 向 www.example.com 发起请求，请求的 body 为 a=b 并自动设置 Content-Type 为 application/x-www-form-urlencoded
// ```
func WithPostParams(i any) PocConfigOption {
	return func(c *PocConfig) {
		if utils.IsMap(i) {
			WithReplaceHttpPacketHeader("Content-Type", "application/x-www-form-urlencoded")(c)
			for k, v := range utils.InterfaceToMap(i) {
				if len(v) == 0 {
					WithReplaceHttpPacketPostParam(k, "")(c)
				} else {
					for _, vv := range v {
						WithAppendPostParam(k, utils.InterfaceToString(vv))(c)
					}
				}
			}
		} else {
			WithPostData(utils.InterfaceToString(i))(c)
		}
	}
}

func WithRandomJA3(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.RandomJA3 = b
	}
}

// not export
func WithClientHelloSpec(spec *utls.ClientHelloSpec) PocConfigOption {
	return func(c *PocConfig) {
		c.ClientHelloSpec = spec
	}
}

// websocket 是一个请求选项参数，用于允许将链接升级为 websocket，此时发送的请求应该为 websocket 握手请求
// Example:
// ```
// rsp, req, err = poc.HTTP(`GET / HTTP/1.1
// Connection: Upgrade
// Upgrade: websocket
// Sec-Websocket-Version: 13
// Sec-Websocket-Extensions: permessage-deflate; client_max_window_bits
// Host: echo.websocket.events
// Accept-Language: zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7
// Sec-Websocket-Key: L31R1As+71fwuXqhwhABuA==`,
//
//	poc.proxy("http://127.0.0.1:7890"), poc.websocketFromServer(func(rsp, cancel) {
//		    dump(rsp)
//		}), poc.websocketOnClient(func(c) {
//		    c.WriteText("123")
//		}), poc.websocket(true),
//
// )
// time.Sleep(100)
// ```
func WithWebsocket(w bool) PocConfigOption {
	return func(c *PocConfig) {
		c.Websocket = w
	}
}

// websocketFromServer 是一个请求选项参数，它接收一个回调函数，这个函数有两个参数，其中第一个参数为服务端发送的数据，第二个参数为取消函数，调用将会强制断开 websocket
// Example:
// ```
// rsp, req, err = poc.HTTP(`GET / HTTP/1.1
// Connection: Upgrade
// Upgrade: websocket
// Sec-Websocket-Version: 13
// Sec-Websocket-Extensions: permessage-deflate; client_max_window_bits
// Host: echo.websocket.events
// Accept-Language: zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7
// Sec-Websocket-Key: L31R1As+71fwuXqhwhABuA==`,
//
//	poc.proxy("http://127.0.0.1:7890"), poc.websocketFromServer(func(rsp, cancel) {
//		    dump(rsp)
//		}), poc.websocketOnClient(func(c) {
//		    c.WriteText("123")
//		}), poc.websocket(true),
//
// )
// time.Sleep(100)
// ```
func WithWebsocketHandler(w func(i []byte, cancel func())) PocConfigOption {
	return func(c *PocConfig) {
		c.WebsocketHandler = w
	}
}

// websocketOnClient 是一个请求选项参数，它接收一个回调函数，这个函数有一个参数，是WebsocketClient结构体，通过该结构体可以向服务端发送数据
// Example:
// ```
// rsp, req, err = poc.HTTP(`GET / HTTP/1.1
// Connection: Upgrade
// Upgrade: websocket
// Sec-Websocket-Version: 13
// Sec-Websocket-Extensions: permessage-deflate; client_max_window_bits
// Host: echo.websocket.events
// Accept-Language: zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7
// Sec-Websocket-Key: L31R1As+71fwuXqhwhABuA==`,
//
//	poc.proxy("http://127.0.0.1:7890"), poc.websocketFromServer(func(rsp, cancel) {
//		    dump(rsp)
//		}), poc.websocketOnClient(func(c) {
//		    c.WriteText("123")
//		}), poc.websocket(true),
//
// )
// time.Sleep(100)
// ```
func WithWebsocketClientHandler(w func(c *lowhttp.WebsocketClient)) PocConfigOption {
	return func(c *PocConfig) {
		c.WebsocketClientHandler = w
	}
}

// websocketStrictMode 是一个请求选项参数，它用于控制是否启用严格模式，如果启用严格模式，则会遵循 RFC 6455 规范
// Example:
// ```
// rsp, req, err = poc.HTTP(`GET / HTTP/1.1
// Connection: Upgrade
// Upgrade: websocket
// Sec-Websocket-Version: 13
// Sec-Websocket-Extensions: permessage-deflate; client_max_window_bits
// Host: echo.websocket.events
// Accept-Language: zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7
// Sec-Websocket-Key: L31R1As+71fwuXqhwhABuA==`,
// poc.proxy("http://127.0.0.1:7890"), poc.websocketStrictMode(true))
//
// time.Sleep(100)
// ```
func WithWebsocketStrictMode(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.WebsocketStrictMode = b
	}
}

// port 是一个请求选项参数，用于指定实际请求的端口，如果没有设置该请求选项，则会依据原始请求报文中的Host字段来确定实际请求的端口
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.host("yaklang.com"), poc.port(443), poc.https(true)) // 实际上请求 yaklang.com 的443端口
// ```
func WithPort(port int) PocConfigOption {
	return func(c *PocConfig) {
		c.Port = port
	}
}

// jsRedirect 是一个请求选项参数，用于指定是否跟踪JS重定向，默认为false即不会自动跟踪JS重定向
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.redirectTimes(5), poc.jsRedirect(true)) // 向 www.baidu.com 发起请求，如果响应重定向到其他链接也会自动跟踪JS重定向，最多进行5次重定向
// ```
func WithJSRedirect(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.JsRedirect = &b
	}
}

// session 是一个请求选项参数，用于指定请求的session，参数可以是任意类型的值，用此值做标识符从而找到唯一的session。使用session进行请求时会自动管理cookie，这在登录后操作的场景非常有用
// Example:
// ```
// poc.Get("https://pie.dev/cookies/set/AAA/BBB", poc.session("test")) // 向 pie.dev 发起第一次请求，这会设置一个名为AAA，值为BBB的cookie
// rsp, req, err = poc.Get("https://pie.dev/cookies", poc.session("test")) // 向 pie.dev 发起第二次请求，这个请求会输出所有的cookies，可以看到第一次请求设置的cookie已经存在了
// ```
func WithSession(i interface{}) PocConfigOption {
	return func(c *PocConfig) {
		c.Session = i
	}
}

// save 是一个请求选项参数，用于指定是否将此次请求的记录保存在数据库中，默认为true即会保存到数据库
// Example:
// ```
// poc.Get("https://exmaple.com", poc.save(true)) // 向 example.com 发起请求，会将此次请求保存到数据库中
// ```
func WithSave(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.SaveHTTPFlow = &b
	}
}

// saveHandler 是一个请求选项参数，用于设置在将此次请求存入数据库之前的回调函数
// Example:
// ```
//
//	poc.Get("https://exmaple.com", poc.save(func(resp){
//		resp.Tags = append(resp.Tags,"test")
//	})) // 向 example.com 发起请求，添加test tag
//
// ```
func WithSaveHandler(f ...func(response *lowhttp.LowhttpResponse)) PocConfigOption {
	return func(c *PocConfig) {
		if c.SaveHTTPFlowHandler == nil {
			c.SaveHTTPFlowHandler = make([]func(*lowhttp.LowhttpResponse), 0)
		}
		c.SaveHTTPFlowHandler = append(c.SaveHTTPFlowHandler, f...)
	}
}

// mitmRule 是一个请求选项参数，用于指定是否使用MITM规则，默认为false即不会使用MITM规则，使用规则可以完成流量染色，附加tag与提取数据的功能
// Example:
// ```
// poc.Get("https://exmaple.com", poc.save(true), poc.mitmReplacer(true))
// ```
func WithMITMRule(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.UseMITMRule = &b
	}
}

// saveSync 是一个请求选项参数，用于指定是否将此次请求的记录保存在数据库中，且同步保存，默认为false即会异步保存到数据库
// Example:
// ```
// poc.Get("https://exmaple.com", poc.save(true), poc.saveSync(true)) // 向 example.com 发起请求，会将此次请求保存到数据库中，且同步保存
// ```
func WithSaveSync(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.SaveHTTPFlowSync = &b
	}
}

// source 是一个请求选项参数，用于在请求记录保存到数据库时标识此次请求的来源
// Example:
// ```
// poc.Get("https://exmaple.com", poc.save(true), poc.source("test")) // 向 example.com 发起请求，会将此次请求保存到数据库中，指示此次请求的来源为test
// ```
func WithSource(i string) PocConfigOption {
	return func(c *PocConfig) {
		c.Source = i
	}
}

// connPool 是一个请求选项参数，用于指定是否使用连接池，默认不使用连接池
// Example:
// ```
// rsp, req, err = poc.HTTP(x`POST /post HTTP/1.1
// Content-Type: application/json
// Host: pie.dev
//
// {"key": "asd"}`, poc.connPool(true)) // 使用连接池发送请求，这在发送多个请求时非常有用
// ```
func WithConnPool(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.ConnectionPool = b
	}
}

// context 是一个请求选项参数，用于指定请求的上下文
// Example:
// ```
// ctx = context.New()
// poc.Get("https://exmaple.com", poc.withContext(ctx)) // 向 example.com 发起请求，使用指定的上下文
// ```
func WithContext(ctx context.Context) PocConfigOption {
	return func(c *PocConfig) {
		c.Context = ctx
	}
}

// dnsServer 是一个请求选项参数，用于指定请求所使用的DNS服务器，默认使用系统自带的DNS服务器
// Example:
// ```
// // 向 example.com 发起请求，使用指定的DNS服务器
// poc.Get("https://exmaple.com", poc.dnsServer("8.8.8.8", "1.1.1.1"))
// ```
func WithDNSServers(servers ...string) PocConfigOption {
	return func(c *PocConfig) {
		c.DNSServers = lo.Uniq(append(c.DNSServers, servers...))
	}
}

// dnsNoCache 是一个请求选项参数，用于指定请求时不使用DNS缓存，默认使用DNS缓存
// Example:
// ```
// // 向 example.com 发起请求，不使用DNS缓存
// poc.Get("https://exmaple.com", poc.dnsNoCache(true))
// ```
func WithDNSNoCache(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.DNSNoCache = b
	}
}

// replaceFirstLine 是一个请求选项参数，用于改变请求报文，修改第一行（即请求方法，请求路径，协议版本）
// Example:
// ```
// poc.Get("https://exmaple.com", poc.replaceFirstLine("GET /test HTTP/1.1")) // 向 example.com 发起请求，修改请求报文的第一行，请求/test路径
// ```
func WithReplaceHttpPacketFirstLine(firstLine string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketFirstLine(packet, firstLine)
		},
		)
	}
}

// replaceMethod 是一个请求选项参数，用于改变请求报文，修改请求方法
// Example:
// ```
// poc.Options("https://exmaple.com", poc.replaceMethod("GET")) // 向 example.com 发起请求，修改请求方法为GET
// ```
func WithReplaceHttpPacketMethod(method string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketMethod(packet, method)
		},
		)
	}
}

// replaceHeader 是一个请求选项参数，用于改变请求报文，修改修改请求头，如果不存在则会增加
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.replaceHeader("AAA", "BBB")) // 向 pie.dev 发起请求，修改AAA请求头的值为BBB，这里没有AAA请求头，所以会增加该请求头
// ```
func WithReplaceHttpPacketHeader(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketHeader(packet, key, value)
		},
		)
	}
}

// replaceAllHeaders 是一个请求选项参数，用于改变请求报文，修改修改所有请求头
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.replaceHeader("AAA", "BBB")) // 向 pie.dev 发起请求，修改AAA请求头的值为BBB，这里没有AAA请求头，所以会增加该请求头
// ```
func WithReplaceAllHttpPacketHeaders(headers map[string]string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceAllHTTPPacketHeaders(packet, headers)
		},
		)
	}
}

// replaceHost 是一个请求选项参数，用于改变请求报文，修改Host请求头，如果不存在则会增加，实际上是replaceHeader("Host", host)的简写
// Example:
// ```
// poc.Get("https://yaklang.com/", poc.replaceHost("www.yaklang.com")) // 向 yaklang.com 发起请求，修改Host请求头的值为 www.yaklang.com
// ```
func WithReplaceHttpPacketHost(host string) PocConfigOption {
	return WithReplaceHttpPacketHeader("Host", host)
}

// replaceBasicAuth 是一个请求选项参数，用于改变请求报文，修改 Authorization 请求头为基础认证的密文，如果不存在则会增加，实际上是replaceHeader("Authorization", codec.EncodeBase64(username + ":" + password))的简写
// Example:
// ```
// poc.Get("https://pie.dev/basic-auth/admin/password", poc.replaceBasicAuth("admin", "password")) // 向 pie.dev 发起请求进行基础认证，会得到200响应状态码
// ```
func WithReplaceHttpPacketBasicAuth(username, password string) PocConfigOption {
	return WithReplaceHttpPacketHeader("Authorization", "Basic "+codec.EncodeBase64(username+":"+password))
}

// replaceUserAgent 是一个请求选项参数，用于改变请求报文，修改 User-Agent 请求头，实际上是replaceHeader("User-Agent", userAgent)的简写
// Example:
// ```
// poc.Get("https://pie.dev/basic-auth/admin/password", poc.replaceUserAgent("yak-http-client")) // 向 pie.dev 发起请求，修改 User-Agent 请求头为 yak-http-client
// ```
func WithReplaceHttpPacketUserAgent(ua string) PocConfigOption {
	return WithReplaceHttpPacketHeader("User-Agent", ua)
}

// replaceRandomUserAgent 是一个请求选项参数，用于改变请求报文，修改 User-Agent 请求头为随机的常见请求头
// Example:
// ```
// poc.Get("https://pie.dev/basic-auth/admin/password", poc.replaceRandomUserAgent()) // 向 pie.dev 发起请求，修改 User-Agent 请求头为随机的常见请求头
// ```
func WithReplaceHttpPacketRandomUserAgent() PocConfigOption {
	return WithReplaceHttpPacketHeader("User-Agent", uarand.GetRandom())
}

// replaceCookie 是一个请求选项参数，用于改变请求报文，修改Cookie请求头中的值，如果不存在则会增加
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.replaceCookie("aaa", "bbb")) // 向 pie.dev 发起请求，这里没有aaa的cookie值，所以会增加
// ```
func WithReplaceHttpPacketCookie(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketCookie(packet, key, value)
		},
		)
	}
}

// replaceAllCookies 是一个请求选项参数，用于改变请求报文，修改所有Cookie请求头中的值
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.replaceAllCookies({"aaa":"bbb", "ccc":"ddd"})) // 向 pie.dev 发起请求，修改aaa的cookie值为bbb，修改ccc的cookie值为ddd
// ```
func WithReplaceHttpPacketCookies(cookies any) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketCookies(packet, cookies)
		},
		)
	}
}

// replaceBody 是一个请求选项参数，用于改变请求报文，修改请求体内容，第一个参数为修改后的请求体内容，第二个参数为是否分块传输
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.replaceBody("a=b", false)) // 向 pie.dev 发起请求，修改请求体内容为a=b
// ```
func WithReplaceHttpPacketBody(body []byte, chunk bool) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketBody(packet, body, chunk)
		},
		)
	}
}

// replacePath 是一个请求选项参数，用于改变请求报文，修改请求路径
// Example:
// ```
// poc.Get("https://pie.dev/post", poc.replacePath("/get")) // 向 pie.dev 发起请求，实际上请求路径为/get
// ```
func WithReplaceHttpPacketPath(path string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketPath(packet, path)
		},
		)
	}
}

func WithReplaceHttpPacketQueryParamRaw(rawQuery string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketQueryParamRaw(packet, rawQuery)
		},
		)
	}
}

// replaceQueryParam 是一个请求选项参数，用于改变请求报文，修改 GET 请求参数，如果不存在则会增加
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.replaceQueryParam("a", "b")) // 向 pie.dev 发起请求，添加GET请求参数a，值为b
// ```
func WithReplaceHttpPacketQueryParam(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketQueryParam(packet, key, value)
		},
		)
	}
}

// replaceAllQueryParams 是一个请求选项参数，用于改变请求报文，修改所有 GET 请求参数，如果不存在则会增加，其接收一个map[string]string 类型的参数，其中 key 为请求参数名，value 为请求参数值
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.replaceAllQueryParams({"a":"b", "c":"d"})) // 向 pie.dev 发起请求，添加GET请求参数a，值为b，添加GET请求参数c，值为d
// ```
func WithReplaceAllHttpPacketQueryParams(values map[string]string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceAllHTTPPacketQueryParams(packet, values)
		},
		)
	}
}

// replaceAllQueryParamsWithoutEscape 是一个请求选项参数，用于改变请求报文，修改所有 GET 请求参数，如果不存在则会增加，其接收一个map[string]string 类型的参数，其中 key 为请求参数名，value 为请求参数值
// 与 poc.replaceAllQueryParams 类似，但是不会将参数值进行转义
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.replaceAllQueryParamsWithoutEscape({"a":"{{}}", "c":"%%"})) // 向 pie.dev 发起请求，添加GET请求参数a，值为{{}}，添加GET请求参数c，值为%%
// ```
func WithReplaceAllHttpPacketQueryParamsWithoutEscape(values map[string]string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceAllHTTPPacketQueryParamsWithoutEscape(packet, values)
		},
		)
	}
}

func WithReplaceFullHttpPacketQueryParamsWithoutEscape(values map[string][]string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceFullHTTPPacketQueryParamsWithoutEscape(packet, values)
		},
		)
	}
}

// replacePostParam 是一个请求选项参数，用于改变请求报文，修改 POST 请求参数，如果不存在则会增加
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.replacePostParam("a", "b")) // 向 pie.dev 发起请求，添加POST请求参数a，值为b
// ```
func WithReplaceHttpPacketPostParam(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketPostParam(packet, key, value)
		},
		)
	}
}

// replaceAllPostParams 是一个请求选项参数，用于改变请求报文，修改所有POST请求参数，如果不存在则会增加，其接收一个map[string]string类型的参数，其中key为POST请求参数名，value为POST请求参数值
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.replaceAllPostParams({"a":"b", "c":"d"})) // 向 pie.dev 发起请求，添加POST请求参数a，值为b，POST请求参数c，值为d
// ```
func WithReplaceAllHttpPacketPostParams(values map[string]string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceAllHTTPPacketPostParams(packet, values)
		},
		)
	}
}

// replaceAllPostParamsWithoutEscape 是一个请求选项参数，用于改变请求报文，修改所有POST请求参数，如果不存在则会增加，其接收一个map[string]string类型的参数，其中key为POST请求参数名，value为POST请求参数值
// 与 poc.replaceAllPostParams 类似，但是不会将参数值进行转义
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.replaceAllPostParamsWithoutEscape({"a":"{{}}", "c":"%%"})) // 向 pie.dev 发起请求，添加POST请求参数a，值为{{}}，POST请求参数c，值为%%
// ```
func WithReplaceAllHttpPacketPostParamsWithoutEscape(values map[string]string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceAllHTTPPacketPostParamsWithoutEscape(packet, values)
		},
		)
	}
}

func WithReplaceFullHttpPacketPostParamsWithoutEscape(values map[string][]string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceFullHTTPPacketPostParamsWithoutEscape(packet, values)
		},
		)
	}
}

// replaceFormEncoded 是一个请求选项参数，用于改变请求报文，修改请求体中的表单，如果不存在则会增加
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.replaceFormEncoded("aaa", "bbb")) // 向 pie.dev 发起请求，添加POST请求表单，其中aaa为键，bbb为值
// ```
func WithReplaceHttpPacketFormEncoded(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketFormEncoded(packet, key, value)
		},
		)
	}
}

// replaceUploadFile 是一个请求选项参数，用于改变请求报文，修改请求体中的上传的文件，其中第一个参数为表单名，第二个参数为文件名，第三个参数为文件内容，第四个参数是可选参数，为文件类型(Content-Type)，如果不存在则会增加
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.replaceUploadFile("file", "phpinfo.php", "<?php phpinfo(); ?>", "application/x-php")) // 向 pie.dev 发起请求，添加POST请求上传文件，其中file为表单名，phpinfo.php为文件名，<?php phpinfo(); ?>为文件内容，application/x-php为文件类型
// ```
func WithReplaceHttpPacketUploadFile(formName, fileName string, fileContent []byte, contentType ...string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.ReplaceHTTPPacketUploadFile(packet, formName, fileName, fileContent, contentType...)
		},
		)
	}
}

// appendHeader 是一个请求选项参数，用于改变请求报文，添加请求头
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.appendHeader("AAA", "BBB")) // 向 pie.dev 发起请求，添加AAA请求头的值为BBB
// ```
func WithAppendHeader(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketHeader(packet, key, value)
		},
		)
	}
}

func WithAppendHeaderIfNotExist(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketHeaderIfNotExist(packet, key, value)
		},
		)
	}
}

// appendHeaders 是一个请求选项参数，用于改变请求报文，添加请求头
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.appendHeaders({"AAA": "BBB","CCC": "DDD"})) // 向 pie.dev 发起请求，添加AAA请求头的值为BBB
// ```
func WithAppendHeaders(headers map[string]string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			for key, value := range headers {
				packet = lowhttp.AppendHTTPPacketHeader(packet, key, value)
			}
			return packet
		},
		)
	}
}

// appendCookie 是一个请求选项参数，用于改变请求报文，添加 Cookie 请求头中的值
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.appendCookie("aaa", "bbb")) // 向 pie.dev 发起请求，添加cookie键值对aaa:bbb
// ```
func WithAppendCookie(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketCookie(packet, key, value)
		},
		)
	}
}

// appendQueryParam 是一个请求选项参数，用于改变请求报文，添加 GET 请求参数
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.appendQueryParam("a", "b")) // 向 pie.dev 发起请求，添加GET请求参数a，值为b
// ```
func WithAppendQueryParam(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketQueryParam(packet, key, value)
		},
		)
	}
}

// appendPostParam 是一个请求选项参数，用于改变请求报文，添加 POST 请求参数
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.appendPostParam("a", "b")) // 向 pie.dev 发起请求，添加POST请求参数a，值为b
// ```
func WithAppendPostParam(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketPostParam(packet, key, value)
		},
		)
	}
}

// appendPath 是一个请求选项参数，用于改变请求报文，在现有请求路径后添加请求路径
// Example:
// ```
// poc.Get("https://yaklang.com/docs", poc.appendPath("/api/poc")) // 向 yaklang.com 发起请求，实际上请求路径为/docs/api/poc
// ```
func WithAppendHttpPacketPath(path string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketPath(packet, path)
		},
		)
	}
}

// appendFormEncoded 是一个请求选项参数，用于改变请求报文，添加请求体中的表单
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.appendFormEncoded("aaa", "bbb")) // 向 pie.dev 发起请求，添加POST请求表单，其中aaa为键，bbb为值
// ```
func WithAppendHttpPacketFormEncoded(key, value string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketFormEncoded(packet, key, value)
		},
		)
	}
}

// appendUploadFile 是一个请求选项参数，用于改变请求报文，添加请求体中的上传的文件，其中第一个参数为表单名，第二个参数为文件名，第三个参数为文件内容，第四个参数是可选参数，为文件类型(Content-Type)
// Example:
// ```
// poc.Post("https://pie.dev/post", poc.appendUploadFile("file", "phpinfo.php", "<?php phpinfo(); ?>", "image/jpeg"))// 向 pie.dev 发起请求，添加POST请求表单，其文件名为phpinfo.php，内容为<?php phpinfo(); ?>，文件类型为image/jpeg
// ```
func WithAppendHttpPacketUploadFile(fieldName, fileName string, fileContent interface{}, contentType ...string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.AppendHTTPPacketUploadFile(packet, fieldName, fileName, fileContent, contentType...)
		},
		)
	}
}

// deleteHeader 是一个请求选项参数，用于改变请求报文，删除请求头
// Example:
// ```
// poc.HTTP(`GET /get HTTP/1.1
// Content-Type: application/json
// AAA: BBB
// Host: pie.dev
//
// `, poc.deleteHeader("AAA"))// 向 pie.dev 发起请求，删除AAA请求头
// ```
func WithDeleteHeader(key string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.DeleteHTTPPacketHeader(packet, key)
		},
		)
	}
}

// deleteCookie 是一个请求选项参数，用于改变请求报文，删除 Cookie 中的值
// Example:
// ```
// poc.HTTP(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: aaa=bbb; ccc=ddd
// Host: pie.dev
//
// `, poc.deleteCookie("aaa"))// 向 pie.dev 发起请求，删除Cookie中的aaa
// ```
func WithDeleteCookie(key string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.DeleteHTTPPacketCookie(packet, key)
		},
		)
	}
}

// deleteQueryParam 是一个请求选项参数，用于改变请求报文，删除 GET 请求参数
// Example:
// ```
// poc.HTTP(`GET /get?a=b&c=d HTTP/1.1
// Content-Type: application/json
// Host: pie.dev
//
// `, poc.deleteQueryParam("a")) // 向 pie.dev 发起请求，删除GET请求参数a
// ```
func WithDeleteQueryParam(key string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.DeleteHTTPPacketQueryParam(packet, key)
		},
		)
	}
}

// deletePostParam 是一个请求选项参数，用于改变请求报文，删除 POST 请求参数
// Example:
// ```
// poc.HTTP(`POST /post HTTP/1.1
// Content-Type: application/json
// Content-Length: 7
// Host: pie.dev
//
// a=b&c=d`, poc.deletePostParam("a")) // 向 pie.dev 发起请求，删除POST请求参数a
// ```
func WithDeletePostParam(key string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.DeleteHTTPPacketPostParam(packet, key)
		},
		)
	}
}

// deleteForm 是一个请求选项参数，用于改变请求报文，删除 POST 请求表单
// Example:
// ```
// poc.HTTP(`POST /post HTTP/1.1
// Host: pie.dev
// Content-Type: multipart/form-data; boundary=------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Length: 308
//
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="aaa"
//
// bbb
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="ccc"
//
// ddd
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm--`, poc.deleteForm("aaa")) // 向 pie.dev 发起请求，删除POST请求表单aaa
// ```
func WithDeleteForm(key string) PocConfigOption {
	return func(c *PocConfig) {
		c.PacketHandler = append(c.PacketHandler, func(packet []byte) []byte {
			return lowhttp.DeleteHTTPPacketForm(packet, key)
		},
		)
	}
}

// randomChunked 是一个请求选项参数，用于启用随机分块传输，默认不启用
// Example:
// ```
// data = `
// POST /post HTTP/1.1
// Host: pie.dev
// Content-Type: multipart/form-data; boundary=------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Length: 308
//
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="aaa"
//
// bbb
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="ccc"
//
// ddd
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm--
// `
//
// poc.HTTP(data,poc.randomChunked(true),poc.randomChunkedLength(10,25),poc.randomChunkedDelay(50,200))~
// ```
func WithEnableRandomChunked(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.EnableRandomChunked = b
	}
}

// randomChunkedLength 是一个请求选项参数，用于设置随机分块传输的分块长度范围，默认最小长度为10，最大长度为25
// Example:
// ```
// data = `
// POST /post HTTP/1.1
// Host: pie.dev
// Content-Type: multipart/form-data; boundary=------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Length: 308
//
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="aaa"
//
// bbb
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="ccc"
//
// ddd
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm--
// `
//
// poc.HTTP(data,poc.randomChunked(true),poc.randomChunkedLength(10,25),poc.randomChunkedDelay(50,200))~
// ```
func WithRandomChunkedLength(min, max int) PocConfigOption {
	return func(c *PocConfig) {
		c.MinChunkedLength = min
		c.MaxChunkedLength = max
	}
}

// randomChunkedDelay是一个请求选项参数，用于设置随机分块传输的分块延迟范围，默认最小延迟为50毫秒，最大延迟为100毫秒
// Example:
// ```
// data = `
// POST /post HTTP/1.1
// Host: pie.dev
// Content-Type: multipart/form-data; boundary=------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Length: 308
//
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="aaa"
//
// bbb
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="ccc"
//
// ddd
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm--
// `
//
// poc.HTTP(data,poc.randomChunked(true),poc.randomChunkedLength(10,25),poc.randomChunkedDelay(50,200))~
// ```
func WithRandomChunkedDelay(min, max int) PocConfigOption {
	return func(c *PocConfig) {
		c.MinChunkDelay = time.Duration(min) * time.Millisecond
		c.MaxChunkDelay = time.Duration(max) * time.Millisecond
	}
}

// randomChunkedResultHandler 是一个请求选项参数，用于设置随机分块传输的结果处理函数
// 处理函数接受四个参数，id为分块的ID，chunkRaw为分块的原始数据，totalTime为总耗时，chunkSendTime为分块发送的耗时
// Example:
// ```
// data = `
// POST /post HTTP/1.1
// Host: pie.dev
// Content-Type: multipart/form-data; boundary=------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Length: 308
//
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="aaa"
//
// bbb
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="ccc"
//
// ddd
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm--
// `
//
// poc.HTTP(data,poc.randomChunked(true),
// poc.randomChunkedLength(10,25),
// poc.randomChunkedDelay(50,200),
//
//	poc.randomChunkedResultHandler(func(id,data,totalTime,chunkTime){
//		print(sprintf("id:%v\tdata:%s\ttotalTime:%vms\tdelay:%vms\n", id,data,totalTime,chunkTime))
//	}))~
//
// ```
func WithRandomChunkedResultHandler(f func(id int, chunkRaw []byte, totalTime time.Duration, chunkSendTime time.Duration)) PocConfigOption {
	return func(c *PocConfig) {
		c.ChunkedHandler = f
	}
}

func FixPacketByPocOptions(packet []byte, opts ...PocConfigOption) []byte {
	config := NewDefaultPoCConfig()
	for _, opt := range opts {
		opt(config)
	}
	return fixPacketByConfig(packet, config)
}

func fixPacketByConfig(packet []byte, config *PocConfig) []byte {
	for _, fixFunc := range config.PacketHandler {
		packet = fixFunc(packet)
	}
	return packet
}

func handleUrlAndConfig(urlStr string, opts ...PocConfigOption) (*PocConfig, error) {
	// poc 模块收 proxy 影响
	proxy := cli.DefaultCliApp.StringSlice("proxy")
	config := NewDefaultPoCConfig()
	config.Proxy = proxy
	for _, opt := range opts {
		opt(config)
	}

	host, port, err := utils.ParseStringToHostPort(urlStr)
	if err != nil {
		return config, utils.Errorf("parse url failed: %s", err)
	}

	if port == 443 || strings.HasPrefix(urlStr, "https://") || strings.HasPrefix(urlStr, "wss://") {
		https := true
		config.ForceHttps = &https
	}

	if config.Host == "" {
		config.Host = host
	}

	if config.Port == 0 {
		config.Port = port
	}

	if config.NoRedirect != nil && *config.NoRedirect {
		config.RedirectTimes = nil
	}
	return config, nil
}

func handleRawPacketAndConfig(i interface{}, opts ...PocConfigOption) ([]byte, *PocConfig, error) {
	var packet []byte
	switch ret := i.(type) {
	case string:
		packet = []byte(ret)
	case []byte:
		packet = ret
	case http.Request:
		r := &ret
		lowhttp.FixRequestHostAndPort(r)
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			return nil, nil, utils.Errorf("dump request out failed: %s", err)
		}
		packet = raw
	case *http.Request:
		lowhttp.FixRequestHostAndPort(ret)
		raw, err := utils.DumpHTTPRequest(ret, true)
		if err != nil {
			return nil, nil, utils.Errorf("dump request out failed: %s", err)
		}
		packet = raw
	case *http_struct.YakHttpRequest:
		raw, err := utils.DumpHTTPRequest(ret.Request, true)
		if err != nil {
			return nil, nil, utils.Errorf("dump request out failed: %s", err)
		}
		packet = raw
	default:
		return nil, nil, utils.Errorf("cannot support: %s", reflect.TypeOf(i))
	}

	// poc 模块收 proxy 影响
	proxy := cli.DefaultCliApp.StringSlice("proxy")
	config := NewDefaultPoCConfig()
	config.Proxy = proxy
	for _, opt := range opts {
		opt(config)
	}

	// 根据config修改packet
	packet = fixPacketByConfig(packet, config)

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

	https := config.IsHTTPS()
	u, err := lowhttp.ExtractURLFromHTTPRequestRaw(packet, https)
	if err != nil {
		return nil, config, utils.Errorf("extract url failed: %s", err)
	}

	host, port, err := utils.ParseStringToHostPort(u.String())
	if err != nil {
		return nil, config, utils.Errorf("parse url failed: %s", err)
	}

	if port == 443 {
		https = true
		config.ForceHttps = &https
	}

	if config.Host == "" {
		config.Host = host
	}

	if config.Port == 0 {
		config.Port = port
	}

	if config.NoRedirect != nil && *config.NoRedirect {
		config.RedirectTimes = nil
	}

	return packet, config, nil
}

func pochttp(packet []byte, config *PocConfig) (*lowhttp.LowhttpResponse, error) {
	if config.Websocket {
		if config.Timeout == nil || *config.Timeout <= 0 {
			config.Timeout = &defaultTimeout
		}
		wsCtx, cancel := context.WithTimeout(context.Background(), *config.Timeout)
		defer cancel()

		https := config.IsHTTPS()

		c, err := lowhttp.NewWebsocketClient(
			packet,
			lowhttp.WithWebsocketTLS(https),
			lowhttp.WithWebsocketProxy(config.Proxy...),
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
			lowhttp.WithWebsocketStrictMode(config.WebsocketStrictMode),
		)
		c.Start()
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

	opts := config.ToLowhttpOptions()
	opts = append(opts, lowhttp.WithPacketBytes(packet))

	response, err := lowhttp.HTTP(
		opts...,
	)
	return response, err
}

// HTTPEx 与 HTTP 类似，它发送请求并且返回响应结构体，请求结构体以及错误，它的第一个参数可以接收 []byte, string, http.Request 结构体，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如设置超时时间，或者修改请求报文等
// 关于结构体中的可用字段和方法可以使用 desc 函数进行查看
// Example:
// ```
// rsp, req, err = poc.HTTPEx(`GET / HTTP/1.1\r\nHost: www.yaklang.com\r\n\r\n`, poc.https(true), poc.replaceHeader("AAA", "BBB")) // 向yaklang.com发送一个基于HTTPS协议的GET请求，并且添加一个请求头AAA，它的值为BBB
// desc(rsp) // 查看响应结构体中的可用字段
// ```
func HTTPEx(i interface{}, opts ...PocConfigOption) (rspInst *lowhttp.LowhttpResponse, reqInst *http.Request, err error) {
	packet, config, err := handleRawPacketAndConfig(i, opts...)
	if err != nil {
		return nil, nil, err
	}
	response, err := pochttp(packet, config)
	if err != nil {
		return nil, nil, err
	}
	request, err := lowhttp.ParseBytesToHttpRequest(response.RawRequest)
	if err != nil {
		return nil, nil, err
	}
	return response, request, nil
}

// BuildRequest 是一个用于辅助构建请求报文的工具函数，它第一个参数可以接收 []byte, string, http.Request 结构体，接下来可以接收零个到多个请求选项，修改请求报文的选项将被作用，最后返回构建好的请求报文
// Example:
// ```
// raw = poc.BuildRequest(poc.BasicRequest(), poc.https(true), poc.replaceHost("yaklang.com"), poc.replacePath("/docs/api/poc")) // 构建一个基础GET请求，修改其Host为yaklang.com，访问的URI路径为/docs/api/poc
// // raw = b"GET /docs/api/poc HTTP/1.1\r\nHost: www.yaklang.com\r\n\r\n"
// ```
func BuildRequest(i interface{}, opts ...PocConfigOption) []byte {
	packet, _, err := handleRawPacketAndConfig(i, opts...)
	if err != nil {
		log.Errorf("build request error: %s", err)
	}
	return packet
}

// HTTP 发送请求并且返回原始响应报文，原始请求报文以及错误，它的第一个参数可以接收 []byte, string, http.Request 结构体，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如设置超时时间，或者修改请求报文等
// Example:
// ```
// poc.HTTP("GET / HTTP/1.1\r\nHost: www.yaklang.com\r\n\r\n", poc.https(true), poc.replaceHeader("AAA", "BBB")) // yaklang.com发送一个基于HTTPS协议的GET请求，并且添加一个请求头AAA，它的值为BBB
// ```
func HTTP(i interface{}, opts ...PocConfigOption) (rsp []byte, req []byte, err error) {
	packet, config, err := handleRawPacketAndConfig(i, opts...)
	if err != nil {
		return nil, nil, err
	}
	response, err := pochttp(packet, config)
	if err != nil {
		return nil, nil, err
	}
	noFixContentLength := config.NoFixContentLength != nil && *config.NoFixContentLength
	return response.RawPacket, lowhttp.FixHTTPPacketCRLF(response.RawRequest, noFixContentLength), err
}

// Do 向指定 URL 发送指定请求方法的请求并且返回响应结构体，请求结构体以及错误，它的是第一个参数是请求方法，第二个参数 URL 字符串，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如设置超时时间，或者修改请求报文等
// 关于结构体中的可用字段和方法可以使用 desc 函数进行查看
// Example:
// ```
// poc.Do("GET","https://yaklang.com", poc.https(true)) // 向yaklang.com发送一个基于HTTPS协议的GET请求
// desc(rsp) // 查看响应结构体中的可用字段
// ```
func Do(method string, urlStr string, opts ...PocConfigOption) (rspInst *lowhttp.LowhttpResponse, reqInst *http.Request, err error) {
	config, err := handleUrlAndConfig(urlStr, opts...)
	if err != nil {
		return nil, nil, err
	}
	https := config.IsHTTPS()
	packet := lowhttp.UrlToRequestPacket(method, urlStr, nil, https)
	packet = fixPacketByConfig(packet, config)

	response, err := pochttp(packet, config)
	if err != nil {
		return nil, nil, err
	}
	request, err := lowhttp.ParseBytesToHttpRequest(response.RawRequest)
	if err != nil {
		return nil, nil, err
	}
	return response, request, nil
}

// Get 向指定 URL 发送 GET 请求并且返回响应结构体，请求结构体以及错误，它的第一个参数是 URL 字符串，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如对设置超时时间，或者修改请求报文等
// 关于结构体中的可用字段和方法可以使用 desc 函数进行查看
// Example:
// ```
// rsp,req = poc.Get("https://yaklang.com", poc.https(true))~ // 向yaklang.com发送一个基于HTTPS协议的GET请求
// desc(rsp) // 查看响应结构体中的可用字段
// ```
func DoGET(urlStr string, opts ...PocConfigOption) (rspInst *lowhttp.LowhttpResponse, reqInst *http.Request, err error) {
	return Do("GET", urlStr, opts...)
}

// Post 向指定 URL 发送 POST 请求并且返回响应结构体，请求结构体以及错误，它的第一个参数是 URL 字符串，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如对设置超时时间，或者修改请求报文等
// 关于结构体中的可用字段和方法可以使用 desc 函数进行查看
// Example:
// ```
// rsp,req = poc.Post("https://yaklang.com", poc.https(true))~ // 向yaklang.com发送一个基于HTTPS协议的POST请求
// desc(rsp) // 查看响应结构体中的可用字段
// ```
func DoPOST(urlStr string, opts ...PocConfigOption) (rspInst *lowhttp.LowhttpResponse, reqInst *http.Request, err error) {
	return Do("POST", urlStr, opts...)
}

// Head 向指定 URL 发送 HEAD 请求并且返回响应结构体，请求结构体以及错误，它的第一个参数是 URL 字符串，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如对设置超时时间，或者修改请求报文等
// 关于结构体中的可用字段和方法可以使用 desc 函数进行查看
// Example:
// ```
// rsp,req = poc.Head("https://yaklang.com", poc.https(true))~ // 向yaklang.com发送一个基于HTTPS协议的HEAD请求
// desc(rsp) // 查看响应结构体中的可用字段
// ```
func DoHEAD(urlStr string, opts ...PocConfigOption) (rspInst *lowhttp.LowhttpResponse, reqInst *http.Request, err error) {
	return Do("HEAD", urlStr, opts...)
}

// Delete 向指定 URL 发送 DELETE 请求并且返回响应结构体，请求结构体以及错误，它的第一个参数是 URL 字符串，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如对设置超时时间，或者修改请求报文等
// 关于结构体中的可用字段和方法可以使用 desc 函数进行查看
// Example:
// ```
// rsp,req = poc.Delete("https://yaklang.com", poc.https(true))~ // 向yaklang.com发送一个基于HTTPS协议的DELETE请求
// desc(rsp) // 查看响应结构体中的可用字段
// ```
func DoDELETE(urlStr string, opts ...PocConfigOption) (rspInst *lowhttp.LowhttpResponse, reqInst *http.Request, err error) {
	return Do("DELETE", urlStr, opts...)
}

// Options 向指定 URL 发送 OPTIONS 请求并且返回响应结构体，请求结构体以及错误，它的第一个参数是 URL 字符串，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如对设置超时时间，或者修改请求报文等
// 关于结构体中的可用字段和方法可以使用 desc 函数进行查看
// Example:
// ```
// rsp,req = poc.Options("https://yaklang.com", poc.https(true))~ // 向yaklang.com发送一个基于HTTPS协议的Options请求
// desc(rsp) // 查看响应结构体中的可用字段
// ```
func DoOPTIONS(urlStr string, opts ...PocConfigOption) (rspInst *lowhttp.LowhttpResponse, reqInst *http.Request, err error) {
	return Do("OPTIONS", urlStr, opts...)
}

// Websocket 实际上等价于`poc.HTTP(..., poc.websocket(true))`，用于快速发送请求并建立websocket连接并且返回原始响应报文，原始请求报文以及错误
// Example:
// ```
// rsp, req, err = poc.Websocket(`GET / HTTP/1.1
// Connection: Upgrade
// Upgrade: websocket
// Sec-Websocket-Version: 13
// Sec-Websocket-Extensions: permessage-deflate; client_max_window_bits
// Host: echo.websocket.events
// Accept-Language: zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7
// Sec-Websocket-Key: L31R1As+71fwuXqhwhABuA==`,
//
//	poc.proxy("http://127.0.0.1:7890"), poc.websocketFromServer(func(rsp, cancel) {
//		    dump(rsp)
//		}), poc.websocketOnClient(func(c) {
//		    c.WriteText("123")
//		})
//
// )
// time.Sleep(100)
// ```
func DoWebSocket(raw interface{}, opts ...PocConfigOption) (rsp []byte, req []byte, err error) {
	opts = append(opts, WithWebsocket(true))
	return HTTP(raw, opts...)
}

// Split 切割 HTTP 报文，返回响应头和响应体，其第一个参数是原始HTTP报文，接下来可以接收零个到多个回调函数，其在每次解析到请求头时回调
// Example:
// ```
// poc.Split(`POST / HTTP/1.1
// Content-Type: application/json
// Host: www.example.com
//
// {"key": "value"}`, func(header) {
// dump(header)
// })
// ```
func split(raw []byte, hook ...func(line string)) (headers string, body []byte) {
	return lowhttp.SplitHTTPHeadersAndBodyFromPacket(raw, hook...)
}

// FixHTTPRequest 尝试对传入的HTTP请求报文进行修复，并返回修复后的请求
// Example:
// ```
// poc.FixHTTPRequest(b"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
// ```
func fixHTTPRequest(raw []byte) []byte {
	return lowhttp.FixHTTPRequest(raw)
}

// FixHTTPResponse 尝试对传入的 HTTP 响应报文进行修复，并返回修复后的响应
// Example:
// ```
// poc.FixHTTPResponse(b"HTTP/1.1 200 OK\nContent-Length: 5\n\nhello")
// ```
func fixHTTPResponse(r []byte) []byte {
	rsp, _, _ := lowhttp.FixHTTPResponse(r)
	return rsp
}

// CurlToHTTPRequest 尝试将curl命令转换为HTTP请求报文，其返回值为bytes，即转换后的HTTP请求报文
// Example:
// ```
// poc.CurlToHTTPRequest("curl -X POST -d 'a=b&c=d' http://example.com")
// ```
func curlToHTTPRequest(command string) (req []byte) {
	raw, err := lowhttp.CurlToRawHTTPRequest(command)
	if err != nil {
		log.Errorf(`CurlToHTTPRequest failed: %s`, err)
	}
	return raw
}

// HTTPRequestToCurl 尝试将 HTTP 请求报文转换为curl命令。第一个参数为是否使用HTTPS，第二个参数为HTTP请求报文，其返回值为string，即转换后的curl命令
// Example:
// ```
// poc.HTTPRequestToCurl(true, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
// ```
func httpRequestToCurl(https bool, raw any) (curlCommand string) {
	cmd, err := lowhttp.GetCurlCommand(https, utils.InterfaceToBytes(raw))
	if err != nil {
		log.Errorf(`http2curl.GetCurlCommand(req): %v`, err)
		return ""
	}
	return cmd.String()
}

// ExtractPostParams 尝试将 HTTP 请求报文中的各种 POST 参数(普通格式，表单格式，JSON格式，XML格式)提取出来，返回提取出来的 POST 参数与错误
// Example:
// ```
// params, err = poc.ExtractPostParams("POST / HTTP/1.1\r\nContent-Type: application/json\r\nHost: example.com\r\n\r\n{\"key\": \"value\"}")
// dump(params) // {"key": "value"}
// ```
func ExtractPostParams(raw []byte) (map[string]string, error) {
	_, body := lowhttp.SplitHTTPPacketFast(raw)
	contentType := lowhttp.GetHTTPPacketHeader(raw, "Content-Type")
	totalParams, useRaw, err := lowhttp.GetParamsFromBody(contentType, body)
	if useRaw && err == nil {
		err = utils.Error("cannot extract post params")
	}
	params := make(map[string]string)
	for _, param := range totalParams.Items {
		if len(param.Values) == 0 {
			params[param.Key] = ""
		} else {
			params[param.Key] = param.Values[0]
		}
	}

	return params, err
}

// ua 是一个请求选项参数，用于改变请求报文，添加 User-Agent 请求头中的值
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.ua("Mozilla/5.0")) // 向 pie.dev 发起请求，添加User-Agent请求头，其值为Mozilla/5.0
// ```
func WithUserAgent(ua string) PocConfigOption {
	return WithReplaceHttpPacketHeader("User-Agent", ua)
}

// cookie 是一个请求选项参数，用于改变请求报文，添加 Cookie 请求头中的值
// Example:
// ```
// poc.Get("https://pie.dev/get", poc.cookie("a=b; c=d")) // 向 pie.dev 发起请求，添加Cookie请求头，其值为a=b; c=d
// poc.Get("https://pie.dev/get", poc.cookie("a", "b")) // 向 pie.dev 发起请求，添加Cookie请求头，其值为a=b
// ```
func WithCookieFull(c string, values ...any) PocConfigOption {
	if len(values) > 0 {
		valueStrings := utils.InterfaceToStringSlice(values)
		return WithReplaceHttpPacketCookie(c, strings.Join(valueStrings, ","))
	}
	return WithReplaceHttpPacketHeader("Cookie", c)
}

func WithGmTls() PocConfigOption {
	return func(c *PocConfig) {
		c.GmTLS = true
	}
}

func WithGmTlsOnly() PocConfigOption {
	return func(c *PocConfig) {
		c.GmTLSOnly = true
	}
}

func WithGmTLSPrefer() PocConfigOption {
	return func(c *PocConfig) {
		c.GmTLSPrefer = true
	}
}

// fixQueryEscape 是一个请求选项参数，用于指定是否修复查询参数中的 URL 编码，默认为 false 即会自动修复URL编码
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.fixQueryEscape(true)) // 向 example.com 发起请求，如果查询参数中的 URL 编码不正确或不存在也不会自动修复URL编码
// ```
func WithFixQueryEscape(b bool) PocConfigOption {
	return func(c *PocConfig) {
		c.FixQueryEscape = &b
	}
}

var PoCExports = map[string]interface{}{
	"HTTP":          HTTP,
	"HTTPEx":        HTTPEx,
	"BasicRequest":  lowhttp.BasicRequest,
	"BasicResponse": lowhttp.BasicResponse,
	"BuildRequest":  BuildRequest,
	"Get":           DoGET,
	"Post":          DoPOST,
	"Head":          DoHEAD,
	"Delete":        DoDELETE,
	"Options":       DoOPTIONS,
	"Do":            Do,
	// websocket，可以直接复用 HTTP 参数
	"Websocket": DoWebSocket,

	// options
	"host":                 WithHost,
	"port":                 WithPort,
	"retryTimes":           WithRetryTimes,
	"retryInStatusCode":    WithRetryInStatusCode,
	"retryNotInStatusCode": WithRetryNotInStausCode,
	"retryWaitTime":        WithRetryWaitTime,
	"retryMaxWaitTime":     WithRetryMaxWaitTime,
	"redirectTimes":        WithRedirectTimes,
	"noRedirect":           WithNoRedirect,
	"noredirect":           WithNoRedirect,
	"jsRedirect":           WithJSRedirect,
	"redirect":             WithRedirect,
	"redirectHandler":      WithRedirectHandler,
	"https":                WithForceHTTPS,
	"http2":                WithForceHTTP2,
	"sni":                  WithSNI,
	"params":               WithParams,
	"proxy":                WithProxy,
	"timeout":              WithTimeout,
	"context":              WithContext,
	"connPool":             WithConnPool,
	"connectTimeout":       WithConnectTimeout,
	"dnsServer":            WithDNSServers,
	"dnsNoCache":           WithDNSNoCache,
	"noFixContentLength":   WithNoFixContentLength,
	"noBodyBuffer":         WithNoBodyBuffer,
	"bodyStreamHandler":    WithBodyStreamReaderHandler,
	"session":              WithSession,
	"save":                 WithSave,
	"saveSync":             WithSaveSync,
	"saveHandler":          WithSaveHandler,
	"useMitmRule":          WithMITMRule,
	"source":               WithSource,
	"websocket":            WithWebsocket,
	"websocketFromServer":  WithWebsocketHandler,
	"websocketOnClient":    WithWebsocketClientHandler,
	"websocketStrictMode":  WithWebsocketStrictMode,
	"username":             WithUsername,
	"password":             WithPassword,
	"randomJA3":            WithRandomJA3,
	"gmTls":                WithGmTls,
	"gmTlsOnly":            WithGmTlsOnly,
	"gmTLSPrefer":          WithGmTLSPrefer,
	"fixQueryEscape":       WithFixQueryEscape,

	"json":       WithJSON,
	"body":       WithBody,
	"query":      WithQuery,
	"postData":   WithPostData,
	"postParams": WithPostParams,
	"postparams": WithPostParams,
	"header":     WithAppendHeader,
	"ua":         WithUserAgent,
	"useragent":  WithUserAgent,
	"fakeua":     WithReplaceHttpPacketRandomUserAgent,
	"uarand":     WithReplaceHttpPacketRandomUserAgent,
	"runtimeID":  WithRuntimeId,
	"fromPlugin": WithFromPlugin,
	"cookie":     WithCookieFull,

	// full options
	"replaceFirstLine":                   WithReplaceHttpPacketFirstLine,
	"replaceMethod":                      WithReplaceHttpPacketMethod,
	"replaceHeader":                      WithReplaceHttpPacketHeader,
	"replaceHost":                        WithReplaceHttpPacketHost,
	"replaceBasicAuth":                   WithReplaceHttpPacketBasicAuth,
	"replaceUserAgent":                   WithReplaceHttpPacketUserAgent,
	"replaceRandomUserAgent":             WithReplaceHttpPacketRandomUserAgent,
	"replaceCookie":                      WithReplaceHttpPacketCookie,
	"replaceCookies":                     WithReplaceHttpPacketCookies,
	"replaceBody":                        WithReplaceHttpPacketBody,
	"replaceAllQueryParams":              WithReplaceAllHttpPacketQueryParams,
	"replaceAllQueryParamsWithoutEscape": WithReplaceAllHttpPacketQueryParamsWithoutEscape,
	"replaceAllPostParams":               WithReplaceAllHttpPacketPostParams,
	"replaceAllPostParamsWithoutEscape":  WithReplaceAllHttpPacketPostParamsWithoutEscape,
	"replaceQueryParam":                  WithReplaceHttpPacketQueryParam,
	"replacePostParam":                   WithReplaceHttpPacketPostParam,
	"replacePath":                        WithReplaceHttpPacketPath,
	"replaceFormEncoded":                 WithReplaceHttpPacketFormEncoded,
	"replaceUploadFile":                  WithReplaceHttpPacketUploadFile,
	"appendHeader":                       WithAppendHeader,
	"appendHeaders":                      WithAppendHeaders,
	"appendCookie":                       WithAppendCookie,
	"appendQueryParam":                   WithAppendQueryParam,
	"appendPostParam":                    WithAppendPostParam,
	"appendPath":                         WithAppendHttpPacketPath,
	"appendFormEncoded":                  WithAppendHttpPacketFormEncoded,
	"appendUploadFile":                   WithAppendHttpPacketUploadFile,
	"deleteHeader":                       WithDeleteHeader,
	"deleteCookie":                       WithDeleteCookie,
	"deleteQueryParam":                   WithDeleteQueryParam,
	"deletePostParam":                    WithDeletePostParam,
	"deleteForm":                         WithDeleteForm,
	// random chunked
	"randomChunked":              WithEnableRandomChunked,
	"randomChunkedLength":        WithRandomChunkedLength,
	"randomChunkedDelay":         WithRandomChunkedDelay,
	"randomChunkedResultHandler": WithRandomChunkedResultHandler,

	// split
	"Split":           split,
	"FixHTTPRequest":  fixHTTPRequest,
	"FixHTTPResponse": fixHTTPResponse,

	// packet helper
	"ReplaceBody":              lowhttp.ReplaceHTTPPacketBody,
	"FixHTTPPacketCRLF":        lowhttp.FixHTTPPacketCRLF,
	"HTTPPacketForceChunked":   lowhttp.HTTPPacketForceChunked,
	"ParseBytesToHTTPRequest":  lowhttp.ParseBytesToHttpRequest,
	"ParseBytesToHTTPResponse": lowhttp.ParseBytesToHTTPResponse,
	"ParseUrlToHTTPRequestRaw": lowhttp.ParseUrlToHttpRequestRaw,

	"ReplaceHTTPPacketMethod":                      lowhttp.ReplaceHTTPPacketMethod,
	"ReplaceHTTPPacketFirstLine":                   lowhttp.ReplaceHTTPPacketFirstLine,
	"ReplaceHTTPPacketHeader":                      lowhttp.ReplaceHTTPPacketHeader,
	"ReplaceHTTPPacketBody":                        lowhttp.ReplaceHTTPPacketBodyFast,
	"ReplaceHTTPPacketJsonBody":                    lowhttp.ReplaceHTTPPacketJsonBody,
	"ReplaceHTTPPacketCookie":                      lowhttp.ReplaceHTTPPacketCookie,
	"ReplaceHTTPPacketCookies":                     lowhttp.ReplaceHTTPPacketCookies,
	"ReplaceHTTPPacketHost":                        lowhttp.ReplaceHTTPPacketHost,
	"ReplaceHTTPPacketBasicAuth":                   lowhttp.ReplaceHTTPPacketBasicAuth,
	"ReplaceAllHTTPPacketQueryParams":              lowhttp.ReplaceAllHTTPPacketQueryParams,
	"ReplaceAllHTTPPacketQueryParamsWithoutEscape": lowhttp.ReplaceAllHTTPPacketQueryParamsWithoutEscape,
	"ReplaceAllHTTPPacketPostParams":               lowhttp.ReplaceAllHTTPPacketPostParams,
	"ReplaceAllHTTPPacketPostParamsWithoutEscape":  lowhttp.ReplaceAllHTTPPacketPostParamsWithoutEscape,
	"ReplaceHTTPPacketQueryParam":                  lowhttp.ReplaceHTTPPacketQueryParam,
	"ReplaceHTTPPacketQueryParamWithoutEscape":     lowhttp.ReplaceHTTPPacketQueryParamWithoutEscape,
	"ReplaceHTTPPacketPostParam":                   lowhttp.ReplaceHTTPPacketPostParam,
	"ReplaceHTTPPacketPath":                        lowhttp.ReplaceHTTPPacketPath,
	"ReplaceHTTPPacketFormEncoded":                 lowhttp.ReplaceHTTPPacketFormEncoded,
	"ReplaceHTTPPacketUploadFile":                  lowhttp.ReplaceHTTPPacketUploadFile,
	"AppendHTTPPacketHeader":                       lowhttp.AppendHTTPPacketHeader,
	"AppendHTTPPacketCookie":                       lowhttp.AppendHTTPPacketCookie,
	"AppendHTTPPacketQueryParam":                   lowhttp.AppendHTTPPacketQueryParam,
	"AppendHTTPPacketPostParam":                    lowhttp.AppendHTTPPacketPostParam,
	"AppendHTTPPacketPath":                         lowhttp.AppendHTTPPacketPath,
	"AppendHTTPPacketFormEncoded":                  lowhttp.AppendHTTPPacketFormEncoded,
	"AppendHTTPPacketUploadFile":                   lowhttp.AppendHTTPPacketUploadFile,
	"DeleteHTTPPacketHeader":                       lowhttp.DeleteHTTPPacketHeader,
	"DeleteHTTPPacketCookie":                       lowhttp.DeleteHTTPPacketCookie,
	"DeleteHTTPPacketQueryParam":                   lowhttp.DeleteHTTPPacketQueryParam,
	"DeleteHTTPPacketPostParam":                    lowhttp.DeleteHTTPPacketPostParam,
	"DeleteHTTPPacketForm":                         lowhttp.DeleteHTTPPacketForm,

	"GetAllHTTPPacketQueryParams":     lowhttp.GetAllHTTPRequestQueryParams,
	"GetAllHTTPPacketQueryParamsFull": lowhttp.GetFullHTTPRequestQueryParams,
	"GetAllHTTPPacketPostParams":      lowhttp.GetAllHTTPRequestPostParams,
	"GetAllHTTPPacketPostParamsFull":  lowhttp.GetFullHTTPRequestPostParams,
	"GetHTTPPacketQueryParam":         lowhttp.GetHTTPRequestQueryParam,
	"GetHTTPPacketPostParam":          lowhttp.GetHTTPRequestPostParam,
	"GetHTTPPacketCookieValues":       lowhttp.GetHTTPPacketCookieValues,
	"GetHTTPPacketCookieFirst":        lowhttp.GetHTTPPacketCookieFirst,
	"GetHTTPPacketCookie":             lowhttp.GetHTTPPacketCookie,
	"GetHTTPPacketContentType":        lowhttp.GetHTTPPacketContentType,
	"GetHTTPPacketCookies":            lowhttp.GetHTTPPacketCookies,
	"GetHTTPPacketCookiesFull":        lowhttp.GetHTTPPacketCookiesFull,
	"GetHTTPPacketHeaders":            lowhttp.GetHTTPPacketHeaders,
	"GetHTTPPacketHeadersFull":        lowhttp.GetHTTPPacketHeadersFull,
	"GetHTTPPacketHeader":             lowhttp.GetHTTPPacketHeader,
	"GetHTTPPacketBody":               lowhttp.GetHTTPPacketBody,
	"GetHTTPPacketFirstLine":          lowhttp.GetHTTPPacketFirstLine,
	"GetStatusCodeFromResponse":       lowhttp.GetStatusCodeFromResponse,
	"GetHTTPRequestMethod":            lowhttp.GetHTTPRequestMethod,
	"GetHTTPRequestPath":              lowhttp.GetHTTPRequestPath,
	"ParseMultiPartFormWithCallback":  lowhttp.ParseMultiPartFormWithCallback,
	// ext for path
	"GetHTTPRequestPathWithoutQuery": lowhttp.GetHTTPRequestPathWithoutQuery,
	// extract url
	"GetUrlFromHTTPRequest": lowhttp.GetUrlFromHTTPRequest,
	// extract post params
	"ExtractPostParams": ExtractPostParams,

	"CurlToHTTPRequest": curlToHTTPRequest,
	"HTTPRequestToCurl": httpRequestToCurl,
	"IsResponse":        lowhttp.IsResp,
}
