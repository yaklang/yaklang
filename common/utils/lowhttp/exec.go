package lowhttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var (
	_systemEtcHosts = make(map[string]string)
	systemEtcOnce   = sync.Once{}
)

// maxReconnectTimes caps how many times a single request may rebuild its
// connection. Stale pooled connections need a retry or two; an origin that
// tears down every connection on sight needs to surface as an error instead,
// so the caller can fall back to another protocol.
const maxReconnectTimes = 3

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

func resolveProxyTargetFromEtcHosts(host string, port int, etcHosts map[string]string) (string, bool) {
	if host == "" || port <= 0 || len(etcHosts) == 0 {
		return "", false
	}

	resolvedHost, ok := netx.ResolveHostByTemporaryHosts(host, etcHosts)
	if !ok || resolvedHost == "" || strings.EqualFold(resolvedHost, host) {
		return "", false
	}

	return utils.HostPort(resolvedHost, port), true
}

func HTTP(opts ...LowhttpOpt) (*LowhttpResponse, error) {
	option := NewLowhttpOption()
	for _, opt := range opts {
		opt(option)
	}

	if !option.DisableSession {
		if option.Session == "" {
			option.Session = uuid.NewString()
			defer RemoveCookiejar(option.Session)
		}
		opts = append(opts, WithSession(option.Session))
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
		repairedMethodRejectedRedirect := false
		recordRedirect := func(rsp *LowhttpResponse) *RedirectFlow {
			flow := &RedirectFlow{
				IsHttps:    rsp.Https,
				Request:    rsp.RawRequest,
				Response:   rsp.RawPacket,
				RespRecord: rsp,
			}
			redirectRawPackets = append(redirectRawPackets, flow)
			rsp.RedirectRawPackets = redirectRawPackets
			return flow
		}

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
			statusCode := GetStatusCodeFromResponse(lastPacket.Response)
			originRequest := r
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

			// Clear the stale NativeHTTPRequestInstance so HTTPWithoutRetry
			// re-parses reqIns from the redirected packet instead of reusing
			// the original request (e.g. POST) that no longer matches.
			newOpts := append(opts, WithHttps(forceHttps), WithHost(nextHost), WithPort(nextPort), WithRequest(r), WithNativeHTTPRequestInstance(nil))
			response, err = HTTPWithoutRedirect(newOpts...)
			if err != nil {
				log.Errorf("met error in redirect: %v", err)
				response.RawPacket = lastPacket.Response // 保留原始报文
				return response, nil
			}
			if response == nil {
				return response, nil
			}

			responseRaw := recordRedirect(response)

			// Some servers reject the browser-style redirect method with a 400 or 405.
			// Retry the same target once, using the opposite method policy and the
			// request from before this redirect (including its original body).
			// This attempt is part of the current hop, not another redirect.
			redirectStatus := GetStatusCodeFromResponse(responseRaw.Response)
			if !repairedMethodRejectedRedirect && (redirectStatus == http.StatusBadRequest || redirectStatus == http.StatusMethodNotAllowed) {
				rewriteToGet := shouldRewriteRedirectToGet(statusCode, GetHTTPRequestMethod(originRequest))
				repairRequest, repairErr := buildRedirectRequestWithMethod(targetUrl, originRequest, lastPacket.IsHttps, !rewriteToGet)
				if repairErr != nil {
					log.Errorf("cannot repair rejected redirect method: %v", repairErr)
				} else if !bytes.Equal(repairRequest, r) {
					repairedMethodRejectedRedirect = true
					repairOpts := append(opts, WithHttps(forceHttps), WithHost(nextHost), WithPort(nextPort), WithRequest(repairRequest), WithNativeHTTPRequestInstance(nil))
					repairResponse, repairErr := HTTPWithoutRedirect(repairOpts...)
					if repairErr != nil {
						log.Errorf("met error repairing rejected redirect method: %v", repairErr)
						return response, nil
					}
					if repairResponse == nil {
						return response, nil
					}
					r = repairRequest
					response = repairResponse
					responseRaw = recordRedirect(response)
				}
			}

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
			rspRaw, err := FixHTTPResponsePacket(rawBytes)
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
		gmTLSCipherSuites       = option.GmTLSCipherSuites
		gmTLSDisableCompatMode  = option.GmTLSDisableCompatMode
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
		afterSaveHTTPFlowList   = option.AfterSaveHTTPFlowHandler
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
		reqIns                  = option.NativeHTTPRequestInstance
		maxContentLength        = option.MaxContentLength
		randomJA3FingerPrint    = option.RandomJA3FingerPrint
		clientHelloSpec         = option.ClientHelloSpec
		tlsFingerprint          = option.TLSFingerprint
		http2Fingerprint        = option.HTTP2Fingerprint
		dialer                  = option.Dialer
		fixQueryEscape          = option.FixQueryEscape
	)
	if clientHelloSpec != nil {
		tlsFingerprint = ""
	} else if tlsFingerprint == "" && randomJA3FingerPrint {
		tlsFingerprint = netx.DefaultTLSFingerprint
	}
	if tlsFingerprint != "" {
		_, err := netx.GetClientHelloProfile(tlsFingerprint)
		if err != nil {
			return response, err
		}
	}
	// HTTP/2 framing is opt-in on its own: selecting a TLS fingerprint must not
	// change it, because the default framing deliberately accommodates servers
	// that reject browser-style HEADERS.
	if http2Fingerprint != "" {
		if _, err := getHTTP2Profile(http2Fingerprint); err != nil {
			return response, err
		}
	}

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

	if connectTimeout <= 0 {
		connectTimeout = 2 * time.Second
	}
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
	option.bodyStreamReaderHandled = bodyStreamReaderHandled
	var streamBodyReaderCh chan io.ReadCloser
	var streamHandlerDone chan struct{}
	defer func() {
		if option != nil && option.BodyStreamReaderHandler != nil {
			waitStreamHandlerDone(streamHandlerDone, streamBodyReaderCh, 2*time.Second, "non-pool stream handler")
		}
		if option != nil && option.BodyStreamReaderHandler != nil && !bodyStreamReaderHandled.IsSet() {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("BodyStreamReaderHandler fallback panic: %v", r)
					}
				}()
				r, w := utils.NewPipe()
				w.Close()
				option.BodyStreamReaderHandler([]byte{}, r)
			}()
		}
	}()

	// ctx
	if ctx == nil {
		if reqIns != nil {
			ctx = reqIns.Context()
		}
		if ctx == nil {
			ctx = context.Background()
		}
	}
	if reqIns != nil {
		*reqIns = *reqIns.WithContext(ctx)
	}
	// fix some field
	response.Source = source
	response.Payloads = payloads
	response.Tags = tags
	if len(afterSaveHTTPFlowList) > 0 {
		response.AfterSaveHTTPFlowHandler = append([]func(*schema.HTTPFlow){}, afterSaveHTTPFlowList...)
	}

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

	// 获取 cookiejar（仅在 session 非空时启用自动 cookie 管理）
	var cookiejar http.CookieJar
	if session != "" {
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
	if len(proxy) > 0 && option.PreferEtcHostsBeforeProxy {
		if mappedAddr, ok := resolveProxyTargetFromEtcHosts(host, port, dnsHosts); ok {
			originAddr = mappedAddr
		}
	}

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
	if option.BorrowFixedRequestPacket {
		requestPacket = FixHTTPPacketCRLFBorrowed(requestPacket, noFixContentLength)
	} else {
		requestPacket = FixHTTPPacketCRLF(requestPacket, noFixContentLength)
	}

	if fixQueryEscape {
		requestPacket = FixHTTPPacketQueryEscape(requestPacket)
	}
	response.RawRequest = requestPacket
	response.Http2 = enableHttp2
	_ = withConnPool // transport reads option.WithConnPool directly

	// https://github.com/mattn/go-ieproxy
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

	// buildDialOpts constructs the dial option slice for the given ALPN
	// next-protocol list.  It is called once for the initial request and
	// again when a protocol downgrade (H2→H1) requires a fresh connection
	// with http/1.1 ALPN instead of h2.
	dnsStart := time.Now()
	var dnsEndNano atomic.Int64
	dnsEndNano.Store(time.Now().UnixNano())
	var dnsEndOnce sync.Once
	dialTraceInfo := netx.NewDialXTraceInfo()

	buildDialOpts := func(np []string) []netx.DialXOption {
		var opts []netx.DialXOption
		opts = append(opts, netx.DialX_WithTimeout(connectTimeout), netx.DialX_WithAppendTLSNextProto(np...))

		if https {
			if gmTLS {
				gmCfg := &gmtls.Config{
					GMSupport:          &gmtls.GMSupport{WorkMode: gmtls.ModeAutoSwitch},
					NextProtos:         np,
					ServerName:         host,
					InsecureSkipVerify: !option.VerifyCertificate,
				}
				if len(gmTLSCipherSuites) > 0 {
					gmCfg.CipherSuites = gmTLSCipherSuites
				}
				opts = append(opts, netx.DialX_WithGMTLSConfig(gmCfg))
			} else {
				opts = append(opts, netx.DialX_WithTLSConfig(&gmtls.Config{
					NextProtos:         np,
					ServerName:         host,
					InsecureSkipVerify: !option.VerifyCertificate,
				}))
			}
			opts = append(opts,
				netx.DialX_WithGMTLSSupport(gmTLS),
				netx.DialX_WithTLS(https),
				netx.DialX_WithGMTLSOnly(onlyGMTLS),
				netx.DialX_WithGMTLSPrefer(preferGMTLS),
				netx.DialX_WithGMTLSDisableCompatMode(gmTLSDisableCompatMode),
			)

			if clientHelloSpec != nil {
				opts = append(opts, netx.DialX_WithClientHelloSpec(clientHelloSpec))
			} else if tlsFingerprint != "" {
				opts = append(opts, netx.DialX_WithTLSFingerprint(tlsFingerprint))
			}
			if sni != nil {
				opts = append(opts, netx.DialX_WithSNI(*sni))
			}
		}

		if forceProxy {
			opts = append(opts, netx.DialX_WithForceProxy(forceProxy))
		}

		if len(proxy) > 0 {
			opts = append(opts, netx.DialX_WithProxy(proxy...))
		}

		opts = append(
			opts,
			netx.DialX_WithTimeoutRetry(maxRetryTimes),
			netx.DialX_WithTimeoutRetryWaitRange(
				retryWaitTime,
				retryMaxWaitTime,
			),
			netx.DialX_WithDNSOptions(
				netx.WithDNSOnFinished(func() {
					dnsEndOnce.Do(func() {
						dnsEndNano.Store(time.Now().UnixNano())
					})
				}),
				netx.WithDNSServers(dnsServers...),
				netx.WithTemporaryHosts(dnsHosts),
			),
			netx.DialX_WithDialTraceInfo(dialTraceInfo),
		)

		if dialer != nil {
			opts = append(opts, netx.DialX_WithDialer(dialer))
		}

		if option.OverrideEnableSystemProxyFromEnv {
			opts = append(opts, netx.DialX_WithEnableSystemProxyFromEnv(option.EnableSystemProxyFromEnv))
		}

		if option.StrongHost != "" {
			opts = append(opts, netx.DialX_WithStrongHostMode(option.StrongHost))
		}

		if len(option.ExtendDialOption) > 0 {
			opts = append(opts, option.ExtendDialOption...)
		}

		return opts
	}

	dialopts := buildDialOpts(nextProto)

	cacheKey := &connectKey{
		proxy:            proxy,
		scheme:           reqSchema,
		addr:             originAddr,
		https:            option.Https,
		gmTls:            option.GmTLS,
		clientHelloSpec:  clientHelloSpec,
		tlsFingerprint:   tlsFingerprint,
		http2Fingerprint: http2Fingerprint,
	}
	if sni != nil {
		cacheKey.sni = *sni
	}

	if option.StrongHost != "" {
		cacheKey.strongHost = option.StrongHost
	}

	haveNativeHTTPRequestInstance := reqIns != nil
	if haveNativeHTTPRequestInstance {
		httpctx.SetRequestHTTPS(reqIns, https)
	}

	// canReconnect bounds the RECONNECT loop below, but only for the failure
	// mode that can actually spin forever.
	//
	// A pooled connection dying between requests — the server closed an idle
	// keep-alive connection, or a read failed mid-flight — says nothing about
	// whether the origin is healthy, and a busy pool can hand out several stale
	// connections in a row. Those retries stay unbounded, as they have always
	// been; capping them makes ordinary traffic fail under load.
	//
	// What must be bounded is an origin that tears down the connection the
	// moment it receives a request — h2 fingerprinting defenses do exactly
	// this. Unbounded, the caller never receives an error, so it never gets to
	// fall back to HTTP/1.1 and the request simply hangs.
	reconnectTimes := 0
	canReconnect := func(err error) bool {
		var poolReadErr connPoolReadFromServerError
		if errors.Is(err, errServerClosedIdle) || errors.As(err, &poolReadErr) {
			return true
		}
		if reconnectTimes >= maxReconnectTimes {
			log.Warnf("lowhttp: giving up after %d reconnects to %v: %v", reconnectTimes, cacheKey.addr, err)
			return false
		}
		reconnectTimes++
		return true
	}
	var (
		rawBytes        []byte
		firstResponse   *http.Response
		multiResponses  []*http.Response
		isMultiResponses bool
	)
// ── Transport dispatch ────────────────────────────────────────────────────
//
// The orchestration layer selects a transport based on the requested protocol,
// executes the request, and handles protocol-level downgrades (H2→H1) and
// stale-connection reconnects.  Each transport owns its connection management
// and stream lifecycle.

	tr := &transportRequest{
		option:     option,
		reqIns:     reqIns,
		packet:     requestPacket,
		dialOpts:   dialopts,
		cacheKey:   cacheKey,
		connPool:   connPool,
		traceInfo:  traceInfo,
		originAddr: originAddr,
		timeout:    timeout,
	}

	// Select initial transport based on protocol flags.
	var transport Transport
	if enableHttp3 {
		transport = NewH3Transport()
	} else if enableHttp2 {
		transport = NewH2Transport(connPool)
	} else {
		transport = NewH1Transport(connPool)
	}

	// Execute request with downgrade and reconnect handling.
	maxDowngrades := 1 // H2→H1 at most once
	downgrades := 0
RECONNECT:
	tResult, err := transport.RoundTrip(ctx, tr)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		// Protocol downgrade: H2 not available on this server.
		if transport.ShouldDowngrade(err) && downgrades < maxDowngrades {
			downgrades++
			enableHttp2 = false
			response.Http2 = false
			withConnPool = false // legacy: downgrade bypasses pool; H1 transport re-reads option.WithConnPool
			tr.cacheKey.scheme = H1
			// Rebuild dial options with http/1.1 ALPN so the new H1 connection
			// does not negotiate h2 and hit the same tarpit/killing origin.
			dialopts = buildDialOpts([]string{H1})
			tr.dialOpts = dialopts
			method, uri, _ := GetHTTPPacketFirstLine(requestPacket)
			requestPacket = ReplaceHTTPPacketFirstLine(requestPacket, strings.Join([]string{method, uri, "HTTP/1.1"}, " "))
			tr.packet = requestPacket
			transport = NewH1Transport(connPool)
			goto RECONNECT
		}
		// Reconnect: stale pooled connection or retryable stream error.
		if isReconnectError(err) {
			underlying := reconnectErrorUnwrap(err)
			if canReconnect(underlying) {
				goto RECONNECT
			}
			return nil, underlying
		}
		if transport.CanRetry(reqIns, err) && (option.bodyStreamReaderHandled == nil || !option.bodyStreamReaderHandled.IsSet()) {
			if canReconnect(err) {
				goto RECONNECT
			}
		}
		return nil, err
	}

	// Populate trace info from dial (dialTraceInfo was filled during RoundTrip).
	traceInfo.DNSTime = time.Unix(0, dnsEndNano.Load()).Sub(dnsStart)
	traceInfo.ParseDialXTraceInfo(dialTraceInfo)

	// Populate response fields from transport result.
	rawBytes = tResult.rawBytes
	firstResponse = tResult.firstResponse
	multiResponses = tResult.multiResponses
	isMultiResponses = tResult.isMultiResponse
	response.MultiResponse = isMultiResponses
	response.RemoteAddr = tResult.remoteAddr
	response.PortIsOpen = tResult.portIsOpen
	if haveNativeHTTPRequestInstance {
		httpctx.SetRemoteAddr(reqIns, response.RemoteAddr)
	}
	response.MultiResponseInstances = multiResponses
	response.ResponseBodySize = httpctx.GetResponseBodySize(reqIns)

	if option.EnableMaxContentLength && maxContentLength > 0 {
		if _, body := SplitHTTPHeadersAndBodyFromPacketView(rawBytes); len(body) > maxContentLength {
			rawBytes = ReplaceHTTPPacketBodyRaw(rawBytes, body[:maxContentLength], true)
		}
	}
	if haveNativeHTTPRequestInstance {
		httpctx.SetBareResponseBytes(reqIns, rawBytes)
	}

	// Seed request cookies first. Leaving Domain and Path empty lets cookiejar
	// apply host-only and RFC default-path rules instead of pinning a cookie
	// from /login.php to that exact path.
	if session != "" && reqIns != nil {
		cookiejar.SetCookies(urlIns, reqIns.Cookies())
	}

	// Apply Set-Cookie after request cookies so rotations and deletions win.
	if session != "" && firstResponse != nil {
		cookiejar.SetCookies(urlIns, firstResponse.Cookies())
	}

	response.BareResponse = rawBytes
	/*
		FixHTTPResponse will be executed when:
		1. SMUGGLE: noFixContentLength is false
		2. PIPELINE(multi response)
		3. Not using NoBodyBuffer (stream mode has incomplete response body)
	*/
	if !noFixContentLength && !isMultiResponses && !option.NoBodyBuffer {
		// return responseRaw.Bytes(), nil
		header := GetHTTPPacketHeader(rawBytes, "Content-Type")
		response.OriginContentType = header
		ContentTypeOptions := GetHTTPPacketHeader(rawBytes, "X-Content-Type-Options")
		response.IsSetContentTypeOptions = ContentTypeOptions != ""
		headerHash := codec.Md5(header)
		/*
			todo: need split fix http response, fix content-type need as option
		*/
		var rspRaw []byte
		var err error
		if option.BorrowFixedResponsePacket {
			rspRaw, err = FixHTTPResponsePacketBorrowed(rawBytes)
		} else {
			rspRaw, err = FixHTTPResponsePacket(rawBytes)
		}
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
		response.ResponsePacketFixed = true

		err = failureChecker(response)
		return response, err
	}

	// 如果不修复的话，默认服务器返回的东西也有点复杂，不适合做其他处理
	response.RawPacket = rawBytes

	err = failureChecker(response)
	return response, err
}
