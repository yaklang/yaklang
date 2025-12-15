package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	utls "github.com/refraction-networking/utls"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

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

func beforeLog(option *LowhttpExecConfig) {
	atomic.AddInt64(option.BeforeCount, 1)
	log.Infof("debug counter: %v, %v", "http call", atomic.LoadInt64(option.BeforeCount))
}

func afterLog(option *LowhttpExecConfig) {
	atomic.AddInt64(option.AfterCount, 1)
	log.Infof("debug counter: %v, %v", "http done", atomic.LoadInt64(option.AfterCount))
}

func HTTP(opts ...LowhttpOpt) (*LowhttpResponse, error) {
	option := NewLowhttpOption()
	for _, opt := range opts {
		opt(option)
	}

	if option.Session == nil {
		sessionId := uuid.New().String()
		option.Session = sessionId
		opts = append(opts, WithSession(option.Session))
		defer RemoveCookiejar(sessionId)
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
	redirectRawPackets = append(redirectRawPackets, raw)

	if redirectTimes > 0 {
		lastPacket := raw
		statusCode := GetStatusCodeFromResponse(lastPacket.Response)

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

			targetUrl := MergeUrlFromHTTPRequest(r, target, forceHttps)

			// should not extract response cookie
			r, err = BuildRedirectRequest(targetUrl, r, lastPacket.IsHttps, statusCode)
			if err != nil {
				log.Errorf("met error in redirect: %v", err)
				response.RawPacket = lastPacket.Response // 保留原始报文
				return response, nil
			}

			if redirectHandler != nil {
				if !redirectHandler(forceHttps, r, lastPacket.Response) {
					break
				}
			}

			nextHost, nextPort, _ := utils.ParseStringToHostPort(targetUrl)
			log.Debugf("[lowhttp] redirect to: %s", targetUrl)

			newOpts := append(opts, WithHttps(forceHttps), WithHost(nextHost), WithPort(nextPort), WithRequest(r))
			response, err = HTTPWithoutRedirect(newOpts...)
			if err != nil {
				log.Errorf("met error in redirect: %v", err)
				response.RawPacket = lastPacket.Response // 保留原始报文
				return response, nil
			}
			if response == nil {
				return response, nil
			}

			responseRaw := &RedirectFlow{
				IsHttps:    response.Https,
				Request:    response.RawRequest,
				Response:   response.RawPacket,
				RespRecord: response,
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

var _debugCounter = new(int64)

func addDebugCounter(prompt string) {
	result := atomic.AddInt64(_debugCounter, 1)
	log.Infof("%v: debug counter: %v", prompt, result)
}

// HTTPWithoutRedirect SendHttpRequestWithRawPacketWithOpt
func HTTPWithoutRedirect(opts ...LowhttpOpt) (*LowhttpResponse, error) {
	option := NewLowhttpOption()
	for _, opt := range opts {
		opt(option)
	}
	retryHandler := option.RetryHandler
	retry := func(rsp *LowhttpResponse, rawBytes []byte, retryTimes int) bool {
		if retryHandler != nil {
			rspRaw, _, err := FixHTTPResponse(rawBytes)
			if err != nil {
				rspRaw = rawBytes
			}
			var httpsFlag = option.Https
			var rawReq = option.Packet
			var retryFlag bool
			retryHandler(httpsFlag, retryTimes, rawReq, rspRaw, func(i ...[]byte) {
				if len(i) > 0 {
					option.Packet = i[0]
				}
				retryFlag = true
			})
			return retryFlag
		} else {
			statusCode := GetStatusCodeFromResponse(rawBytes)
			if len(option.RetryNotInStatusCode) > 0 {
				var retryNotIn = true
				for _, sc := range option.RetryNotInStatusCode { // black list first
					if statusCode == sc || (statusCode >= 300 && statusCode < 400) { // 3xx code can't retry
						retryNotIn = false
						break
					}
				}
				if retryNotIn {
					return true
				}
			}
			// in statuscode
			for _, sc := range option.RetryInStatusCode {
				if statusCode == sc {
					return true
				}
			}
			return false
		}
	}
	retryTimes := 0
	for {
		response, err := HTTPWithoutRetry(option)
		if err != nil {
			return response, err
		}
		if retry(response, response.RawPacket, retryTimes) && (retryTimes < option.RetryTimes || retryHandler != nil) {
			retryTimes += 1
			time.Sleep(utils.JitterBackoff(option.RetryWaitTime, option.RetryMaxWaitTime, retryTimes))
			log.Infof("retry reconnect because [%d / %d]", retryTimes, option.RetryMaxWaitTime)
			continue
		}
		return response, nil
	}
}

func HTTPWithoutRetry(option *LowhttpExecConfig) (*LowhttpResponse, error) {
	var (
		https                   = option.Https
		forceHttp2              = option.Http2
		forceHttp3              = option.Http3
		gmTLS                   = option.GmTLS
		onlyGMTLS               = option.GmTLSOnly
		preferGMTLS             = option.GmTLSPrefer
		host                    = option.Host
		port                    = option.Port
		requestPacket           = option.Packet
		timeout                 = option.Timeout
		connectTimeout          = option.ConnectTimeout
		maxRetryTimes           = option.RetryTimes
		customFailureChecker    = option.CustomFailureChecker
		retryWaitTime           = option.RetryWaitTime
		retryMaxWaitTime        = option.RetryMaxWaitTime
		noFixContentLength      = option.NoFixContentLength
		proxy                   = option.Proxy
		saveHTTPFlow            = option.SaveHTTPFlow
		saveHTTPFlowSync        = option.SaveHTTPFlowSync
		saveHTTPFlowHandlerList = option.SaveHTTPFlowHandler
		session                 = option.Session
		ctx                     = option.Ctx
		traceInfo               = newLowhttpTraceInfo()
		response                = newLowhttpResponse(traceInfo)
		source                  = option.RequestSource
		dnsServers              = option.DNSServers
		dnsHosts                = option.EtcHosts
		connPool                = option.ConnPool
		withConnPool            = option.WithConnPool
		sni                     = option.SNI
		payloads                = option.Payloads
		tags                    = option.Tags
		firstAuth               = true
		reqIns                  = option.NativeHTTPRequestInstance
		maxContentLength        = option.MaxContentLength
		randomJA3FingerPrint    = option.RandomJA3FingerPrint
		clientHelloSpec         = option.ClientHelloSpec
		dialer                  = option.Dialer
		fixQueryEscape          = option.FixQueryEscape
	)

	failureChecker := func(rsp *LowhttpResponse) error {
		if customFailureChecker != nil && rsp != nil {
			var failureMsg string
			var hasFailed bool
			customFailureChecker(rsp.Https, rsp.RawRequest, rsp.BareResponse, func(msg string) {
				failureMsg = msg
				hasFailed = true
			})
			if hasFailed {
				return utils.Errorf("request failed intentionally by custom failure checker: %s", failureMsg)
			}
		}
		return nil
	}

	connectTimeout = 2 * time.Second
	if reqIns == nil {
		// create new request instance for httpctx
		reqIns, _ = utils.ReadHTTPRequestFromBytes(requestPacket)
	}
	response.RequestInstance = reqIns

	if connPool == nil {
		connPool = DefaultLowHttpConnPool
	}

	// 用于检查 BodyStreamReaderHandler 是否被正常调用
	bodyStreamReaderHandled := utils.NewAtomicBool()
	// 等待 BodyStreamReaderHandler 调用结束
	waitBodyStreamReaderHandledCallEnd := sync.WaitGroup{}
	defer func() {
		waitBodyStreamReaderHandledCallEnd.Wait()
		if option != nil && option.BodyStreamReaderHandler != nil && !bodyStreamReaderHandled.IsSet() {
			r, w := utils.NewPipe()
			w.Close()
			option.BodyStreamReaderHandler([]byte{}, r)
		}
	}()

	// ctx
	if ctx == nil {
		ctx = context.Background()
	}
	// fix some field
	response.Source = source
	response.Payloads = payloads
	response.Tags = tags

	if option.EnableMaxContentLength && maxContentLength > 0 {
		httpctx.SetResponseMaxContentLength(reqIns, maxContentLength)
	}

	if option.NoBodyBuffer {
		httpctx.SetNoBodyBuffer(reqIns, true)
	}

	if option.UseMITMRule && mitmReplacerLabelingHTTPFlowFunc != nil {
		saveHTTPFlowHandlerList = append(saveHTTPFlowHandlerList, mitmReplacerLabelingHTTPFlowFunc)
	}
	/*
		save http flow defer
	*/
	defer func() {
		if httpctx.GetResponseTooLarge(reqIns) {
			response.TooLarge = true
			response.TooLargeLimit = int64(maxContentLength)
		}

		if saveHTTPFlowHandlerList != nil {
			for _, f := range saveHTTPFlowHandlerList {
				f(response)
			}
		}

		if response == nil || !saveHTTPFlow {
			return
		}

		log.Debugf("should save url: %v", response.Url)
		saveCtx, cancel := context.WithCancel(ctx)

		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("save response panic! reason: %v", err)
				}
				cancel()
			}()

			SaveLowHTTPResponse(response, saveHTTPFlowSync)
		}()
		select {
		case <-saveCtx.Done():
		}
	}()

	/*
	   proxy
	*/
	var regulatoryProxy []string
	for _, p := range proxy {
		i, err := url.Parse(p)
		if err != nil {
			continue
		}
		if i.Hostname() == "" {
			continue
		}
		regulatoryProxy = append(regulatoryProxy, p)
	}
	proxy = regulatoryProxy

	forceProxy := len(proxy) > 0
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

	/*
	   get some config from packet
	*/
	var forceOverrideURL string
	var requestURI string
	var hostInPacket string
	var haveTE bool
	var haveCL bool
	var clInt int
	enableHttp2 := false
	enableHttp3 := false
	_, originBody := SplitHTTPHeadersAndBodyFromPacketEx(requestPacket, func(method string, uri string, proto string) error {
		requestURI = uri
		if strings.HasPrefix(proto, "HTTP/2") || forceHttp2 {
			enableHttp2 = true
			// TODO: http should set pool force???
			withConnPool = true
		} else if strings.HasPrefix(proto, "HTTP/3") || forceHttp3 {
			enableHttp3 = true
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

	/*
	   extract url
	*/
	if gmTLS || enableHttp3 {
		https = true
	}
	var urlBuf bytes.Buffer
	if https {
		urlBuf.WriteString("https://")
	} else {
		urlBuf.WriteString("http://")
	}

	if hostInPacket == "" && host == "" {
		return response, utils.Errorf("host not found in packet and option (Check your `Host: ` header)")
	}

	urlStr := forceOverrideURL
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
	}

	/*
		checking pipeline or smuggle
	*/
	if haveTE && haveCL {
		if !noFixContentLength {
			log.Warnf("request \n%v\n have both `Transfer-Encoding` and `Content-Length` header, maybe pipeline or smuggle, please enable noFixContentLength", spew.Sdump(requestPacket))
		}
		// noFixContentLength = true
	} else if haveCL && !haveTE && clInt >= 0 && len(originBody) > clInt {
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
		body, nextPacket := codec.HTTPChunkedDecodeWithRestBytes(originBody)
		_ = body
		if len(nextPacket) > 0 {
			SplitHTTPPacket(nextPacket, func(method string, requestUri string, proto string) error {
				if ret := len(proto); ret > 5 && ret <= 8 && strings.HasPrefix(proto, "HTTP/") && proto[5] >= '0' && proto[5] <= '9' {
					if _, ok := commonHTTPMethod[method]; ok {
						// noFixContentLength = true
					}
				}
				return utils.Error("pipeline or smuggle detected, auto enable noFixContentLength")
			}, nil)
		}
	}

	// 逐个记录 response 中的内容
	response.Url = urlStr

	// 获取 cookiejar（仅在 session != nil 时启用自动 cookie 管理）
	var cookiejar http.CookieJar
	if session != nil {
		cookiejar = GetCookiejar(session)
		cookies := cookiejar.Cookies(urlIns)
		if cookies != nil {
			var needAppendCookie []*http.Cookie
			for _, cookie := range cookies {
				if GetHTTPPacketCookie(requestPacket, cookie.Name) == "" {
					needAppendCookie = append(needAppendCookie, cookie)
				}
			}
			requestPacket, err = AddOrUpgradeCookieHeader(requestPacket, CookiesToString(needAppendCookie))
			if err != nil {
				return response, err
			}
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
	originAddr := utils.HostPort(host, port)

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	if maxRetryTimes < 0 {
		maxRetryTimes = 0
	}

	response.TraceInfo.AvailableDNSServers = dnsServers
	response.RuntimeId = option.RuntimeId
	response.FromPlugin = option.FromPlugin

	// fix CRLF
	requestPacket = FixHTTPPacketCRLF(requestPacket, noFixContentLength)

	if fixQueryEscape {
		requestPacket = FixHTTPPacketQueryEscape(requestPacket)
	}
	response.RawRequest = requestPacket
	response.Http2 = enableHttp2

	// https://github.com/mattn/go-ieproxy
	var (
		conn net.Conn
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
	reqSchema := H1
	if enableHttp2 {
		nextProto = []string{H2}
		reqSchema = H2
	} else {
		nextProto = []string{H1}
	}

	// 需要用于标识连接 https gmTLS
	// configTLS
	var dialopts []netx.DialXOption

	dialopts = append(dialopts, netx.DialX_WithTimeout(connectTimeout), netx.DialX_WithTLSNextProto(nextProto...))

	if https {
		if gmTLS {
			dialopts = append(dialopts, netx.DialX_WithGMTLSConfig(&gmtls.Config{
				GMSupport:          &gmtls.GMSupport{WorkMode: gmtls.ModeAutoSwitch},
				NextProtos:         nextProto,
				ServerName:         host,
				InsecureSkipVerify: !option.VerifyCertificate,
			}))
		} else {
			dialopts = append(dialopts, netx.DialX_WithTLSConfig(&gmtls.Config{
				NextProtos:         nextProto,
				ServerName:         host,
				InsecureSkipVerify: !option.VerifyCertificate,
			}))
		}
		dialopts = append(dialopts, netx.DialX_WithGMTLSSupport(gmTLS), netx.DialX_WithTLS(https), netx.DialX_WithGMTLSOnly(onlyGMTLS), netx.DialX_WithGMTLSPrefer(preferGMTLS))

		if clientHelloSpec != nil {
			dialopts = append(dialopts, netx.DialX_WithClientHelloSpec(clientHelloSpec))
		} else if randomJA3FingerPrint {
			spec, err := utls.UTLSIdToSpec(utls.HelloRandomizedALPN)
			if err == nil {
				clientHelloSpec = &spec
				dialopts = append(dialopts, netx.DialX_WithClientHelloSpec(&spec))
			} else {
				log.Debugf("generate random JA3 fingerprint failed: %v", err)
			}
		}
		if sni != nil {
			dialopts = append(dialopts, netx.DialX_WithSNI(*sni))
		}
	}

	if forceProxy {
		dialopts = append(dialopts, netx.DialX_WithForceProxy(forceProxy))
	}

	if len(proxy) > 0 {
		dialopts = append(dialopts, netx.DialX_WithProxy(proxy...))
	}

	// 初次连接需要的
	// retry use DialX
	dnsStart := time.Now()
	dnsEnd := time.Now()
	dialTraceInfo := netx.NewDialXTraceInfo()
	dialopts = append(
		dialopts,
		netx.DialX_WithTimeoutRetry(maxRetryTimes),
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
		netx.DialX_WithDialTraceInfo(dialTraceInfo),
	)

	if dialer != nil {
		dialopts = append(dialopts, netx.DialX_WithDialer(dialer))
	}

	if option.OverrideEnableSystemProxyFromEnv {
		dialopts = append(dialopts, netx.DialX_WithEnableSystemProxyFromEnv(option.EnableSystemProxyFromEnv))
	}

	if len(option.ExtendDialOption) > 0 {
		dialopts = append(dialopts, option.ExtendDialOption...)
	}

	cacheKey := &connectKey{
		proxy:           proxy,
		scheme:          reqSchema,
		addr:            originAddr,
		https:           option.Https,
		gmTls:           option.GmTLS,
		clientHelloSpec: clientHelloSpec,
	}
	if sni != nil {
		cacheKey.sni = *sni
	}
	haveNativeHTTPRequestInstance := reqIns != nil
	if haveNativeHTTPRequestInstance {
		httpctx.SetRequestHTTPS(reqIns, https)
	}
RECONNECT:
	if enableHttp3 {
		http3Conn, err := getHTTP3Conn(ctx, originAddr, dialopts...)
		if err != nil {
			return nil, err
		}
		_, responsePacket, err := doHttp3Request(ctx, http3Conn, requestPacket)

		httpctx.SetBareResponseBytes(reqIns, responsePacket)
		response.RawPacket = responsePacket
		return response, nil
	} else if withConnPool {
		conn, err = connPool.getIdleConn(cacheKey, dialopts...)
	} else {
		conn, err = netx.DialX(originAddr, dialopts...)
	}

	traceInfo.DNSTime = dnsEnd.Sub(dnsStart) // safe
	traceInfo.ParseDialXTraceInfo(dialTraceInfo)
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
			tried := make(map[string]struct{})
			merged := make([]string, len(legacyProxy)+len(proxy))
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
	if haveNativeHTTPRequestInstance {
		httpctx.SetRemoteAddr(reqIns, response.RemoteAddr)
	}
	response.PortIsOpen = true

	if enableHttp2 {
		pc := conn.(*persistConn)
		if pc.cacheKey.scheme != H2 { // http2 downgrade to http1.1
			enableHttp2 = false
			withConnPool = false // downgrade can not with conn pool
			method, uri, _ := GetHTTPPacketFirstLine(requestPacket)
			requestPacket = ReplaceHTTPPacketFirstLine(requestPacket, strings.Join([]string{method, uri, "HTTP/1.1"}, " "))
		} else {
			h2Conn := pc.alt
			if h2Conn == nil {
				return nil, utils.Error("conn h2 Processor is nil")
			}

			h2Stream, err := h2Conn.newStream(reqIns, requestPacket)
			if err != nil {
				if err == CreateStreamAfterGoAwayErr {
					goto RECONNECT
				} else {
					return nil, err
				}
			}

			currentRPS.Add(1)
			if err := h2Stream.doRequest(); err != nil {
				if h2Stream.ID == 1 { // first stream
					return nil, err
				} else {
					goto RECONNECT
				}
			}
			serverStart := time.Now()
			h2Stream.SetReadFirstFrameCallback(func() {
				traceInfo.ServerTime = time.Now().Sub(serverStart)
			})

			resp, responsePacket, err := h2Stream.waitResponse(timeout)
			_ = resp
			if err != nil {
				if conn.(*persistConn).shouldRetryRequest(err) {
					goto RECONNECT
				} else {
					return nil, err
				}
			}
			httpctx.SetBareResponseBytes(reqIns, responsePacket)
			response.RawPacket = responsePacket

			err = failureChecker(response)
			return response, err
		}
	}
	//log.Infof("dns time + dial time cost: %v", time.Since(dnsStart))
	var multiResponses []*http.Response
	var isMultiResponses bool
	var firstResponse *http.Response
	var responseRaw bytes.Buffer
	var rawBytes []byte

	if withConnPool {
		// 连接池分支
		pc := conn.(*persistConn)
		writeErrCh := make(chan error, 2)
		if option.BeforeDoRequest != nil {
			requestPacket = option.BeforeDoRequest(requestPacket)
		}
		if oldVersionProxyChecking {
			requestPacket, err = BuildLegacyProxyRequest(requestPacket, https)
			if err != nil {
				return nil, err
			}
		}

		resc := make(chan responseInfo)
		pc.reqCh <- requestAndResponseCh{
			reqPacket:   requestPacket,
			ch:          resc,
			reqInstance: reqIns,
			option:      option,
			writeErrCh:  writeErrCh,
		}
		pc.writeCh <- writeRequest{reqPacket: requestPacket, ch: writeErrCh, reqInstance: reqIns, options: option}
		//beforeLog(option)
		select {
		case re := <-resc:
			// get response
			if re.err != nil && len(rawBytes) == 0 { // get some bytes but get error too
				if pc.shouldRetryRequest(re.err) {
					goto RECONNECT
				}
				return nil, re.err
			}
			firstResponse = re.resp
			rawBytes = re.respBytes
			response.MultiResponse = false
			traceInfo.ServerTime = re.info.ServerTime
			// Mark BodyStreamReaderHandler as handled in conn pool mode
			// to prevent the defer from calling it again
			if option != nil && option.BodyStreamReaderHandler != nil {
				bodyStreamReaderHandled.Set()
			}
		case <-pc.ctx.Done(): // if persistConn closed before read response , check error can retry or not
			if pc.closed == nil {
				return nil, utils.Error("BUG: closeCh but closed is nil")
			}
			if pc.shouldRetryRequest(pc.closed) {
				goto RECONNECT
			}
			return nil, pc.closed
		}
		//afterLog(option)

	} else {
		// 不使用连接池分支
		if conn != nil {
			readConnEndCtx, readConnEnd := context.WithCancel(ctx)
			defer readConnEnd()
			go func() {
				<-readConnEndCtx.Done()
				conn.Close()
			}()
		}
		// 写报文
		if option.BeforeDoRequest != nil {
			requestPacket = option.BeforeDoRequest(requestPacket)
		}

		if haveNativeHTTPRequestInstance {
			httpctx.SetBareRequestBytes(reqIns, requestPacket)
		}
		currentRPS.Add(1)

		if oldVersionProxyChecking {
			requestPacket, err = BuildLegacyProxyRequest(requestPacket, https)
			if err != nil {
				return response, err
			}
		}

		if option.EnableRandomChunked {
			chunkSender, err := option.GetOrCreateChunkSender()
			if err != nil {
				return response, errors.Wrap(err, "get or create chunk sender failed")
			}
			err = chunkSender.Send(requestPacket, conn)
		} else {
			_, err = conn.Write(requestPacket)
		}

		if err != nil {
			return response, errors.Wrap(err, "write request failed")
		}
		// TeeReader 用于畸形响应包: 即 ReadHTTPResponseFromBufioReader 无法解析但是conn中存在数据的情况
		if option.DefaultBufferSize <= 0 {
			option.DefaultBufferSize = 4096
		}

		var mirrorWriter io.Writer = &responseRaw

		// BodyStreamReaderHandler for non-pool connection
		// Note: connection pool mode also supports BodyStreamReaderHandler (see conn_pool.go readLoop)
		if option != nil && option.BodyStreamReaderHandler != nil {
			reader, writer := utils.NewBufPipe(nil)
			defer func() {
				utils.Debug(func() {
					log.Infof("close reader and writer")
				})
				writer.Close()
			}()
			waitBodyStreamReaderHandledCallEnd.Add(1)
			//startHandlerStream := time.Now()
			//onceForHeader := new(sync.Once)
			//onceForBody := new(sync.Once)
			go func() {
				bodyReader, bodyWriter := utils.NewBufPipe(nil)
				defer func() {
					bodyWriter.Close()
					if err := recover(); err != nil {
						log.Errorf("BodyStreamReaderHandler panic: %v", err)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
					waitBodyStreamReaderHandledCallEnd.Done()
				}()

				packetReader := bufio.NewReader(reader)
				responseHeader := bytes.NewBufferString("")
				var responseHeaderWriter io.Writer = responseHeader
				if option.NoBodyBuffer {
					responseHeaderWriter = io.MultiWriter(responseHeaderWriter, &responseRaw)
				}

				for {
					line, err := utils.BufioReadLine(packetReader)
					if err != nil {
						if err != io.EOF {
							log.Errorf("BodyStreamReaderHandler read response failed: %s", err)
						}
						bodyWriter.Close()
						break
					}
					//onceForHeader.Do(func() {
					//	log.Infof("stream handler start to handle header: %v", time.Since(startHandlerStream))
					//})

					responseHeaderWriter.Write(line)
					responseHeaderWriter.Write([]byte("\r\n"))
					if len(line) == 0 {
						go func() {
							io.Copy(bodyWriter, packetReader)
							//io.Copy(bodyWriter, io.TeeReader(packetReader, utils.FirstWriter(func(i []byte) {
							//	onceForBody.Do(func() {
							//		log.Infof("stream handler start to handle body: %v", time.Since(startHandlerStream))
							//	})
							//})))
							bodyWriter.Close()
						}()
						break
					}
				}
				if err != nil {
					log.Warnf("Normal handle BodyStreamReaderHandler read response failed: %s", err)
				} else {
					bodyStreamReaderHandled.Set()
					option.BodyStreamReaderHandler(responseHeader.Bytes(), bodyReader)
				}
			}()
			if option.NoBodyBuffer {
				mirrorWriter = writer
			} else {
				mirrorWriter = io.MultiWriter(&responseRaw, writer)
			}
		}

		httpResponseReader := bufio.NewReaderSize(io.TeeReader(conn, mirrorWriter), option.DefaultBufferSize)

		//log.Infof("dns time + dial time cost + write request finished: %v", time.Since(dnsStart))
		// 服务器响应第一个字节
	READ:
		serverTimeStart := time.Now()
		_ = conn.SetReadDeadline(serverTimeStart.Add(timeout))
		firstByte, err := httpResponseReader.Peek(1)
		if err != nil {
			return response, errors.Wrap(err, "read first byte failed")
		}
		//log.Infof("dns time + dial time cost + write request finished + peek 1: %v", time.Since(dnsStart))
		//log.Infof("[lowhttp] first byte in %v", time.Since(serverTimeStart))

		// 检查是否是 TLS 握手错误的特定序列
		if firstByte[0] == 0x15 {
			// 尝试读取更多字节以确认是否是特定的 TLS 错误
			tlsHeader, err := httpResponseReader.Peek(6)
			if err == nil && bytes.Equal(tlsHeader, []byte("\x15\x03\x01\x00\x02\x02")) {
				return response, utils.Errorf("tls record header error detected... raw: %v", spew.Sdump(tlsHeader))
			}
		}

		traceInfo.ServerTime = time.Since(serverTimeStart)

		firstResponse, err = utils.ReadHTTPResponseFromBufioReader(httpResponseReader, reqIns)
		if err != nil {
			log.Warnf("[lowhttp] read response failed: %s", err)
		}

		if firstAuth && firstResponse != nil && firstResponse.StatusCode == http.StatusUnauthorized {
			if authHeader := IGetHeader(firstResponse, "WWW-Authenticate"); len(authHeader) > 0 {
				if auth := GetHttpAuth(authHeader[0], option); auth != nil {
					authReq, err := auth.Authenticate(conn, option)
					if err == nil {
						_, err := conn.Write(authReq)
						responseRaw.Reset() // 发送认证请求成功，清空缓冲区
						if err != nil {
							return response, errors.Wrap(err, "write request failed")
						}
						firstAuth = false
						goto READ
					}
				}
			}
		}

		response.ResponseBodySize = httpctx.GetResponseBodySize(reqIns)
		respClose := false
		if firstResponse != nil {
			respClose = firstResponse.Close
		}
		if firstResponse != nil {
			multiResponses = append(multiResponses, firstResponse)
		}

		if firstResponse == nil || respClose {
			if len(responseRaw.Bytes()) == 0 {
				return response, errors.Wrap(err, "empty result.")
			} else { // peek 到了数据,但是无法解析,说明是畸形响应包
				stableTimeout := timeout
				if respClose && timeout < 1*time.Second { // 取设置timeout与1s的较小值
					stableTimeout = 1 * time.Second
				}
				restBytes, _ := utils.ReadUntilStable(httpResponseReader, conn, stableTimeout, 300*time.Millisecond)
				if len(restBytes) > 0 {
					if len(restBytes) > 256 {
						restBytes = restBytes[:256]
					}
					log.Warnf("unhandled rest data in connection: %#v ...", string(restBytes))
				}
			}
		} else {
			firstResponse.Request = reqIns

			// handle response
			for noFixContentLength && !option.NoReadMultiResponse { // 尝试读取pipeline/smuggle响应包
				// log.Infof("checking next(pipeline/smuggle) response...")
				nextResponse, err := utils.ReadHTTPResponseFromBufioReaderConn(httpResponseReader, conn, nil)
				var nextRespClose bool
				if nextResponse != nil {
					nextRespClose = nextResponse.Close
				}
				if err != nil || nextRespClose {
					if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) { // 停止读取
						break
					}
					// read second response rest in buffer
					stableTimeout := timeout
					if nextRespClose && timeout < 1*time.Second {
						stableTimeout = 1 * time.Second
					}
					restBytes, _ := utils.ReadUntilStable(httpResponseReader, conn, stableTimeout, 300*time.Millisecond)
					if len(restBytes) > 0 {
						if len(restBytes) > 256 {
							restBytes = restBytes[:256]
						}
						log.Errorf("unhandled rest data in connection: %#v ...", string(restBytes))
					}
					break
				}

				if nextResponse != nil {
					multiResponses = append(multiResponses, nextResponse)
					isMultiResponses = true
					response.MultiResponse = true
				}
			}
		}
		response.MultiResponseInstances = multiResponses
		rawBytes = responseRaw.Bytes()
	}

	if option.EnableMaxContentLength && maxContentLength > 0 {
		if body := GetHTTPPacketBody(rawBytes); len(body) > maxContentLength {
			rawBytes = ReplaceHTTPPacketBodyRaw(rawBytes, body[:maxContentLength], true)
		}
	}
	if haveNativeHTTPRequestInstance {
		httpctx.SetBareResponseBytes(reqIns, rawBytes)
	}

	// 更新 cookiejar 中的 cookie
	if session != nil && firstResponse != nil {
		cookiejar.SetCookies(urlIns, firstResponse.Cookies())
	}

	// 将请求中的cookie更新到cookiejar中
	if session != nil && reqIns != nil {
		reqCookies := reqIns.Cookies()
		for _, cookie := range reqCookies {
			// 限制domain为当前域, path为当前路径
			cookie.Domain = urlIns.Hostname()
			cookie.Path = urlIns.Path
		}
		cookiejar.SetCookies(urlIns, reqCookies)
	}

	response.BareResponse = rawBytes
	/*
		FixHTTPResponse will be executed when:
		1. SMUGGLE: noFixContentLength is false
		2. PIPELINE(multi response)
	*/
	if !noFixContentLength && !isMultiResponses {
		// return responseRaw.Bytes(), nil
		header := GetHTTPPacketHeader(rawBytes, "Content-Type")
		response.OriginContentType = header
		ContentTypeOptions := GetHTTPPacketHeader(rawBytes, "X-Content-Type-Options")
		response.IsSetContentTypeOptions = ContentTypeOptions != ""
		headerHash := codec.Md5(header)
		/*
			todo: need split fix http response, fix content-type need as option
		*/
		rspRaw, _, err := FixHTTPResponse(rawBytes)
		if err != nil {
			log.Errorf("fix http response failed: %s", err)
			response.RawPacket = rawBytes
			response.IsFixContentType = false
			return response, nil
		}
		fixHeader := GetHTTPPacketHeader(rspRaw, "Content-Type")
		md5 := codec.Md5(fixHeader)
		response.IsFixContentType = headerHash != md5
		if response.IsFixContentType {
			response.FixContentType = fixHeader
		}
		response.RawPacket = rspRaw

		err = failureChecker(response)
		return response, err
	}

	// 如果不修复的话，默认服务器返回的东西也有点复杂，不适合做其他处理
	response.RawPacket = rawBytes

	err = failureChecker(response)
	return response, err
}
