package minimartian

import (
	"io"
	"net"
	"net/http"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (p *Proxy) doHTTPRequest(ctx *Context, req *http.Request) (*http.Response, error) {
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
	return p.execLowhttp(ctx, req)
}

func (p *Proxy) execLowhttp(ctx *Context, req *http.Request) (*http.Response, error) {
	bareBytes := httpctx.GetRequestBytes(req)
	reqBytes := lowhttp.FixHTTPRequest(bareBytes)

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
	useConnPool := !isStrongHostMode
	opts := append(
		p.lowhttpConfig,
		lowhttp.WithRequest(reqBytes),
		lowhttp.WithHttp2(isH2),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithGmTLS(isGmTLS),
		lowhttp.WithGmTLSOnly(p.gmTLSOnly),
		lowhttp.WithGmTLSPrefer(p.gmPrefer),
		lowhttp.WithConnPool(useConnPool),
		lowhttp.WithSaveHTTPFlow(false),
		lowhttp.WithNativeHTTPRequestInstance(req),
		lowhttp.WithMaxContentLength(MaxContentLength),
	)

	// Use custom connection pool if available and not in strong host mode
	// In strong host mode, connections must not be reused from pool
	if p.connPool != nil && !isStrongHostMode {
		opts = append(opts, lowhttp.ConnPool(p.connPool))
	}

	if p.dialer != nil {
		opts = append(opts, lowhttp.WithDialer(p.dialer))
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

	if connectedPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort); connectedPort > 0 {
		opts = append(opts, lowhttp.WithPort(connectedPort))
	}

	connectedHost := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost)

	// Determine the hostname to use for strong host mode
	if connectedHost != "" {
		opts = append(opts, lowhttp.WithHost(connectedHost))
		if isStrongHostMode {
			log.Debugf("mitm: using strong host mode, dialing with original host: %s", connectedHost)
		}

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
				opts = append(opts, lowhttp.WithExtendDialXOption(netx.DialX_WithStrongHostMode(localAddrIP)))
				log.Debugf("mitm: strong host mode with localAddr: %s", localAddrIP)
			} else {
				log.Warnf("mitm: strong host mode localAddr '%s' is not a valid IP address, ignoring", localAddrIP)
			}
		} else {
			log.Debugf("mitm: strong host mode enabled but no localAddr found in httpctx")
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
	if err != nil {
		req.RemoteAddr = ""
		httpctx.SetRemoteAddr(req, "")
		return nil, err
	}
	// set trace info
	httpctx.SetResponseTraceInfo(req, lowHttpResp.TraceInfo)

	if lowHttpResp.RemoteAddr != "" {
		httpctx.SetRemoteAddr(req, lowHttpResp.RemoteAddr)
		req.RemoteAddr = lowHttpResp.RemoteAddr
	}

	rsp, err := lowhttp.ParseBytesToHTTPResponse(lowHttpResp.RawPacket)
	if rsp != nil {
		rsp.Request = req
	}

	utils.FixHTTPResponseForGolangNativeHTTPClient(rsp)
	return rsp, err
}
