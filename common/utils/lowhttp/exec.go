package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"reflect"
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
	VerifyCertificate    bool
	Https                bool
	ResponseCallback     func(response *LowhttpResponse)
	Http2                bool
	GmTLS                bool
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
	RuntimeId            string
	FromPlugin           string
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
	IsHttps  bool
	Request  []byte
	Response []byte
}

func HTTP(opts ...LowhttpOpt) (*LowhttpResponse, error) {
	option := NewLowhttpOption()
	for _, opt := range opts {
		opt(option)
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
		IsHttps:  response.Https,
		Request:  response.RawRequest,
		Response: response.RawPacket,
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

			r = UrlToGetRequestPacket(targetUrl, r, forceHttps, ExtractCookieJarFromHTTPResponse(lastPacket.Response)...)
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
				IsHttps:  response.Https,
				Request:  response.RawRequest,
				Response: response.RawPacket,
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

// SendHttpRequestWithRawPacketWithOpt
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

	https := forceHttps

	if gmTLS {
		https = true
	}
	// 获取url
	url, err := ExtractURLFromHTTPRequestRaw(r, https)
	if err != nil {
		if host != "" && port > 0 {
			var fallbackUrlMaterial []string
			var handledHost bool
			SplitHTTPHeadersAndBodyFromPacketEx(r, func(method string, requestUri string, proto string) error {
				fallbackUrlMaterial = append(fallbackUrlMaterial, method+" "+requestUri+" "+proto)
				return nil
			}, func(line string) {
				if strings.HasPrefix(strings.ToLower(line), "host:") {
					handledHost = true
					fallbackUrlMaterial = append(fallbackUrlMaterial, "Host: "+utils.HostPort(host, port))
				} else {
					fallbackUrlMaterial = append(fallbackUrlMaterial, line)
				}
			})
			if !handledHost {
				fallbackUrlMaterial = append(fallbackUrlMaterial, "Host: "+utils.HostPort(host, port))
			}
			var data = strings.Join(fallbackUrlMaterial, "\r\n") + "\r\n\r\n"
			url, err = ExtractURLFromHTTPRequestRaw([]byte(data), https)
			if err != nil {
				return nil, utils.Errorf("extract(fallback) url from request raw failed! reason: %v", err)
			}
		} else {
			return response, err
		}
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
			proxyConnIns, err = GetProxyConnWithContext(ctx, originAddr, proxyUrl, timeout)
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
		// no proxy

		var extraDNS []netx.DNSOption
		if len(dnsServers) > 0 {
			extraDNS = append(extraDNS, netx.WithDNSServers(dnsServers...), netx.WithDNSDisableSystemResolver(true))
		}
		if dnsHosts != nil && len(dnsHosts) > 0 {
			extraDNS = append(extraDNS, netx.WithTemporaryHosts(dnsHosts))
		}

		// DNS Resolve
		// ATTENTION: DO DNS AFTER PROXY CONN!
		var ip = host
		var addr string
		startDNS := time.Now()
		if !(utils.IsIPv4(host) || utils.IsIPv6(host)) {
			var ips = netx.LookupFirst(host, extraDNS...)
			traceInfo.DNSTime = time.Since(startDNS)
			if ips == "" {
				return response, utils.Errorf("[%vms] dns failed for querying: %s", traceInfo.DNSTime.Milliseconds(), host)
			}
			ip = ips
			addr = utils.HostPort(ip, port)
		} else {
			addr = utils.HostPort(host, port)
		}
		response.RemoteAddr = addr

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

			conn, err = GetTLSConn(gmTLS, option.VerifyCertificate, enableHttp2, host, rawConn, timeout)
			if err != nil {
				return response, err
			}
		}
	} else {
		response.RemoteAddr = conn.RemoteAddr().String()
		if https {
			conn, err = GetTLSConn(gmTLS, option.VerifyCertificate, enableHttp2, host, conn, timeout)
			if err != nil {
				return response, err
			}
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

func GetTLSConn(isGM bool, verify, enableHttp2 bool, host string, rawConn net.Conn, timeout time.Duration) (net.Conn, error) {
	var nextProtos []string
	if enableHttp2 {
		nextProtos = []string{http2.NextProtoTLS}
	} else {
		nextProtos = []string{"http/1.1"}
	}

	insecureSkipVerify := true
	if verify {
		insecureSkipVerify = false
	}

	if isGM {
		gmSupport := gmtls.NewGMSupport()
		gmSupport.EnableMixMode()

		config := &gmtls.Config{
			InsecureSkipVerify: insecureSkipVerify,
			MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
			MaxVersion:         tls.VersionTLS13,
			ServerName:         host,
			GMSupport:          gmSupport,
			NextProtos:         nextProtos,
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		var tlsConn = gmtls.Client(rawConn, config)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			rawConn.Close()
			return nil, err
		}
		return tlsConn, nil
	}
	config := &tls.Config{
		InsecureSkipVerify: insecureSkipVerify,
		MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
		MaxVersion:         tls.VersionTLS13,
		ServerName:         host,
		NextProtos:         nextProtos,
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var tlsConn = tls.Client(rawConn, config)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, err
	}
	return tlsConn, nil
}
