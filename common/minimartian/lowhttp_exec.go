package minimartian

import (
	"io"
	"net"
	"net/http"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (p *Proxy) doHTTPRequest(ctx *Context, req *http.Request) (*http.Response, error) {
	// Check if mock response should be used (set by mockHTTPRequest)
	if httpctx.GetShouldMockResponse(req) {
		log.Debugf("mitm: using mock response")
		mockRespBytes := httpctx.GetMockResponseBytes(req)
		if len(mockRespBytes) > 0 {
			mockRsp, err := utils.ReadHTTPResponseFromBytes(mockRespBytes, nil)
			if err != nil {
				log.Warnf("mitm: failed to parse mock response, returning 502: %v", err)
				return proxyutil.NewResponse(502, nil, req), nil
			}
			mockRsp.Request = req
			return mockRsp, nil
		}
		// No mock response bytes, return 502
		log.Warnf("mitm: mock response flag set but no response bytes, returning 502")
		return proxyutil.NewResponse(502, nil, req), nil
	}
	if ctx.SkippingRoundTrip() {
		log.Debugf("mitm: skipping round trip")
		return proxyutil.NewResponse(200, nil, req), nil
	}
	if httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsDropped) {
		log.Debugf("mitm: skipping round trip due to user manually drop")
		return proxyutil.NewResponse(200, nil, req), nil
	}

	inherit := func(i string) {
		// 从session中继承， session > httpctx
		// 可能存在session中没有，httpctx中有的情况
		sessionValue := ctx.GetSessionStringValue(i)
		if sessionValue != "" {
			httpctx.SetContextValueInfoFromRequest(req, i, ctx.GetSessionStringValue(i))
		}
	}
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedTo)
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort)
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost)
	res, err := p.execLowhttp(ctx, req)
	if err == nil && res != nil {
		httpctx.SetUpstreamRoundTripSucceeded(req, true)
	}
	return res, err
}

func parseLowHTTPResponsePacket(packet []byte) (*http.Response, error) {
	return utils.ReadHTTPResponseFromBytesWithBodyView(packet)
}

// forceHTTP11FirstLine rewrites a request packet's version marker to HTTP/1.1.
//
// A request parsed from an h2 client keeps its "HTTP/2" marker so the UI shows
// the client-facing protocol faithfully, but lowhttp reads that same marker as
// a force-h2 signal (see lowhttp.exec). Any hop that goes out over HTTP/1.1
// must therefore rewrite the wire copy, or lowhttp overrides the caller's
// choice of protocol.
func forceHTTP11FirstLine(packet []byte) []byte {
	method, uri, _ := lowhttp.GetHTTPPacketFirstLine(packet)
	return lowhttp.ReplaceHTTPPacketFirstLine(packet, method+" "+uri+" HTTP/1.1")
}

func (p *Proxy) execLowhttp(ctx *Context, req *http.Request) (*http.Response, error) {
	// PlainRequestBytes is the decoded representation used by the UI, history,
	// and plugins. It must not replace the wire packet during transparent
	// forwarding. Explicitly hijacked bytes still take precedence.
	requestBytes := httpctx.GetHijackedRequestBytes(req)
	if len(requestBytes) == 0 {
		requestBytes = httpctx.GetBareRequestBytes(req)
	}
	if len(requestBytes) == 0 {
		requestBytes = httpctx.GetPlainRequestBytes(req)
	}
	reqBytes := lowhttp.FixHTTPRequestBorrowed(requestBytes)

	isHttps := httpctx.GetRequestHTTPS(req)

	newUrl, err := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
	if err != nil {
		return nil, err
	}

	host, port, err := utils.ParseStringToHostPort(newUrl.String())
	if err != nil {
		return nil, err
	}

	cacheKey := utils.HostPort(host, port)

	var isH2 bool

	if cached, ok := p.h2Cache.Load(cacheKey); ok {
		isH2 = cached.(bool)
	} else if p.http2 && isHttps {
		// Origin h2 capability unknown: try h2 optimistically. Every failure
		// mode below downgrades to HTTP/1.1 and negative-caches the origin:
		//  - h1-only origin: ALPN does not negotiate h2 -> instant downgrade
		//  - WAF kills the h2 conn (preface EOF/RST): closeCh -> downgrade
		//  - WAF tarpit (no SETTINGS): bounded server-preface wait -> downgrade
		//  - any other transport error: err fallback below
		// so optimistic h2 never hangs the request. The probe still runs in
		// the background to populate the cache for later requests.
		isH2 = true
		p.detectServerH2Async(cacheKey, "")
	}

	if !isH2 {
		// The upstream protocol is decided by the origin h2 cache, not by the
		// packet's version marker.
		reqBytes = forceHTTP11FirstLine(reqBytes)
	}

	isGmTLS := p.gmTLS && isHttps
	MaxContentLength := int(consts.GetGlobalMaxContentLength())
	if p.GetMaxContentLength() != 0 {
		MaxContentLength = p.maxContentLength
	}

	// In strong host mode, we must use the original host from the request
	// This is critical for transparent hijacking of tun-generated data
	// The host should be taken from ConnectedToHost which preserves the original host header
	isStrongHostMode := httpctx.GetIsStrongHostMode(req)

	// In strong host mode, disable connection pool
	// Strong host connections must not be reused from pool
	upstreamPortModified := httpctx.GetUpstreamPortIsModified(req)
	opts := append(
		p.lowhttpConfig,
		lowhttp.WithRequest(reqBytes),
		lowhttp.WithContext(req.Context()),
		lowhttp.WithHttp2(isH2),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithGmTLS(isGmTLS),
		lowhttp.WithGmTLSOnly(p.gmTLSOnly),
		lowhttp.WithGmTLSPrefer(p.gmPrefer),
		lowhttp.WithExtendReadDeadline(true),
		lowhttp.WithSaveHTTPFlow(false),
		lowhttp.WithNativeHTTPRequestInstance(req),
		lowhttp.WithDiscardIntermediateResponseBody(true),
		lowhttp.WithBorrowConnPoolResponsePacket(true),
		lowhttp.WithBorrowFixedRequestPacket(true),
		lowhttp.WithBorrowFixedResponsePacket(true),
		lowhttp.WithMaxContentLength(MaxContentLength),
	)

	if p.sniResolver != nil && isHttps {
		if sni := p.sniResolver(host); sni != nil {
			opts = append(opts, lowhttp.WithSNI(*sni))
		}
	}

	// Use custom connection pool if available and not in strong host mode
	// In strong host mode, connections must not be reused from pool

	if isStrongHostMode && p.strongHostConnPool != nil {
		opts = append(opts, lowhttp.WithConnPool(true), lowhttp.ConnPool(p.strongHostConnPool))
	} else if p.connPool != nil {
		opts = append(opts, lowhttp.WithConnPool(true), lowhttp.ConnPool(p.connPool))
	}

	if p.dialer != nil {
		opts = append(opts, lowhttp.WithDialer(p.dialer))
	}

	if proxies := p.selectProxiesForHost(host); len(proxies) > 0 {
		opts = append(opts, lowhttp.WithProxy(proxies...))
	} else {
		opts = append(opts, lowhttp.WithProxy())
	}

	//if connectedPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort); connectedPort > 0 {
	//	portValid := (connectedPort == 443 && isHttps) || (connectedPort == 80 && !isHttps)
	//	if !portValid {
	//		// 修复host和port
	//		if host := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost); host != "" {
	//			opts = append(opts, lowhttp.WithHost(host))
	//		}
	//		opts = append(opts, lowhttp.WithPort(connectedPort))
	//	}
	//}

	connectedHost := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost)
	if isStrongHostMode || !upstreamPortModified {
		connectedPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort)
		if connectedPort > 0 {
			opts = append(opts, lowhttp.WithPort(connectedPort))
		}
	}

	// Preserve the original connection host so Host-header rewriting continues to support
	// virtual-host testing. Only an explicitly edited port changes the socket target.
	if connectedHost != "" {
		opts = append(opts, lowhttp.WithHost(connectedHost))
	}

	// Host-header rewriting must not implicitly change TLS SNI.
	tlsSNI := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_TLS_SNI)
	if isHttps && tlsSNI != "" && tlsSNI != connectedHost {
		opts = append(opts, lowhttp.WithSNI(tlsSNI))
	}

	// In strong host mode, get localAddr from httpctx request context
	// The strong host mode configuration IP is the localAddr, which must be a local IP address
	if isStrongHostMode {
		// Get localAddr from httpctx - this is set from WrapperedConn's metaInfo
		localAddrIP := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_StrongHostLocalAddr)

		// Validate that localAddr is an IP address (not a hostname)
		if localAddrIP != "" {
			// Extract IP from host:port format if needed
			host, _, err := utils.ParseStringToHostPort(localAddrIP)
			if err == nil {
				localAddrIP = host
			}
			// Validate it's an IP address
			ip := net.ParseIP(utils.FixForParseIP(localAddrIP))
			if ip != nil {
				// Pass strong host mode with localAddr IP to netx dial layer
				// DialX_WithStrongHostMode expects the local IP address to bind to
				opts = append(opts, lowhttp.WithStrongHostMode(localAddrIP))
			}
		}
	}

	httpctx.SetResponseHeaderParsed(req, func(key string, value string) {
		bwr := httpctx.GetMITMFrontendReadWriter(req)
		if bwr == nil {
			return
		}

		// filter / forward to client conn via Content-Type
		if key == "content-type" {
			if ret := httpctx.GetResponseContentTypeFiltered(req); ret != nil {
				if ret(value) {
					// filtered by content-type
					httpctx.SetResponseHeaderCallback(req, func(response *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error) {
						httpctx.SetMITMSkipFrontendFeedback(req, true)
						bwr.Write(headerBytes)
						utils.FlushWriter(bwr)
						httpctx.SetResponseFinishedCallback(req, func() {
							utils.FlushWriter(bwr)
						})
						return io.TeeReader(bodyReader, bwr), nil
					})
					return
				}
			}
		}

		// content-length is too short
		if key != "content-length" && key != "transfer-encoding" {
			return
		}

		if key == "content-length" {
			if contentLength := codec.Atoi(value); contentLength < int(MaxContentLength) {
				return
			}
		}

		// set if chunked or content-length is too large
		httpctx.SetResponseHeaderCallback(req, func(response *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error) {
			writerCloser := utils.NewTriggerWriterEx(uint64(MaxContentLength), p.maxReadWaitTime, func(buffer io.ReadCloser, triggerEvent string) {
				httpctx.SetContextValueInfoFromRequest(req, triggerEvent, true)
				httpctx.SetMITMSkipFrontendFeedback(req, true)
				bwr.Write(headerBytes)
				utils.FlushWriter(bwr)
				go func() {
					_, err := utils.IOCopy(utils.WriterAutoFlush(bwr), buffer, nil)
					utils.FlushWriter(bwr)
					if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
						log.Errorf("io.Copy error: %s", err)
					}
				}()
			})
			httpctx.SetResponseFinishedCallback(req, func() {
				httpctx.SetResponseTooLargeSize(req, writerCloser.GetCount())
				writerCloser.Close()
			})
			return io.TeeReader(bodyReader, writerCloser), nil
		})
	})

	lowHttpResp, err := lowhttp.HTTPWithoutRedirect(opts...)
	if err != nil && isH2 && req.Context().Err() == nil {
		// The origin advertised h2 during detection, but the h2 upstream
		// attempt failed at transport level. Some endpoints (WAF/bot
		// mitigation, e.g. fingerprinting-protected APIs) kill non-browser
		// h2 clients right after the preface. Downgrade: remember h1 for
		// this origin and retry over HTTP/1.1 — the same behavior as Burp.
		//
		// The context check above matters: a request abandoned by the client
		// also surfaces as an error here, but says nothing about the origin's
		// h2 support. Downgrading on it would poison the cache for the whole
		// origin and burn a second, equally doomed round trip.
		log.Warnf("h2 upstream to %v failed: %v, downgrading to HTTP/1.1", cacheKey, err)
		p.h2Cache.Store(cacheKey, false)
		// Rewrite the version marker as well: leaving "HTTP/2" on the wire copy
		// makes lowhttp force h2 again and the retry rebuilds the very
		// connection that just failed.
		opts = append(opts, lowhttp.WithHttp2(false), lowhttp.WithRequest(forceHTTP11FirstLine(reqBytes)))
		lowHttpResp, err = lowhttp.HTTPWithoutRedirect(opts...)
	}
	if err != nil {
		req.RemoteAddr = ""
		httpctx.SetRemoteAddr(req, "")
		return nil, err
	}

	// If h2 was requested (the origin advertised it during detection) but the
	// connection was silently downgraded to HTTP/1.1 — ALPN mismatch, preface
	// failure, or a tarpit that never sent its SETTINGS frame — remember h1
	// for this origin so later requests skip the h2 attempt entirely.
	if isH2 && lowHttpResp != nil && !lowHttpResp.Http2 {
		log.Infof("h2 upstream to %v was downgraded to HTTP/1.1, caching h1 for this origin", cacheKey)
		p.h2Cache.Store(cacheKey, false)
	}

	// set trace info
	httpctx.SetResponseTraceInfo(req, lowHttpResp.TraceInfo)

	if lowHttpResp.RemoteAddr != "" {
		httpctx.SetRemoteAddr(req, lowHttpResp.RemoteAddr)
		req.RemoteAddr = lowHttpResp.RemoteAddr
	}

	rsp, err := parseLowHTTPResponsePacket(lowHttpResp.RawPacket)
	if rsp != nil {
		rsp.Request = req
		if err == nil {
			transferFixedResponsePacket(req, lowHttpResp)
		}
	}

	utils.FixHTTPResponseForGolangNativeHTTPClient(rsp)
	return rsp, err
}

func transferFixedResponsePacket(req *http.Request, response *lowhttp.LowhttpResponse) {
	if req == nil || response == nil || httpctx.GetResponseIsModified(req) || !response.ResponsePacketFixed || len(response.RawPacket) == 0 {
		return
	}
	packet := response.RawPacket
	response.RawPacket = nil
	response.ResponsePacketFixed = false
	httpctx.SetFixedResponseBytesOwned(req, packet)
}
