package lowhttp

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/netx"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/davecgh/go-spew/spew"
	utls "github.com/refraction-networking/utls"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	defaultWaitTime    = time.Duration(100) * time.Millisecond
	defaultMaxWaitTime = time.Duration(2000) * time.Millisecond
)

type RetryHandler func(https bool, retryCount int, req []byte, rsp []byte, retryFunc func(...[]byte))
type CustomFailureChecker func(https bool, req []byte, rsp []byte, fail func(string))

type LowhttpExecConfig struct {
	Host              string
	Port              int
	Packet            []byte
	VerifyCertificate bool
	Https             bool
	//ResponseCallback                 func(response *LowhttpResponse) have same option SaveHTTPFlowHandler
	Http2                            bool
	Http3                            bool
	GmTLS                            bool
	GmTLSOnly                        bool
	GmTLSPrefer                      bool
	OverrideEnableSystemProxyFromEnv bool
	EnableSystemProxyFromEnv         bool
	ConnectTimeout                   time.Duration
	Timeout                          time.Duration
	NoBodyBuffer                     bool
	RedirectTimes                    int
	RetryTimes                       int
	RetryInStatusCode                []int
	RetryNotInStatusCode             []int
	RetryHandler                     RetryHandler
	CustomFailureChecker             CustomFailureChecker
	RetryWaitTime                    time.Duration
	RetryMaxWaitTime                 time.Duration
	JsRedirect                       bool
	Proxy                            []string
	ForceLegacyProxy                 bool
	NoFixContentLength               bool
	NoReadMultiResponse              bool
	RedirectHandler                  func(bool, []byte, []byte) bool
	Session                          interface{}
	BeforeDoRequest                  func([]byte) []byte
	Ctx                              context.Context
	SaveHTTPFlow                     bool
	SaveHTTPFlowSync                 bool
	SaveHTTPFlowHandler              []func(*LowhttpResponse)
	UseMITMRule                      bool
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
	SNI *string

	// payloads (web fuzzer)
	Payloads []string

	RandomJA3FingerPrint bool
	ClientHelloSpec      *utls.ClientHelloSpec

	Tags []string

	BeforeCount *int64
	AfterCount  *int64

	Dialer func(duration time.Duration, addr string) (net.Conn, error)

	ExtendDialOption []netx.DialXOption // for test

	// random chunked
	EnableRandomChunked bool
	MinChunkedLength    int
	MaxChunkedLength    int
	MinChunkDelay       time.Duration
	MaxChunkDelay       time.Duration
	ChunkedHandler      ChunkedResultHandler
	chunkedSender       *RandomChunkedSender
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
	RequestInstance        *http.Request

	// if TooLarge, the database will drop some response data
	TooLarge         bool
	TooLargeLimit    int64
	ResponseBodySize int64

	// !deprecated
	// HiddenIndex associate between http_flows and web_fuzzer_response table
	HiddenIndex string

	// payloads (web fuzzer)
	Payloads []string

	// custom tags
	Tags []string

	IsFixContentType        bool
	OriginContentType       string
	FixContentType          string
	IsSetContentTypeOptions bool

	postParts []*multipart.Part
}

func (f *LowhttpResponse) RemoveColor() {
	newTags := make([]string, 0, len(f.Tags))
	for _, i := range f.Tags {
		if strings.HasPrefix(i, schema.COLORPREFIX) {
			continue
		}
		newTags = append(newTags, i)
	}
}

func yakitColor(i string) string {
	return schema.COLORPREFIX + i
}

func (r *LowhttpResponse) AddTag(i string) {
	r.Tags = append(r.Tags, i)
}

func (r *LowhttpResponse) AddTags(i ...string) {
	r.Tags = append(r.Tags, i...)
}

func (r *LowhttpResponse) Red() {
	r.RemoveColor()
	r.AddTag(yakitColor("RED"))
}

func (r *LowhttpResponse) Green() {
	r.RemoveColor()
	r.AddTag(yakitColor("GREEN"))
}

func (r *LowhttpResponse) Blue() {
	r.RemoveColor()
	r.AddTag(yakitColor("BLUE"))
}

func (r *LowhttpResponse) Yellow() {
	r.RemoveColor()
	r.AddTag(yakitColor("YELLOW"))
}

func (r *LowhttpResponse) Orange() {
	r.RemoveColor()
	r.AddTag(yakitColor("ORANGE"))
}

func (r *LowhttpResponse) Purple() {
	r.RemoveColor()
	r.AddTag(yakitColor("PURPLE"))
}

func (r *LowhttpResponse) Cyan() {
	r.RemoveColor()
	r.AddTag(yakitColor("CYAN"))
}

func (r *LowhttpResponse) Grey() {
	r.RemoveColor()
	r.AddTag(yakitColor("GREY"))
}

func (f *LowhttpResponse) ColorSharp(rgbHex string) {
	f.RemoveColor()
	f.AddTag(yakitColor(rgbHex))
}

func (r *LowhttpResponse) GetHeader(key string) string {
	return GetHTTPPacketHeader(r.RawPacket, key)
}

func (r *LowhttpResponse) GetHeaders() map[string]string {
	return GetHTTPPacketHeaders(r.RawPacket)
}

func (r *LowhttpResponse) GetHeadersFull() map[string][]string {
	return GetHTTPPacketHeadersFull(r.RawPacket)
}

func (r *LowhttpResponse) GetContentType() string {
	return GetHTTPPacketContentType(r.RawPacket)
}

func (r *LowhttpResponse) GetCookie(key string) string {
	return GetHTTPPacketCookie(r.RawPacket, key)
}

func (r *LowhttpResponse) GetBody() []byte {
	_, body := SplitHTTPPacketFast(r.RawPacket)
	return body
}

func (r *LowhttpResponse) GetHost() string {
	return GetHTTPPacketHeader(r.RawPacket, "Host")
}

func (r *LowhttpResponse) URL() string {
	if r.RawRequest == nil {
		return ""
	}
	scheme := "http"
	if r.Https {
		scheme = "https"
	}
	return GetUrlFromHTTPRequest(scheme, r.RawRequest)
}

func (r *LowhttpResponse) Json() any {
	if utils.IContains(r.GetHeader("Content-Type"), "json") {
		var i any
		err := json.Unmarshal(r.GetBody(), &i)
		if err != nil || utils.IsNil(i) {
			log.Warnf("json unmarshal failed: %s", err)
			return make(map[string]any)
		}
		return i
	}

	// check body
	if utils.IsNil(r.GetBody()) {
		return make(map[string]any)
	}

	// check is json
	if _, ok := utils.IsJSON(string(r.GetBody())); ok {
		var i any
		err := json.Unmarshal(r.GetBody(), &i)
		if err != nil || utils.IsNil(i) {
			log.Warnf("json unmarshal failed: %s", err)
			return make(map[string]any)
		}
		return i
	}

	return make(map[string]any)
}

func (r *LowhttpResponse) GetStatusCode() int {
	return GetStatusCodeFromResponse(r.RawPacket)
}

func (r *LowhttpResponse) GetDurationFloat() float64 {
	if r == nil {
		return 0
	}
	if r.TraceInfo == nil {
		return 0
	}
	return float64(r.TraceInfo.GetServerDurationMS()) / float64(1000)
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
	// tls 握手耗时
	TLSHandshakeTime time.Duration
	// tcp dial 耗时
	TCPTime time.Duration
}

func (l *LowhttpTraceInfo) ParseDialXTraceInfo(info *netx.DialXTraceInfo) {
	if info == nil {
		return
	}
	l.ConnTime = info.TotalTime
	l.TCPTime = info.TCPtime
	l.TLSHandshakeTime = info.TLSHandshakeTime
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
		MaxContentLength:     10 * 1024 * 1024, // 10MB roughly
	}
}

type LowhttpOpt func(o *LowhttpExecConfig)

func WithNoBodyBuffer(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.NoBodyBuffer = b
	}
}

func WithNoReadMultiResponse(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.NoReadMultiResponse = b
	}
}

func WithAppendHTTPFlowTag(tag string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Tags = append(o.Tags, tag)
	}
}

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

func WithBodyStreamReaderHandler(t func(headerBytes []byte, bodyReader io.ReadCloser)) LowhttpOpt {
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

func WithGmTLSPrefer(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.GmTLSPrefer = b
	}
}

func WithGmTLSOnly(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.GmTLSOnly = b
	}
}

// WithDNSNoCache is not effective
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

func WithHttp2(Http2 bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Http2 = Http2
	}
}

func WithHttp3(http3 bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Http3 = http3
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

func WithConnectTimeoutFloat(i float64) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ConnectTimeout = utils.FloatSecondDuration(i)
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

// WithRetryHandler sets a retry handler function that will be called when a request fails.
// return true for retry, return false for not retry.
func WithRetryHandler(retryHandler RetryHandler) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		if !utils.IsNil(retryHandler) {
			o.RetryHandler = func(https bool, retryCount int, req []byte, rsp []byte, retryFunc func(...[]byte)) {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("retry handler failed: %v\n%v", err, utils.ErrorStack(err))
					}
				}()
				retryHandler(https, retryCount, req, rsp, retryFunc)
				return
			}
		}
	}
}

// WithCustomFailureChecker sets a custom failure checker function that will be called when a request succeeds.
// The checker can call the fail function with an error message to mark the request as failed.
func WithCustomFailureChecker(customFailureChecker CustomFailureChecker) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		if !utils.IsNil(customFailureChecker) {
			o.CustomFailureChecker = func(https bool, req []byte, rsp []byte, fail func(string)) {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("custom failure checker failed: %v\n%v", err, utils.ErrorStack(err))
					}
				}()
				customFailureChecker(https, req, rsp, fail)
			}
		}
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

func WithSaveHTTPFlow(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.SaveHTTPFlow = b
	}
}

func WithSaveHTTPFlowSync(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.SaveHTTPFlowSync = b
	}
}

func WithSaveHTTPFlowHandler(f ...func(*LowhttpResponse)) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		if o.SaveHTTPFlowHandler == nil {
			o.SaveHTTPFlowHandler = make([]func(*LowhttpResponse), 0, 1)
		}
		o.SaveHTTPFlowHandler = append(o.SaveHTTPFlowHandler, f...)
	}
}

func WithUseMITMRule(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.UseMITMRule = b
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

func ConnPool(p *LowHttpConnPool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ConnPool = p
	}
}

func WithConnPool(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.WithConnPool = b
	}
}

func WithDebugCount(before, after *int64) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.BeforeCount = before
		o.AfterCount = after
	}
}

func WithSNI(sni string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.SNI = &sni
	}
}

func WithPayloads(payloads []string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Payloads = payloads
	}
}

func WithRandomJA3FingerPrint(b bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.RandomJA3FingerPrint = b
	}
}

func WithClientHelloSpec(spec *utls.ClientHelloSpec) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ClientHelloSpec = spec
	}
}

func WithDialer(dialer func(duration time.Duration, addr string) (net.Conn, error)) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Dialer = dialer
	}
}

func WithExtendDialXOption(options ...netx.DialXOption) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ExtendDialOption = options
	}
}

func WithEnableRandomChunked(enable bool) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.EnableRandomChunked = enable
	}
}

func WithRandomChunkedLength(min, max int) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.MinChunkedLength = min
		o.MaxChunkedLength = max
	}
}

func WithRandomChunkedDelay(min, max time.Duration) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.MinChunkDelay = min
		o.MaxChunkDelay = max
	}
}

func WithRandomChunkedHandler(handler ChunkedResultHandler) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.ChunkedHandler = handler
	}
}

func (o *LowhttpExecConfig) GetOrCreateChunkSender() (*RandomChunkedSender, error) {
	if o.chunkedSender != nil {
		return o.chunkedSender, nil
	}
	options := []randomChunkedHTTPOption{
		_withRandomChunkCtx(o.Ctx),
		_withRandomChunkChunkLength(o.MinChunkedLength, o.MaxChunkedLength),
		_withRandomChunkDelay(o.MinChunkDelay, o.MaxChunkDelay),
		_withRandomChunkResultHandler(o.ChunkedHandler),
	}
	sender, err := NewRandomChunkedSender(
		options...,
	)
	if err != nil {
		return nil, err
	}
	o.chunkedSender = sender
	return sender, nil
}
