package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"yaklang/common/consts"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

const (
	defaultWaitTime    = time.Duration(100) * time.Millisecond
	defaultMaxWaitTime = time.Duration(2000) * time.Millisecond
)

type LowhttpExecConfig struct {
	Host                 string
	Port                 int
	Packet               []byte
	Https                bool
	Http2                bool
	Timeout              time.Duration
	RedirectTimes        int
	RetryTimes           int
	RetryInStatusCode    []int
	RetryNotInStatusCode []int
	RetryWaitTime        time.Duration
	RetryMaxWaitTime     time.Duration
	JsRedirect           bool
	Proxy                []string
	NoFixContentLength   bool
	RedirectHandler      func(bool, []byte, []byte) bool
	Session              interface{}
	BeforeDoRequest      func([]byte) []byte
	Ctx                  context.Context
	SaveHTTPFlow         bool
	RequestSource        string
	EtcHosts             map[string]string
	DNSServers           []string
}

type LowhttpResponse struct {
	RawPacket          []byte
	RedirectRawPackets [][]byte
	PortIsOpen         bool
	TraceInfo          *LowhttpTraceInfo
	Url                string
	RemoteAddr         string
	Proxy              string
	Https              bool
	Http2              bool
	RawRequest         []byte
	Source             string // 请求源
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

// new LowhttpOpt
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
		SaveHTTPFlow:         consts.GetDefaultSaveHTTPFlowFromEnv(),
	}
}

type LowhttpOpt func(o *LowhttpExecConfig)

func WithETCHosts(hosts map[string]string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.EtcHosts = hosts
	}
}

func WithDNSServers(servers []string) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.DNSServers = servers
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

func WithPacket(packet []byte) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Packet = packet
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

func WithTimeout(timeout time.Duration) LowhttpOpt {
	return func(o *LowhttpExecConfig) {
		o.Timeout = timeout
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
		o.Proxy = proxy
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

func SendHTTPRequestRawQuickWithTimeout(https bool, r *http.Request, timeout time.Duration) ([]byte, error) {
	u, err := ExtractURLFromHTTPRequest(r, https)
	if err != nil {
		return nil, err
	}

	host, port, err := utils.ParseStringToHostPort(u.String())
	if err != nil {
		return nil, err
	}

	return SendHTTPRequestRaw(https, host, port, r, timeout)
}

func SendHTTPRequestRawQuick(https bool, r *http.Request) ([]byte, error) {
	return SendHTTPRequestRawQuickWithTimeout(https, r, 5*time.Second)
}

func SendHTTPRequestWithRawPacketWithRedirect(https bool, host string, port int, r []byte, timeout time.Duration, redirectTimes int, proxy ...string) ([]byte, [][]byte, error) {
	return SendHTTPRequestWithRawPacketWithRedirectEx(https, host, port, r, timeout, redirectTimes, nil, proxy...)
}

func SendHTTPRequestWithRawPacketWithRedirectEx(
	https bool, host string, port int, r []byte, timeout time.Duration,
	redirectTimes int, redirectHandler func(isHttps bool, req []byte, rsp []byte) bool,
	proxy ...string) ([]byte, [][]byte, error) {
	return SendHTTPRequestWithRawPacketWithRedirectFullEx(https, host, port, r, timeout, redirectTimes, false, redirectHandler, false, false, proxy...)
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

// SendHTTPRequestWithRawPacketWithRedirectWithStateWithOptFullEx
func SendHTTPRequestWithRawPacketWithRedirectWithStateWithOptFullEx(opts ...LowhttpOpt) (*LowhttpResponse, error) {
	option := NewLowhttpOption()
	for _, opt := range opts {
		opt(option)
	}

	var (
		forceHttps           = option.Https
		forceHttp2           = option.Http2
		host                 = option.Host
		port                 = option.Port
		r                    = option.Packet
		timeout              = option.Timeout
		retryTimes           = option.RetryTimes
		retryInStatusCode    = option.RetryInStatusCode
		retryNotInStatusCode = option.RetryNotInStatusCode
		retryWaitTime        = option.RetryWaitTime
		retryMaxWaitTime     = option.RetryMaxWaitTime
		redirectTimes        = option.RedirectTimes
		redirectHandler      = option.RedirectHandler
		jsRedirect           = option.JsRedirect
		noFixContentLength   = option.NoFixContentLength
		proxy                = option.Proxy
		saveHTTPFlow         = option.SaveHTTPFlow
		session              = option.Session
		requestHook          = option.BeforeDoRequest
		ctx                  = option.Ctx
		redirectRawPackets   [][]byte
		commonOptions        = []LowhttpOpt{
			WithHttp2(forceHttp2),
			WithTimeout(timeout),
			WithRetryTimes(retryTimes),
			WithRetryInStatusCode(retryInStatusCode),
			WithRetryNotInStatusCode(retryNotInStatusCode),
			WithRetryWaitTime(retryWaitTime),
			WithRetryMaxWaitTime(retryMaxWaitTime),
			WithRedirectTimes(redirectTimes),
			WithNoFixContentLength(noFixContentLength),
			WithProxy(proxy...),
			WithSession(session),
			WithBeforeDoRequest(requestHook),
			WithContext(ctx),
			WithSaveHTTPFlow(saveHTTPFlow),
			WithSource(option.RequestSource),
			WithETCHosts(option.EtcHosts),
			WithDNSServers(option.DNSServers),
		}
		requestOptions []LowhttpOpt

		response *LowhttpResponse
		err      error
	)

	requestOptions = append(commonOptions,
		WithHttps(forceHttps),
		WithHost(host),
		WithPort(port),
		WithPacket(r),
	)

	response, err = SendHttpRequestWithRawPacketWithOptEx(requestOptions...)
	raw := response.RawPacket
	if err != nil {
		return response, err
	}

	if raw != nil {
		redirectRawPackets = append(redirectRawPackets, raw)
	}

	if redirectTimes > 0 {
		lastPacket := raw
		for i := 0; i < redirectTimes; i++ {
			target := GetRedirectFromHTTPResponse(lastPacket, jsRedirect)
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
				if !redirectHandler(forceHttps, r, lastPacket) {
					break
				}
			}

			targetUrl := MergeUrlFromHTTPRequest(r, target, forceHttps)

			r = UrlToGetRequestPacket(targetUrl, r, forceHttps, ExtractCookieJarFromHTTPResponse(lastPacket)...)
			nextHost, nextPort, _ := utils.ParseStringToHostPort(targetUrl)
			log.Debugf("[lowhttp] redirect to: %s", targetUrl)

			requestOptions = append(commonOptions,
				WithHttps(forceHttps),
				WithHost(nextHost),
				WithPort(nextPort),
				WithPacket(r),
			)

			// 更新response
			response, err = SendHttpRequestWithRawPacketWithOptEx(requestOptions...)
			responseRaw := response.RawPacket

			if err != nil {
				log.Errorf("met error in redirect...: %s", err)
				response.RawPacket = lastPacket // 保留原始报文
				return response, nil
			}
			if responseRaw == nil {
				response.RawPacket = lastPacket // 保留原始报文
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

// SendHTTPRequestWithRawPacketWithRedirectWithState 返回端口状态
func SendHTTPRequestWithRawPacketWithRedirectWithStateFullEx(
	https bool, host string, port int, r []byte, timeout time.Duration,
	redirectTimes int, jsRedirect bool,
	redirectHandler func(isHttps bool, req []byte, rsp []byte) bool,
	noFixContentLength bool, forceHttp2 bool, // 这个参数很关键，一般有这个情况的话，很可能用户发了多个包。
	proxy ...string) ([]byte, [][]byte, bool, error) {

	response, err := SendHTTPRequestWithRawPacketWithRedirectWithStateWithOptFullEx(
		WithHttps(https),
		WithHost(host),
		WithPort(port),
		WithPacket(r),
		WithTimeout(timeout),
		WithRedirectTimes(redirectTimes),
		WithJsRedirect(jsRedirect),
		WithRedirectHandler(redirectHandler),
		WithNoFixContentLength(noFixContentLength),
		WithHttp2(forceHttp2),
		WithProxy(proxy...),
	)
	return response.RawPacket, response.RedirectRawPackets, response.PortIsOpen, err
}

func SendHTTPRequestWithRawPacketWithRedirectWithContextFullEx(
	https bool, host string, port int, r []byte, timeout time.Duration,
	redirectTimes int, jsRedirect bool, ctx context.Context,
	redirectHandler func(isHttps bool, req []byte, rsp []byte) bool,
	noFixContentLength bool, forceHttp2 bool, source string,
	proxy ...string) ([]byte, [][]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	response, err := SendHTTPRequestWithRawPacketWithRedirectWithStateWithOptFullEx(
		WithHttps(https),
		WithHost(host),
		WithPort(port),
		WithPacket(r),
		WithTimeout(timeout),
		WithRedirectTimes(redirectTimes),
		WithJsRedirect(jsRedirect),
		WithRedirectHandler(redirectHandler),
		WithNoFixContentLength(noFixContentLength),
		WithHttp2(forceHttp2),
		WithProxy(proxy...),
		WithContext(ctx),
		WithSource(source),
	)
	return response.RawPacket, response.RedirectRawPackets, err
}

// SendHTTPRequestWithRawPacketWithRedirectFullEx
func SendHTTPRequestWithRawPacketWithRedirectFullEx(
	https bool, host string, port int, r []byte, timeout time.Duration,
	redirectTimes int, jsRedirect bool,
	redirectHandler func(isHttps bool, req []byte, rsp []byte) bool,
	noFixContentLength bool, forceHttp2 bool, // 这个参数很关键，一般有这个情况的话，很可能用户发了多个包。
	proxy ...string) ([]byte, [][]byte, error) {
	rsp, reqs, _, err := SendHTTPRequestWithRawPacketWithRedirectWithStateFullEx(
		https, host, port, r, timeout,
		redirectTimes, jsRedirect, redirectHandler,
		noFixContentLength, forceHttp2, proxy...)
	return rsp, reqs, err
}

func SendHTTPRequestWithRawPacket(forceHttps bool, host string, port int, r []byte, timeout time.Duration, proxy ...string) ([]byte, error) {
	rsp, _, err := SendHTTPRequestWithRawPacketEx(forceHttps, host, port, r, timeout, false, false, proxy...)
	return rsp, err
}

// SendHttpRequestWithRawPacketWithOpt
func SendHttpRequestWithRawPacketWithOptEx(opts ...LowhttpOpt) (*LowhttpResponse, error) {
	option := NewLowhttpOption()
	for _, opt := range opts {
		opt(option)
	}

	var (
		forceHttps           = option.Https
		forceHttp2           = option.Http2
		host                 = option.Host
		port                 = option.Port
		r                    = option.Packet
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
	)
	if ctx == nil {
		ctx = context.Background()
	}

	if response.Source == "" {
		response.Source = source
	}

	defer func() {
		if response == nil && !saveHTTPFlow {
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

	https := forceHttps
	// 获取url
	url, err := ExtractURLFromHTTPRequestRaw(r, https)
	if err != nil {
		return response, err
	}

	// 逐个记录 response 中的内容
	response.Url = url.String()

	// 获取cookiejar
	cookiejar := GetCookiejar(session)
	if session != nil {
		cookies := cookiejar.Cookies(url)

		// 复用session中的cookie
		r, err = AddOrUpgradeCookie(r, CookiesToString(cookies))
		if err != nil {
			return response, err
		}
	}

	// 修复 host port
	if port <= 0 || host == "" {
		newHost, newPort, err := utils.ParseStringToHostPort(url.String())
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

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	if retryTimes < 0 {
		retryTimes = 0
	}

	if dnsServers == nil || len(dnsServers) <= 0 {
		dnsServers = utils.DefaultDNSServer
	}

	// 修正域名的情况
	var ip string = host
	response.TraceInfo.AvailableDNSServers = dnsServers
	startDNS := time.Now()
	if !(utils.IsIPv4(host) || utils.IsIPv6(host)) {
		var ips string
		if dnsHosts != nil {
			raw, ok := dnsHosts[host]
			if ok {
				ips = raw
			} else {
				raw2, ok2 := GetSystemHostByName(host)
				if ok2 {
					ips = raw2
				}
			}
		}

		if ips == "" {
			ips = utils.GetFirstIPByDnsWithCache(
				host,
				timeout,
				dnsServers...)
		}
		traceInfo.DNSTime = time.Since(startDNS)
		if ips == "" {
			return response, utils.Errorf("[%vms] dns failed for querying: %s", traceInfo.DNSTime.Milliseconds(), host)
		}
		ip = ips
	}

	var targetAddr string
	if ip != host {
		targetAddr = utils.HostPort(ip, port)
	} else {
		targetAddr = utils.HostPort(host, port)
	}
	response.RemoteAddr = utils.HostPort(ip, port)

	// 修复CRLF
	r = FixHTTPPacketCRLF(r, noFixContentLength)
	response.RawRequest = r

	var (
		enableHttp2 = false
	)
	// http2
	SplitHTTPHeadersAndBodyFromPacketEx(r, func(method string, requestUri string, proto string) error {
		if forceHttp2 || (strings.HasPrefix(proto, "HTTP/2") && https) {
			enableHttp2 = true
		}
		return errors.New("normal abort")
	})

	response.Http2 = enableHttp2

	//https://github.com/mattn/go-ieproxy
	var (
		conn                 net.Conn
		proxyConn            net.Conn
		proxyConnIns         net.Conn
		retry                int
		statusCodeRetryTimes int = 0
	)
	if len(proxy) == 1 && proxy[0] == "" {
		proxy = proxy[1:]
	}

RECONNECT:
	totalTimeStart := time.Now()
	defer func() {
		traceInfo.TotalTime = time.Since(totalTimeStart)
	}()

	for _, proxyUrl := range proxy {
		// retry when timeout
		for retry = 0; retry <= retryTimes; retry++ {
			start := time.Now()
			proxyConnIns, err = GetProxyConnWithContext(ctx, targetAddr, proxyUrl, timeout)
			traceInfo.ConnTime = time.Since(start)

			if err == nil || !isErrorTimeout(err) {
				break
			}
			log.Debugf("[lowhttp] [%d / %d] retry dial with proxy: %s", retry, retryTimes, proxy)
			time.Sleep(jitterBackoff(retryWaitTime, retryMaxWaitTime, retry))
		}

		if err != nil {
			log.Errorf("use proxy failed: %s, reason: %v", proxy, err)
			continue
		}
		conn = proxyConnIns
		proxyConn = proxyConnIns
		response.Proxy = proxyUrl
		break
	}

	if conn == nil && len(proxy) > 0 {
		return response, utils.Errorf("cannot create proxy[%v] connection", proxy)
	}

	if proxyConn != nil {
		defer proxyConn.Close()
	}

	var tcpDailer = &net.Dialer{
		Timeout: timeout,
	}

	response.Https = https

	if conn == nil {
		switch https {
		case false:
			// retry when timeout
			for retry = 0; retry <= retryTimes; retry++ {
				start := time.Now()
				conn, err = tcpDailer.DialContext(ctx, "tcp", utils.HostPort(ip, port))
				//conn, err = tcpDailer.Dial("tcp", utils.HostPort(ip, port))
				traceInfo.ConnTime = time.Since(start)

				if err == nil || !isErrorTimeout(err) {
					break
				}
				log.Debugf("[lowhttp] [%d / %d] retry dial with remote: %s:%d", retry, retryTimes, ip, port)
				time.Sleep(jitterBackoff(retryWaitTime, retryMaxWaitTime, retry))
			}
			if err != nil {
				return response, utils.Errorf("dial %v failed: %s", utils.HostPort(ip, port), err)
			}
			response.PortIsOpen = true
		default:
			var rawConn net.Conn
			// retry when timeout
			for retry = 0; retry <= retryTimes; retry++ {
				start := time.Now()
				rawConn, err = tcpDailer.Dial("tcp", utils.HostPort(ip, port))
				traceInfo.ConnTime = time.Since(start)

				if err == nil || !isErrorTimeout(err) {
					break
				}
				log.Debugf("[lowhttp] [%d / %d] retry dial with remote: %s:%d", retry, retryTimes, ip, port)
				time.Sleep(jitterBackoff(retryWaitTime, retryMaxWaitTime, retry))
			}

			if err != nil {
				return response, utils.Errorf("dial %v failed: %s", utils.HostPort(ip, port), err)
			}
			response.PortIsOpen = true

			config := &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
				ServerName:         host,
			}
			if enableHttp2 {
				config.NextProtos = []string{http2.NextProtoTLS}
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			var tlsConn = tls.Client(rawConn, config)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				rawConn.Close()
				return response, err
			}
			conn = tlsConn
			//conn, err = tls.DialWithDialer(tcpDailer, "tcp", utils.HostPort(ip, port))
			//if err != nil {
			//	return nil, utils.Errorf("tls dial %v failed: %s", utils.HostPort(ip, port), err)
			//}
		}
	} else {
		if https {
			config := &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
				ServerName:         host,
			}
			if enableHttp2 {
				config.NextProtos = []string{http2.NextProtoTLS}
			}
			var tlsConn = tls.Client(conn, config)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				conn.Close()
				return response, err
			}
			conn = tlsConn
		}
	}

	if conn != nil {
		defer func() {
			conn.Close()
		}()
	}

	if enableHttp2 {
		//ddl, cancel := context.WithTimeout(context.Background(), timeout)
		//defer cancel()
		if option.BeforeDoRequest != nil {
			r = option.BeforeDoRequest(r)
		}
		rsp, err := HTTPRequestToHTTP2("https", utils.HostPort(host, port), conn, r, noFixContentLength)
		if err != nil {
			return response, utils.Errorf("yak.http2 error: %s", err)
		}
		response.RawPacket = rsp
		return response, err
	}

	// 写报文
	if option.BeforeDoRequest != nil {
		r = option.BeforeDoRequest(r)
	}
	_, err = conn.Write(r)
	if err != nil {
		return response, utils.Errorf("write request failed: %s", err)
	}

	// 读报文
	var responseRaw bytes.Buffer
	conn.SetDeadline(time.Now().Add(timeout))
	httpResponseReader := bufio.NewReader(io.TeeReader(conn, &responseRaw))

	// 服务器响应第一个字节
	serverTimeStart := time.Now()
	peek, err := httpResponseReader.Peek(1)
	if err == nil && len(peek) == 1 {
		traceInfo.ServerTime = time.Since(serverTimeStart)
	}

	rspIns, err := http.ReadResponse(httpResponseReader, nil)

	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return response, nil
		}
		log.Infof("[lowhttp] read response failed: %s", err)
	}
	if rspIns == nil {
		if len(responseRaw.Bytes()) == 0 {
			return response, utils.Errorf("empty result. %v", err.Error())
		}
	} else {
		if rspIns.Body != nil {
			_, _ = io.Copy(ioutil.Discard, io.LimitReader(rspIns.Body, 10*1024*1024))
		}
	}

	rawBytes := responseRaw.Bytes()
	var result = codec.EncodeBase64(rawBytes[:])
	_ = result

	// 更新cookiejar中的cookie
	if session != nil {
		cookiejar.SetCookies(url, rspIns.Cookies())
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
			if rspIns.StatusCode == sc || (rspIns.StatusCode >= 300 && rspIns.StatusCode < 400) {
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
		if rspIns.StatusCode == sc {
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

	// 在修复 ContentLength 的情况下，默认应该有一个响应被返回。
	if !noFixContentLength {
		// fix
		//return responseRaw.Bytes(), nil
		rspRaw, _, err := FixHTTPResponse(responseRaw.Bytes())
		if err != nil {
			log.Errorf("fix http response failed: %s", err)
			response.RawPacket = responseRaw.Bytes()
			return response, nil
		}
		response.RawPacket = rspRaw
		return response, nil
	}

	// 如果不修复的话，默认服务器返回的东西也有点复杂，不适合做其他处理
	//return utils.StableReader(io.TeeReader(conn, os.Stdout), timeout, 10*1024*1024), nil
	response.RawPacket = responseRaw.Bytes()
	return response, nil
}

/*
SendHTTPRequestWithRawPacketEx

Returns:
 1. response bytes
 2. is port opened?
 3. error
*/
func SendHTTPRequestWithRawPacketEx(forceHttps bool, host string, port int, r []byte, timeout time.Duration, noFixContentLength bool, forceHttp2 bool, proxy ...string) ([]byte, bool, error) {
	response, err := SendHttpRequestWithRawPacketWithOptEx(
		WithHttps(forceHttps),
		WithHost(host),
		WithPort(port),
		WithPacket(r),
		WithTimeout(timeout),
		WithNoFixContentLength(noFixContentLength),
		WithHttp2(forceHttp2),
		WithProxy(proxy...),
	)
	return response.RawPacket, response.PortIsOpen, err
}

func SendHTTPRequestRaw(https bool, host string, port int, r *http.Request, timeout time.Duration) ([]byte, error) {
	// 修复 host port
	if port <= 0 || host == "" {
		u, err := ExtractURLFromHTTPRequest(r, https)
		if err != nil {
			return nil, err
		}

		newHost, newPort, err := utils.ParseStringToHostPort(u.String())
		if err != nil {
			return nil, err
		}

		if port <= 0 {
			port = newPort
		}

		if host == "" {
			host = newHost
		}
	}

	if port <= 0 {
		return nil, utils.Errorf("empty port...")
	}

	// 修正域名的情况
	var (
		ip string = host
	)
	if utils.IsValidDomain(host) {
		ips := utils.GetFirstIPByDnsWithCache(
			host,
			timeout, utils.DefaultDNSServer...)
		if ips == "" {
			return nil, utils.Errorf("dns failed for querying: %s", host)
		}
		ip = ips
	}

	raw, err := httputil.DumpRequest(r, true)
	if err != nil {
		return nil, utils.Errorf("dump http.Request failed: %s", err)
	}

	var (
		conn net.Conn
	)
	switch https {
	case false:
		dialer := &net.Dialer{Timeout: timeout}
		conn, err = dialer.Dial("tcp", utils.HostPort(ip, port))
		if err != nil {
			return nil, utils.Errorf("dial %v failed: %s", utils.HostPort(ip, port), err)
		}

	default:
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", utils.HostPort(ip, port), &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
			MaxVersion:         tls.VersionTLS13,
			ServerName:         host,
		})
		if err != nil {
			return nil, utils.Errorf("tls dial %v failed: %s", utils.HostPort(ip, port), err)
		}
	}

	utils.Debug(func() {
		println(string(raw))
	})

	// 写报文
	_, err = conn.Write(raw)
	if err != nil {
		return nil, utils.Errorf("write http.Request packet failed: %v ", err)
	}

	raw, err = utils.ReadConnWithTimeout(conn, timeout)
	if len(raw) > 0 {
		return raw, nil
	}

	return nil, utils.Errorf("read failed: empty with: Err[%v]", err)
}

func SendPacketQuick(
	https bool,
	packet []byte,
	timeout float64,
	proxy ...string) ([]byte, [][]byte, error) {
	req, err := ParseBytesToHttpRequest(packet)
	if err != nil {
		return nil, nil, err
	}
	var host string = req.Host
	if host == "" {
		host = req.Header.Get("Host")
	}

	targetHost, targetPort, err := utils.ParseStringToHostPort(host)
	if err != nil {
		if https {
			host = utils.HostPort(host, 443)
		} else {
			host = utils.HostPort(host, 80)
		}
	} else {
		host = utils.HostPort(targetHost, targetPort)
	}

	targetHost, targetPort, err = utils.ParseStringToHostPort(host)
	if err != nil {
		return nil, nil, err
	}
	if !https {
		https = utils.IsTLSService(targetHost)
	}
	return SendHTTPRequestWithRawPacketWithRedirect(
		https, targetHost, targetPort, packet, utils.FloatSecondDuration(timeout),
		5, proxy...,
	)
}
