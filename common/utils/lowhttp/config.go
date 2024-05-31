package lowhttp

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	defaultWaitTime    = time.Duration(100) * time.Millisecond
	defaultMaxWaitTime = time.Duration(2000) * time.Millisecond
)

type LowhttpExecConfig struct {
	Host                             string
	Port                             int
	Packet                           []byte
	VerifyCertificate                bool
	Https                            bool
	ResponseCallback                 func(response *LowhttpResponse)
	Http2                            bool
	GmTLS                            bool
	OverrideEnableSystemProxyFromEnv bool
	EnableSystemProxyFromEnv         bool
	ConnectTimeout                   time.Duration
	Timeout                          time.Duration
	RedirectTimes                    int
	RetryTimes                       int
	RetryInStatusCode                []int
	RetryNotInStatusCode             []int
	RetryWaitTime                    time.Duration
	RetryMaxWaitTime                 time.Duration
	JsRedirect                       bool
	Proxy                            []string
	ForceLegacyProxy                 bool
	NoFixContentLength               bool
	RedirectHandler                  func(bool, []byte, []byte) bool
	Session                          interface{}
	BeforeDoRequest                  func([]byte) []byte
	Ctx                              context.Context
	SaveHTTPFlow                     bool
	RequestSource                    string
	EtcHosts                         map[string]string
	DNSServers                       []string
	RuntimeId                        string
	FromPlugin                       string
	WithConnPool                     bool
	ConnPool                         *LowHttpConnPool
	NativeHTTPRequestInstance        *http.Request
	Username                         string
	Password                         string

	// DefaultBufferSize means unexpected situation's buffer size
	DefaultBufferSize int

	// MaxContentLength: too large content-length will be ignored(truncated)
	EnableMaxContentLength bool
	MaxContentLength       int

	DNSNoCache bool

	// BodyStreamReaderHandler is a callback function to handle the body stream reader
	BodyStreamReaderHandler func(responseHeader []byte, closer io.ReadCloser)

	// SNI
	SNI string

	// payloads (web fuzzer)
	Payloads []string
}

type LowhttpResponse struct {
	RawPacket              []byte
	BareResponse           []byte
	RedirectRawPackets     []*RedirectFlow
	PortIsOpen             bool
	TraceInfo              *LowhttpTraceInfo
	Url                    string
	RemoteAddr             string
	Proxy                  string
	Https                  bool
	Http2                  bool
	RawRequest             []byte
	Source                 string // 请求源
	RuntimeId              string
	FromPlugin             string
	MultiResponse          bool
	MultiResponseInstances []*http.Response

	// if TooLarge, the database will drop some response data
	TooLarge         bool
	TooLargeLimit    int64
	ResponseBodySize int64

	// !deprecated
	// HiddenIndex associate between http_flows and web_fuzzer_response table
	HiddenIndex string

	// payloads (web fuzzer)
	Payloads []string
}

func (l *LowhttpResponse) GetBody() []byte {
	_, body := SplitHTTPPacketFast(l.RawPacket)
	return body
}

func (l *LowhttpResponse) GetDurationFloat() float64 {
	if l == nil {
		return 0
	}
	if l.TraceInfo == nil {
		return 0
	}
	return float64(l.TraceInfo.GetServerDurationMS()) / float64(1000)
}

type LowhttpTraceInfo struct {
	AvailableDNSServers []string
	// DNS 完整请求时间
	DNSTime time.Duration
	// 获取一个连接的耗时
	ConnTime time.Duration
	// 服务器处理耗时，计算从连接建立到客户端收到第一个字节的时间间隔
	ServerTime time.Duration
	// 完整请求的耗时
	TotalTime time.Duration
}

func (l *LowhttpTraceInfo) GetServerDurationMS() int64 {
	if l == nil {
		return 0
	}
	return l.ServerTime.Milliseconds()
}

func newLowhttpResponse(trace *LowhttpTraceInfo) *LowhttpResponse {
	return &LowhttpResponse{
		RawPacket:  nil,
		PortIsOpen: false,
		TraceInfo:  trace,
	}
}

func newLowhttpTraceInfo() *LowhttpTraceInfo {
	return &LowhttpTraceInfo{}
}

// NewLowhttpOption create a new LowhttpExecConfig
func NewLowhttpOption() *LowhttpExecConfig {
	return &LowhttpExecConfig{
		Host:                 "",
		Port:                 0,
		Packet:               []byte{},
		Https:                false,
		Http2:                false,
		Timeout:              15 * time.Second,
		ConnectTimeout:       15 * time.Second,
		RetryTimes:           0,
		RetryInStatusCode:    []int{},
		RetryNotInStatusCode: []int{},
		RetryWaitTime:        defaultWaitTime,
		RetryMaxWaitTime:     defaultMaxWaitTime,
		RedirectTimes:        5,
		Proxy:                nil,
		RedirectHandler:      nil,
		SaveHTTPFlow:         consts.GLOBAL_HTTP_FLOW_SAVE.IsSet(),
		MaxContentLength:     10 * 1000 * 1000, // 10MB roughly
	}
}

type LowhttpOpt func(o *LowhttpExecConfig)

func WithMaxContentLength(m int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.EnableMaxContentLength = m > 0
		o.MaxContentLength = m
	}
}

func WithETCHosts(hosts map[string]string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.EtcHosts = hosts
	}
}

// stream 是一个请求选项参数，可以设置一个回调函数，如果 body 读取了，将会复制一份给这个流，在这个流中处理 body 是不会影响最终结果的，一般用于处理较长的 chunk 数据
func WithBodyStreamReaderHandler(t func([]byte, io.ReadCloser)) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.BodyStreamReaderHandler = t
	}
}

func WithVerifyCertificate(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.VerifyCertificate = b
	}
}

func WithGmTLS(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.GmTLS = b
	}
}

// dnsNoCache 是一个请求选项参数，用于指定请求时不使用DNS缓存，默认使用DNS缓存
// Example:
// ```
// // 向 example.com 发起请求，不使用DNS缓存
// poc.Get("https://exmaple.com", poc.dnsNoCache(true))
// ```
func WithDNSNoCache(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.DNSNoCache = b
		log.Debug("WithDNSNoCache is not effective")
	}
}

func WithDNSServers(servers []string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.DNSServers = servers
	}
}

// dnsServer 是一个请求选项参数，用于指定请求所使用的DNS服务器，默认使用系统自带的DNS服务器
// Example:
// ```
// // 向 example.com 发起请求，使用指定的DNS服务器
// poc.Get("https://exmaple.com", poc.dnsServer("8.8.8.8", "1.1.1.1"))
// ```
func WithExportedDNSServers(servers ...string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.DNSServers = servers
	}
}

func WithRuntimeId(runtimeId string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RuntimeId = runtimeId
	}
}

func WithFromPlugin(fromPlugin string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.FromPlugin = fromPlugin
	}
}

func WithNativeHTTPRequestInstance(req *http.Request) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.NativeHTTPRequestInstance = req
	}
}

// username 是一个请求选项参数，用于指定认证时的用户名
// Example:
// ```
// poc.Get("https://www.example.com", poc.username("admin"), poc.password("admin"))
// ```
func WithUsername(username string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Username = username
	}
}

// password 是一个请求选项参数，用于指定认证时的密码
// Example:
// ```
// poc.Get("https://www.example.com", poc.username("admin"), poc.password("admin"))
// ```
func WithPassword(password string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Password = password
	}
}

// source 是一个请求选项参数，用于在请求记录保存到数据库时标识此次请求的来源
// Example:
// ```
// poc.Get("https://exmaple.com", poc.save(true), poc.source("test")) // 向 example.com 发起请求，会将此次请求保存到数据库中，指示此次请求的来源为test
// ```
func WithSource(s string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RequestSource = s
	}
}

// host 是一个请求选项参数，用于指定实际请求的 host，如果没有设置该请求选项，则会依据原始请求报文中的Host字段来确定实际请求的host
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.host("yaklang.com")) // 实际上请求 yaklang.com
// ```
func WithHost(host string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Host = host
	}
}

// port 是一个请求选项参数，用于指定实际请求的端口，如果没有设置该请求选项，则会依据原始请求报文中的Host字段来确定实际请求的端口
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.host("yaklang.com"), poc.port(443), poc.https(true)) // 实际上请求 yaklang.com 的443端口
// ```
func WithPort(port int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Port = port
	}
}

func WithPacketBytes(packet []byte) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Packet = packet
	}
}

func WithDefaultBufferSize(size int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.DefaultBufferSize = size
	}
}

func WithEnableSystemProxyFromEnv(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.OverrideEnableSystemProxyFromEnv = true
		o.EnableSystemProxyFromEnv = b
	}
}

func WithRequest(packet any) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		switch ret := packet.(type) {
		case []byte:
			o.Packet = ret
			return
		case string:
			o.Packet = []byte(ret)
			return
		case *http.Request:
			reqRaw, err := utils.HttpDumpWithBody(ret, true)
			if err != nil {
				log.Errorf("parse request failed: %s", err)
			}
			o.Packet = reqRaw
		default:
			o.Packet = utils.InterfaceToBytes(packet)
			log.Warnf("any(%v) to request packet: %s", reflect.TypeOf(ret), spew.Sdump(o.Packet))
		}
	}
}

func WithBeforeDoRequest(h func([]byte) []byte) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.BeforeDoRequest = h
	}
}

// https 是一个请求选项参数，用于指定是否使用 https 协议，默认为 false 即使用 http 协议
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.https(true)) // 向 example.com 发起请求，使用 https 协议
// ```
func WithHttps(https bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Https = https
	}
}

func WithResponseCallback(h func(i *LowhttpResponse)) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ResponseCallback = h
	}
}

// http2 是一个请求选项参数，用于指定是否使用 http2 协议，默认为 false 即使用http1协议
// Example:
// ```
// poc.Get("https://www.example.com", poc.http2(true), poc.https(true)) // 向 www.example.com 发起请求，使用 http2 协议
// ```
func WithHttp2(Http2 bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Http2 = Http2
	}
}

func WithTimeout(timeout time.Duration) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Timeout = timeout
	}
}

func WithConnectTimeout(timeout time.Duration) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ConnectTimeout = timeout
	}
}

// connectTimeout 是一个请求选项参数，用于指定连接超时时间，默认为15秒
// Example:
// ```
// poc.Get("https://www.example.com", poc.timeout(15)) // 向 www.baidu.com 发起请求，读取超时时间为15秒
// ```
func WithConnectTimeoutFloat(i float64) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ConnectTimeout = utils.FloatSecondDuration(i)
	}
}

// timeout 是一个请求选项参数，用于指定读取超时时间，默认为15秒
// Example:
// ```
// poc.Get("https://www.example.com", poc.timeout(15)) // 向 www.baidu.com 发起请求，读取超时时间为15秒
// ```
func WithTimeoutFloat(i float64) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Timeout = utils.FloatSecondDuration(i)
	}
}

// retryTimes 是一个请求选项参数，用于指定请求失败时的重试次数，需要搭配 retryInStatusCode 或 retryNotInStatusCode 使用，来设置在什么响应码的情况下重试
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.retryTimes(5), poc.retryInStatusCode(500, 502)) // 向 example.com 发起请求，如果响应状态码500或502则进行重试，最多进行5次重试
// ```
func WithRetryTimes(retryTimes int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryTimes = retryTimes
	}
}

func WithRetryInStatusCode(sc []int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryInStatusCode = sc
	}
}

// retryInStatusCode 是一个请求选项参数，用于指定在某些响应状态码的情况下重试，需要搭配 retryTimes 使用
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.retryTimes(5), poc.retryInStatusCode(500, 502)) // 向 example.com 发起请求，如果响应状态码500或502则进行重试，最多进行5次重试
// ```
func WithRetryInStatusCodes(sc ...int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryInStatusCode = sc
	}
}

func WithRetryNotInStatusCode(sc []int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryNotInStatusCode = sc
	}
}

// retryNotInStatusCode 是一个请求选项参数，用于指定非某些响应状态码的情况下重试，需要搭配 retryTimes 使用
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.retryTimes(5), poc.retryNotInStatusCode(200)) // 向 example.com 发起请求，如果响应状态码不等于200则进行重试，最多进行5次重试
// ```
func WithRetryNotInStatusCodes(sc ...int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryNotInStatusCode = sc
	}
}

func WithRetryWaitTime(retryWaitTime time.Duration) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryWaitTime = retryWaitTime
	}
}

// retryWaitTime 是一个请求选项参数，用于指定重试时最小等待时间，需要搭配 retryTimes 使用，默认为0.1秒
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.retryTimes(5), poc.retryNotInStatusCode(200), poc.retryWaitTime(0.1)) // 向 example.com 发起请求，如果响应状态码不等于200则进行重试，最多进行5次重试，重试时最小等待0.1秒
// ```
func WithRetryWaitTimeFloat(i float64) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryWaitTime = utils.FloatSecondDuration(i)
	}
}

func WithRetryMaxWaitTime(retryMaxWaitTime time.Duration) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryMaxWaitTime = retryMaxWaitTime
	}
}

// retryMaxWaitTime 是一个请求选项参数，用于指定重试时最大等待时间，需要搭配 retryTimes 使用，默认为2秒
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.retryTimes(5), poc.retryNotInStatusCode(200), poc.retryWaitTime(2)) // 向 example.com 发起请求，如果响应状态码不等于200则进行重试，最多进行5次重试，重试时最多等待2秒
// ```
func WithRetryMaxWaitTimeFloat(i float64) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryMaxWaitTime = utils.FloatSecondDuration(i)
	}
}

// noRedirect 是一个请求选项参数，用于指定是否跟踪重定向，默认为 false 即会自动跟踪重定向
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.noRedirect()) // 向 example.com 发起请求，如果响应重定向到其他链接也不会自动跟踪重定向
// ```
func WithNoRedirect(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RedirectTimes = 0
	}
}

// redirectTimes 是一个请求选项参数，用于指定最大重定向次数，默认为5次
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.redirectTimes(5)) // 向 example.com 发起请求，如果响应重定向到其他链接，则会自动跟踪重定向最多5次
// ```
func WithRedirectTimes(redirectTimes int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RedirectTimes = redirectTimes
	}
}

// jsRedirect 是一个请求选项参数，用于指定是否跟踪JS重定向，默认为false即不会自动跟踪JS重定向
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.redirectTimes(5), poc.jsRedirect(true)) // 向 www.baidu.com 发起请求，如果响应重定向到其他链接也会自动跟踪JS重定向，最多进行5次重定向
// ```
func WithJsRedirect(jsRedirect bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.JsRedirect = jsRedirect
	}
}

// context 是一个请求选项参数，用于指定请求的上下文
// Example:
// ```
// ctx = context.New()
// poc.Get("https://exmaple.com", poc.withContext(ctx)) // 向 example.com 发起请求，使用指定的上下文
// ```
func WithContext(ctx context.Context) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Ctx = ctx
	}
}

// proxy 是一个请求选项参数，用于指定请求使用的代理，可以指定多个代理，默认会使用系统代理
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.proxy("http://127.0.0.1:7890")) // 向 example.com 发起请求，使用 http://127.0.0.1:7890 代理
// ```
func WithProxy(proxy ...string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Proxy = utils.StringArrayFilterEmpty(proxy)
	}
}

func WithProxyGetter(getter func() []string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Proxy = utils.StringArrayFilterEmpty(getter())
	}
}

func WithForceLegacyProxy(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ForceLegacyProxy = b
	}
}

// save 是一个请求选项参数，用于指定是否将此次请求的记录保存在数据库中，默认为true即会保存到数据库
// Example:
// ```
// poc.Get("https://exmaple.com", poc.save(true)) // 向 example.com 发起请求，会将此次请求保存到数据库中
// ```
func WithSaveHTTPFlow(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.SaveHTTPFlow = b
	}
}

// noFixContentLength 是一个请求选项参数，用于指定是否修复响应报文中的 Content-Length 字段，默认为 false 即会自动修复Content-Length字段
// Example:
// ```
// poc.HTTP(poc.BasicRequest(), poc.noFixContentLength()) // 向 example.com 发起请求，如果响应报文中的Content-Length字段不正确或不存在	也不会自动修复
// ```
func WithNoFixContentLength(noFixContentLength bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.NoFixContentLength = noFixContentLength
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
func WithRedirectHandler(redirectHandler func(bool, []byte, []byte) bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RedirectHandler = redirectHandler
	}
}

// session 是一个请求选项参数，用于指定请求的session，参数可以是任意类型的值，用此值做标识符从而找到唯一的session。使用session进行请求时会自动管理cookie，这在登录后操作的场景非常有用
// Example:
// ```
// poc.Get("https://pie.dev/cookies/set/AAA/BBB", poc.session("test")) // 向 pie.dev 发起第一次请求，这会设置一个名为AAA，值为BBB的cookie
// rsp, req, err = poc.Get("https://pie.dev/cookies", poc.session("test")) // 向 pie.dev 发起第二次请求，这个请求会输出所有的cookies，可以看到第一次请求设置的cookie已经存在了
// ```
func WithSession(session interface{}) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Session = session
	}
}

func ConnPool(p *LowHttpConnPool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ConnPool = p
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
func WithConnPool(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.WithConnPool = b
	}
}

// sni 是一个请求选项参数，用于指定使用 tls(https) 协议时的 服务器名称指示(SNI)
// Example:
// ```
// poc.Get("https://www.example.com", poc.sni("google.com"))
// ```
func WithSNI(sni string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.SNI = sni
	}
}

func WithPayloads(payloads []string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Payloads = payloads
	}
}
