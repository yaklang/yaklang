package lowhttp

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net/http"
	"reflect"
	"time"
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
	ConnPool                         *lowHttpConnPool
	NativeHTTPRequestInstance        *http.Request
	Username                         string
	Password                         string

	// DefaultBufferSize means unexpected situation's buffer size
	DefaultBufferSize int

	// MaxContentLength: too large content-length will be ignored(truncated)
	EnableMaxContentLength bool
	MaxContentLength       int

	// ResponseBodyMirrorWriter will be not effected by MaxContentLength
	// response body will be TeeReader to ResponseBodyMirrorWriter
	ResponseBodyMirrorWriter io.Writer
}

type LowhttpResponse struct {
	RawPacket              []byte
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
		Username:             consts.GLOBAL_HTTP_AUTH_USERNAME.Load(),
		Password:             consts.GLOBAL_HTTP_AUTH_PASSWORD.Load(),
	}
}

type LowhttpOpt func(o *LowhttpExecConfig)

func WithMaxContentLength(m int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.EnableMaxContentLength = m > 0
		o.MaxContentLength = m
	}
}

func WithResponseBodyMirrorWriter(w io.Writer) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ResponseBodyMirrorWriter = w
	}
}

func WithETCHosts(hosts map[string]string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.EtcHosts = hosts
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

func WithDNSServers(servers []string) LowhttpOpt {
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

func WithUsername(username string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Username = username
	}
}

func WithPassword(password string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Password = password
	}
}

func WithSource(s string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RequestSource = s
	}
}

func WithHost(host string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Host = host
	}
}

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

func WithTimeoutFloat(i float64) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Timeout = utils.FloatSecondDuration(i)
	}
}

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

func WithRetryNotInStatusCode(sc []int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryNotInStatusCode = sc
	}
}

func WithRetryWaitTime(retryWaitTime time.Duration) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryWaitTime = retryWaitTime
	}
}

func WithRetryMaxWaitTime(retryMaxWaitTime time.Duration) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RetryMaxWaitTime = retryMaxWaitTime
	}
}

func WithRedirectTimes(redirectTimes int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RedirectTimes = redirectTimes
	}
}

func WithJsRedirect(jsRedirect bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.JsRedirect = jsRedirect
	}
}
func WithContext(ctx context.Context) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Ctx = ctx
	}
}
func WithProxy(proxy ...string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Proxy = utils.StringArrayFilterEmpty(proxy)
	}
}

func WithForceLegacyProxy(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ForceLegacyProxy = b
	}
}

func WithSaveHTTPFlow(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.SaveHTTPFlow = b
	}
}

func WithNoFixContentLength(noFixContentLength bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.NoFixContentLength = noFixContentLength
	}
}

func WithRedirectHandler(redirectHandler func(bool, []byte, []byte) bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RedirectHandler = redirectHandler
	}
}

func WithSession(session interface{}) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Session = session
	}
}

func ConnPool(p *lowHttpConnPool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ConnPool = p
	}
}

func WithConnPool(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.WithConnPool = b
	}
}
