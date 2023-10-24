package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
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
}

type LowhttpResponse struct {
	RawPacket          []byte
	RedirectRawPackets []*RedirectFlow
	PortIsOpen         bool
	TraceInfo          *LowhttpTraceInfo
	Url                string
	RemoteAddr         string
	Proxy              string
	Https              bool
	Http2              bool
	RawRequest         []byte
	Source             string // 请求源
	RuntimeId          string
	FromPlugin         string
	MultiResponse      bool
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
	}
}

type LowhttpOpt func(o *LowhttpExecConfig)

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

var (
	_systemEtcHosts = make(map[string]string)
	systemEtcOnce   = sync.Once{}
)

func GetSystemHostByName(domain string) (string, bool) {
	systemEtcOnce.Do(func() {
		_systemEtcHosts = GetSystemEtcHosts()
	})
	raw, ok := _systemEtcHosts[domain]
	return raw, ok
}

type RedirectFlow struct {
	IsHttps    bool
	Request    []byte
	Response   []byte
	RespRecord *LowhttpResponse
}

func HTTP(opts ...LowhttpOpt) (*LowhttpResponse, error) {
	option := NewLowhttpOption()
	for _, opt := range opts {
		opt(option)
	}
	if option.WithConnPool && option.ConnPool == nil {
		option.ConnPool = DefaultLowHttpConnPool
	}

	var (
		forceHttps         = option.Https
		r                  = option.Packet
		redirectTimes      = option.RedirectTimes
		redirectHandler    = option.RedirectHandler
		jsRedirect         = option.JsRedirect
		redirectRawPackets []*RedirectFlow
		response           *LowhttpResponse
		err                error
	)

	response, err = HTTPWithoutRedirect(opts...)
	if err != nil {
		return response, err
	}
	raw := &RedirectFlow{
		IsHttps:    response.Https,
		Request:    response.RawRequest,
		Response:   response.RawPacket,
		RespRecord: response,
	}
	if raw != nil {
		redirectRawPackets = append(redirectRawPackets, raw)
	}

	if redirectTimes > 0 {
		lastPacket := raw
		for i := 0; i < redirectTimes; i++ {
			target := GetRedirectFromHTTPResponse(lastPacket.Response, jsRedirect)
			if target == "" {
				response.RedirectRawPackets = redirectRawPackets
				return response, nil
			}

			// 当跳转地址携带协议头时,强制更新forceHttps状态，自动升降级
			if strings.HasPrefix(strings.TrimSpace(target), "http://") {
				forceHttps = false
			} else if strings.HasPrefix(strings.TrimSpace(target), "https://") {
				forceHttps = true
			}

			if redirectHandler != nil {
				if !redirectHandler(forceHttps, r, lastPacket.Response) {
					break
				}
			}

			targetUrl := MergeUrlFromHTTPRequest(r, target, forceHttps)

			r = UrlToGetRequestPacketWithResponse(targetUrl, r, lastPacket.Response, forceHttps, ExtractCookieJarFromHTTPResponse(lastPacket.Response)...)
			nextHost, nextPort, _ := utils.ParseStringToHostPort(targetUrl)
			log.Debugf("[lowhttp] redirect to: %s", targetUrl)

			newOpts := append(opts, WithHttps(forceHttps), WithHost(nextHost), WithPort(nextPort), WithRequest(r))
			response, err = HTTPWithoutRedirect(newOpts...)
			if err != nil {
				log.Errorf("met error in redirect...: %s", err)
				response.RawPacket = lastPacket.Response // 保留原始报文
				return response, nil
			}
			responseRaw := &RedirectFlow{
				IsHttps:    response.Https,
				Request:    response.RawRequest,
				Response:   response.RawPacket,
				RespRecord: response,
			}
			if responseRaw == nil {
				response.RawPacket = lastPacket.Response // 保留原始报文
				return response, nil
			}
			redirectRawPackets = append(redirectRawPackets, responseRaw)
			response.RedirectRawPackets = redirectRawPackets

			// raw
			lastPacket = responseRaw
		}
	}

	return response, nil
}

var commonHTTPMethod = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodDelete:  {},
	http.MethodPatch:   {},
	http.MethodHead:    {},
	http.MethodOptions: {},
	http.MethodConnect: {},
	http.MethodTrace:   {},
}

// HTTPWithoutRedirect SendHttpRequestWithRawPacketWithOpt
func HTTPWithoutRedirect(opts ...LowhttpOpt) (*LowhttpResponse, error) {
	option := NewLowhttpOption()
	for _, opt := range opts {
		opt(option)
	}

	var (
		forceHttps           = option.Https
		forceHttp2           = option.Http2
		gmTLS                = option.GmTLS
		host                 = option.Host
		port                 = option.Port
		requestPacket        = option.Packet
		timeout              = option.Timeout
		retryTimes           = option.RetryTimes
		retryInStatusCode    = option.RetryInStatusCode
		retryNotInStatusCode = option.RetryNotInStatusCode
		retryWaitTime        = option.RetryWaitTime
		retryMaxWaitTime     = option.RetryMaxWaitTime
		noFixContentLength   = option.NoFixContentLength
		proxy                = option.Proxy
		saveHTTPFlow         = option.SaveHTTPFlow
		session              = option.Session
		ctx                  = option.Ctx
		traceInfo            = newLowhttpTraceInfo()
		response             = newLowhttpResponse(traceInfo)
		source               = option.RequestSource
		dnsServers           = option.DNSServers
		dnsHosts             = option.EtcHosts
		connPool             = option.ConnPool
		withConnPool         = option.WithConnPool
	)

	if option.WithConnPool && option.ConnPool == nil {
		option.ConnPool = DefaultLowHttpConnPool
		connPool = DefaultLowHttpConnPool
	}

	reqSchema := "HTTP"
	if forceHttp2 {
		reqSchema = "HTTP2"
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if response.Source == "" {
		response.Source = source
	}

	defer func() {
		if response == nil || !saveHTTPFlow {
			return
		}

		// 保存 http flow
		log.Debugf("should save url: %v", response.Url)
		saveCtx, cancel := context.WithCancel(ctx)

		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("save response panic! reason: %v", err)
				}
				cancel()
			}()
			if option.ResponseCallback != nil {
				option.ResponseCallback(response)
			}
			SaveResponse(response)
		}()
		select {
		case <-saveCtx.Done():
		}
	}()
	var newProxy []string
	for _, p := range proxy {
		i, err := url.Parse(p)
		if err != nil {
			continue
		}
		if i.Hostname() == "" {
			continue
		}
		newProxy = append(newProxy, p)
	}
	proxy = newProxy

	var forceProxy = len(proxy) > 0
	var legacyProxy []string
	if option.ForceLegacyProxy {
		var ordinaryProxy []string
		lo.ForEach(proxy, func(i string, idx int) {
			if utils.IsHttpOrHttpsUrl(i) {
				legacyProxy = append(legacyProxy, i)
			} else {
				ordinaryProxy = append(ordinaryProxy, i)
			}
		})
		proxy = ordinaryProxy
	}

	https := forceHttps

	if gmTLS {
		https = true
	}

	/*
	   extract url
	*/
	var forceOverrideURL string
	var urlBuf bytes.Buffer
	if https {
		urlBuf.WriteString("https://")
	} else {
		urlBuf.WriteString("http://")
	}
	var requestURI string
	var hostInPacket string
	var haveTE bool
	var haveCL bool
	var clInt int
	var enableHttp2 = false
	_, originBody := SplitHTTPHeadersAndBodyFromPacketEx(requestPacket, func(method string, uri string, proto string) error {
		requestURI = uri
		if strings.HasPrefix(proto, "HTTP/2") || forceHttp2 {
			enableHttp2 = true
		}
		if utils.IsHttpOrHttpsUrl(requestURI) {
			forceOverrideURL = requestURI
		}
		return nil
	}, func(line string) {
		key, value := SplitHTTPHeader(line)
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if strings.ToLower(key) == "host" {
			hostInPacket = value
		}
		if !haveTE && strings.ToLower(key) == "transfer-encoding" {
			haveTE = true
		}
		if !haveCL && strings.ToLower(key) == "content-length" {
			haveCL = true
			clInt = codec.Atoi(value)
		}
	})
	if hostInPacket == "" && host == "" {
		return response, utils.Errorf("host not found in packet and option (Check your `Host: ` header)")
	}

	var urlStr = forceOverrideURL
	var noURI string
	if urlStr == "" {
		if hostInPacket != "" {
			urlBuf.WriteString(hostInPacket)
		} else {
			urlBuf.WriteString(host)
			if (https && port != 443) || (!https && port != 80) {
				urlBuf.WriteString(fmt.Sprintf(":%d", port))
			}
		}
		noURI = urlBuf.String()
		if requestURI == "" {
			urlBuf.WriteString("/")
		} else {
			if !strings.HasPrefix(requestURI, "/") {
				urlBuf.WriteString("/")
			}
			urlBuf.WriteString(utils.EscapeInvalidUTF8Byte([]byte(requestURI)))
		}
		urlStr = urlBuf.String()
	}

	urlIns, err := url.Parse(urlStr)
	if err != nil {
		urlIns = utils.ParseStringToUrl(noURI)
		//urlIns, err = url.Parse(noURI)
		//if err != nil {
		//	return nil, utils.Errorf(`parse url %#v failed: %s`, urlStr, err)
		//}
	}

	/*
		checking pipeline or smuggle
	*/
	if haveTE && haveCL {
		log.Infof("request \n%v\n have both `Transfer-Encoding` and `Content-Length` header, maybe pipeline or smuggle, auto enable noFixContentLength", spew.Sdump(requestPacket))
		//noFixContentLength = true
	} else if haveCL && !haveTE && len(originBody) > clInt {
		SplitHTTPPacket(originBody[clInt:], func(method string, requestUri string, proto string) error {
			if ret := len(proto); ret > 5 && ret <= 8 && strings.HasPrefix(proto, "HTTP/") && proto[5] >= '0' && proto[5] <= '9' {
				if _, ok := commonHTTPMethod[method]; ok {
					noFixContentLength = true
				}
			}
			return utils.Error("pipeline or smuggle detected, auto enable noFixContentLength")
		}, nil)
	} else if haveTE && !haveCL {
		// have transfer-encoding and no cl!
		var body, nextPacket = codec.HTTPChunkedDecodeWithRestBytes(originBody)
		_ = body
		if len(nextPacket) > 0 {
			SplitHTTPPacket(nextPacket, func(method string, requestUri string, proto string) error {
				if ret := len(proto); ret > 5 && ret <= 8 && strings.HasPrefix(proto, "HTTP/") && proto[5] >= '0' && proto[5] <= '9' {
					if _, ok := commonHTTPMethod[method]; ok {
						//noFixContentLength = true
					}
				}
				return utils.Error("pipeline or smuggle detected, auto enable noFixContentLength")
			}, nil)
		}
	}

	// 逐个记录 response 中的内容
	response.Url = urlIns.String()

	// 获取cookiejar
	cookiejar := GetCookiejar(session)
	if session != nil {
		cookies := cookiejar.Cookies(urlIns)

		// 复用session中的cookie
		requestPacket, err = AddOrUpgradeCookie(requestPacket, CookiesToString(cookies))
		if err != nil {
			return response, err
		}
	}

	// 修复 host port
	if port <= 0 || host == "" {
		newHost, newPort, err := utils.ParseStringToHostPort(urlIns.String())
		if err != nil {
			return response, err
		}

		if port <= 0 {
			port = newPort
		}

		if host == "" {
			host = newHost
		}
	}

	if port <= 0 {
		return response, utils.Errorf("empty port...")
	}
	var originAddr = utils.HostPort(host, port)

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	if retryTimes < 0 {
		retryTimes = 0
	}

	response.TraceInfo.AvailableDNSServers = dnsServers
	response.RuntimeId = option.RuntimeId
	response.FromPlugin = option.FromPlugin

	// fix CRLF
	requestPacket = FixHTTPPacketCRLF(requestPacket, noFixContentLength)
	response.RawRequest = requestPacket
	response.Http2 = enableHttp2

	//https://github.com/mattn/go-ieproxy
	var (
		conn                 net.Conn
		retry                int
		statusCodeRetryTimes int = 0
	)
	if len(proxy) == 1 && proxy[0] == "" {
		proxy = proxy[1:]
	}

	totalTimeStart := time.Now()
	defer func() {
		traceInfo.TotalTime = time.Since(totalTimeStart)
	}()

	// h2
	var nextProto []string
	if enableHttp2 {
		nextProto = []string{http2.NextProtoTLS}
	} else {
		nextProto = []string{"http/1.1"}
	}

	// 需要用于标识连接 https gmTLS
	// configTLS
	var dialopts []netx.DialXOption
	if https {
		if gmTLS {
			dialopts = append(dialopts, netx.DialX_WithGMTLSConfig(&gmtls.Config{
				GMSupport:          &gmtls.GMSupport{WorkMode: gmtls.ModeAutoSwitch},
				NextProtos:         nextProto,
				ServerName:         host,
				InsecureSkipVerify: !option.VerifyCertificate,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
			}))
		} else {
			dialopts = append(dialopts, netx.DialX_WithTLSConfig(&tls.Config{
				NextProtos:         nextProto,
				ServerName:         host,
				InsecureSkipVerify: !option.VerifyCertificate,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
			}))
		}
		dialopts = append(dialopts, netx.DialX_WithGMTLSSupport(gmTLS), netx.DialX_WithTLS(https))
	}

	if forceProxy {
		dialopts = append(dialopts, netx.DialX_WithForceProxy(forceProxy))
	}

	if len(proxy) > 0 {
		dialopts = append(dialopts, netx.DialX_WithProxy(proxy...))
	}

	// 初次连接需要的
	// retry use DialX
	var dnsStart = time.Now()
	var dnsEnd = time.Now()
	dialopts = append(
		dialopts,
		netx.DialX_WithTimeoutRetry(retryTimes),
		netx.DialX_WithTimeoutRetryWaitRange(
			retryWaitTime,
			retryMaxWaitTime,
		),
		netx.DialX_WithDNSOptions(
			netx.WithDNSOnFinished(func() {
				dnsEnd = time.Now()
			}),
			netx.WithDNSServers(dnsServers...),
			netx.WithTemporaryHosts(dnsHosts),
		),
		netx.DialX_WithSNI(host),
	)

	if option.OverrideEnableSystemProxyFromEnv {
		dialopts = append(dialopts, netx.DialX_WithEnableSystemProxyFromEnv(option.EnableSystemProxyFromEnv))
	}

	cacheKey := connectKey{
		proxy:  proxy,
		scheme: reqSchema,
		addr:   originAddr,
		https:  option.Https,
		gmTls:  option.GmTLS,
	}
RECONNECT:
	if withConnPool && !enableHttp2 {
		conn, err = connPool.getIdleConn(cacheKey, dialopts...)
	} else {
		conn, err = netx.DialX(originAddr, dialopts...)
	}
	traceInfo.DNSTime = dnsEnd.Sub(dnsStart) // safe
	response.Https = https

	// checking old proxy
	oldVersionProxyChecking := false
	var tryOldVersionProxy []string
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, `no proxy available`) {
			noProxyDial := make([]netx.DialXOption, len(dialopts), len(dialopts)+1)
			copy(noProxyDial, dialopts)
			noProxyDial = append(noProxyDial, netx.DialX_WithDisableProxy(true))
			var tried = make(map[string]struct{})
			var merged = make([]string, len(legacyProxy)+len(proxy))
			copy(merged, legacyProxy)
			copy(merged[len(legacyProxy):], proxy)
			for _, basicProxy := range lo.Filter(merged, func(item string, index int) bool {
				return utils.IsHttpOrHttpsUrl(item)
			}) {
				if _, ok := tried[basicProxy]; ok {
					continue
				} else {
					tried[basicProxy] = struct{}{}
				}
				if withConnPool {
					cacheKey.addr = utils.ExtractHostPort(basicProxy)
					conn, err = connPool.getIdleConn(cacheKey, noProxyDial...)
				} else {
					conn, err = netx.DialX(utils.ExtractHostPort(basicProxy), noProxyDial...)
				}
				if err != nil {
					log.Debugf("try old version proxy failed: %s", err)
					continue
				}
				oldVersionProxyChecking = true
				enableHttp2 = false
				tryOldVersionProxy = append(tryOldVersionProxy, basicProxy)
				break
			}
		}

		if utils.IsNil(conn) {
			return response, err
		}
	}
	response.RemoteAddr = conn.RemoteAddr().String()
	response.PortIsOpen = true

	if enableHttp2 {
		//ddl, cancel := context.WithTimeout(context.Background(), timeout)
		//defer cancel()
		if option.BeforeDoRequest != nil {
			requestPacket = option.BeforeDoRequest(requestPacket)
		}
		rsp, err := HTTPRequestToHTTP2("https", utils.HostPort(host, port), conn, requestPacket, noFixContentLength)
		if err != nil {
			return response, utils.Errorf("yak.http2 error: %s", err)
		}
		response.RawPacket = rsp
		return response, err
	}

	var multiResponses []*http.Response
	var isMultiResponses bool
	var firstResponse *http.Response
	var responseRaw bytes.Buffer
	var rawBytes []byte

	if withConnPool {
		//连接池分支
		pc := conn.(*persistConn)
		writeErrCh := make(chan error, 1)
		if oldVersionProxyChecking {
			requestPacket, err = BuildLegacyProxyRequest(requestPacket)
			if err != nil {
				return nil, err
			}
		}
		pc.writeCh <- writeRequest{reqPacket: requestPacket, ch: writeErrCh}

		resc := make(chan responseInfo)
		pc.reqCh <- requestAndResponseCh{
			reqPacket: requestPacket,
			ch:        resc,
		}
		pcClosed := pc.closeCh
	LOOP:
		for {
			select {
			case err := <-writeErrCh:
				//写入失败，退出等待
				if err != nil {
					if pc.shouldRetryRequest(err) {
						conn.(*persistConn).removeConn()
						goto RECONNECT
					}
					return nil, err
				}
			case re := <-resc:
				//收到响应
				if (re.resp == nil) == (re.err == nil) {
					panic(fmt.Sprintf("internal error: exactly one of res or err should be set; nil=%v", re.resp == nil))
				}
				if re.err != nil && len(rawBytes) == 0 {
					if pc.shouldRetryRequest(re.err) {
						goto RECONNECT
					}
					return nil, re.err
				}
				firstResponse = re.resp
				rawBytes = re.respBytes
				traceInfo.ServerTime = re.info.ServerTime
				break LOOP
			case <-pcClosed:
				pcClosed = nil
				if pc.shouldRetryRequest(pc.closed) {
					goto RECONNECT
				}
				return nil, pc.closed
			}
		}

	} else {
		//不使用连接池分支
		if conn != nil {
			defer func() {
				conn.Close()
			}()
		}
		// 写报文
		if option.BeforeDoRequest != nil {
			requestPacket = option.BeforeDoRequest(requestPacket)
		}
		if oldVersionProxyChecking {
			var legacyRequest []byte
			legacyRequest, err = BuildLegacyProxyRequest(requestPacket)
			if err != nil {
				return nil, err
			}
			_, err = conn.Write(legacyRequest)
		} else {
			_, err = conn.Write(requestPacket)
		}
		if err != nil {
			return response, errors.Wrap(err, "write request failed")
		}

		// 读报文
		conn.SetDeadline(time.Now().Add(timeout))
		//TeeReader 用于畸形响应包: 即 ReadHTTPResponseFromBufioReader 无法解析但是conn中存在数据的情况
		httpResponseReader := bufio.NewReader(io.TeeReader(conn, &responseRaw))

		// 服务器响应第一个字节
		serverTimeStart := time.Now()
		peek, err := httpResponseReader.Peek(1)
		if err == nil && len(peek) == 1 {
			traceInfo.ServerTime = time.Since(serverTimeStart)
		}

		firstResponse, err = utils.ReadHTTPResponseFromBufioReader(httpResponseReader, nil)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return response, nil
			}
			log.Infof("[lowhttp] read response failed: %s", err)
		}

		if firstResponse == nil {
			if len(responseRaw.Bytes()) == 0 {
				return response, errors.Wrap(err, "empty result.")
			} else { // peek 到了数据,但是无法解析,说明是畸形响应包
				_, _ = io.ReadAll(httpResponseReader)
				rawBytes = responseRaw.Bytes()
			}
		} else {
			rawBytes, err = utils.DumpHTTPResponse(firstResponse, true)
			if err != nil {
				rawBytes = responseRaw.Bytes()
			} else {
				multiResponses = append(multiResponses, firstResponse)
			}

			// handle response
			for noFixContentLength { // 尝试读取pipeline/smuggle响应包
				// log.Infof("checking next(pipeline/smuggle) response...")

				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				nextResponse, err := utils.ReadHTTPResponseFromBufioReader(httpResponseReader, nil)

				if err != nil {
					if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) { // 停止读取
						break
					}
					// 非EOF错误,说明是conn内部大概率还有数据,但是解析失败证明是畸形响应包,所以改用TeeReader里的数据
					for {
						_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
						_, err = io.ReadAll(httpResponseReader)
						if err != nil {
							break //如果ReadAll失败则出现了问题,直接退出
						}
					}
					rawBytes = responseRaw.Bytes()
					break
				}

				if nextResponse != nil {
					multiResponses = append(multiResponses, nextResponse)
					isMultiResponses = true
					response.MultiResponse = true
					nextRaw, err := utils.DumpHTTPResponse(nextResponse, true)
					if err != nil {
						break
					}
					rawBytes = append(rawBytes, nextRaw...)
				}
			}
		}
	}

	var result = codec.EncodeBase64(rawBytes[:])
	_ = result

	// 更新cookiejar中的cookie
	if session != nil {
		cookiejar.SetCookies(urlIns, firstResponse.Cookies())
	}

	// status code retry
	var (
		retryFlag      = false
		retryNotInFlag = true
	)

	// not in statuscode
	if len(retryNotInStatusCode) > 0 {
		// 3xx status code can't retry
		for _, sc := range retryNotInStatusCode {
			if firstResponse.StatusCode == sc || (firstResponse.StatusCode >= 300 && firstResponse.StatusCode < 400) {
				retryNotInFlag = false
				break
			}
		}
		if retryNotInFlag {
			retryFlag = true
			goto STATUSCODERETRY
		}
	}

	// in statuscode
	for _, sc := range retryInStatusCode {
		if firstResponse.StatusCode == sc {
			retryFlag = true
			break
		}
	}

STATUSCODERETRY:
	if retryFlag && statusCodeRetryTimes < retryTimes {
		statusCodeRetryTimes += 1
		time.Sleep(jitterBackoff(retryWaitTime, retryMaxWaitTime, retry))
		log.Infof("retry reconnect because of status code [%d / %d]", statusCodeRetryTimes, retryTimes)
		goto RECONNECT
	}

	/*
		FixHTTPResponse will be executed when:
		1. SMUGGLE: noFixContentLength is false
		2. PIPELINE(multi response)
	*/
	if !noFixContentLength && !isMultiResponses {
		// fix
		//return responseRaw.Bytes(), nil
		rspRaw, _, err := FixHTTPResponse(rawBytes)
		if err != nil {
			log.Errorf("fix http response failed: %s", err)
			response.RawPacket = rawBytes
			return response, nil
		}
		response.RawPacket = rspRaw
		return response, nil
	}

	// 如果不修复的话，默认服务器返回的东西也有点复杂，不适合做其他处理
	response.RawPacket = rawBytes
	return response, nil
}
