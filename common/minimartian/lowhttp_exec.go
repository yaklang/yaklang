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
	log.Infof("execLowhttp: ====== 开始执行 HTTP 请求 ======")
	log.Infof("execLowhttp: 请求方法: %s, 请求路径: %s", req.Method, req.URL.Path)

	bareBytes := httpctx.GetRequestBytes(req)
	reqBytes := lowhttp.FixHTTPRequest(bareBytes)
	log.Infof("execLowhttp: 原始请求长度: %d, 修复后请求长度: %d", len(bareBytes), len(reqBytes))

	isHttps := httpctx.GetRequestHTTPS(req)
	log.Infof("execLowhttp: 是否为 HTTPS: %v", isHttps)

	newUrl, err := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
	if err != nil {
		log.Errorf("execLowhttp: 提取 URL 失败: %v", err)
		return nil, err
	}
	log.Infof("execLowhttp: 提取的 URL: %s", newUrl.String())

	host, port, err := utils.ParseStringToHostPort(newUrl.String())
	if err != nil {
		log.Errorf("execLowhttp: 解析 Host:Port 失败: %v", err)
		return nil, err
	}
	log.Infof("execLowhttp: 解析结果 - Host: %s, Port: %d", host, port)

	cacheKey := utils.HostPort(host, port)

	var isH2 bool

	if cached, ok := p.h2Cache.Load(cacheKey); ok {
		isH2 = cached.(bool)
	}
	log.Infof("execLowhttp: 是否使用 HTTP/2: %v (cacheKey: %s)", isH2, cacheKey)

	isGmTLS := p.gmTLS && isHttps
	MaxContentLength := int(consts.GetGlobalMaxContentLength())
	if p.GetMaxContentLength() != 0 {
		MaxContentLength = p.maxContentLength
	}
	log.Infof("execLowhttp: GM TLS: %v, MaxContentLength: %d", isGmTLS, MaxContentLength)

	// In strong host mode, we must use the original host from the request
	// This is critical for transparent hijacking of tun-generated data
	// The host should be taken from ConnectedToHost which preserves the original host header
	isStrongHostMode := httpctx.GetIsStrongHostMode(req)
	log.Infof("execLowhttp: Strong Host Mode: %v", isStrongHostMode)

	// In strong host mode, disable connection pool
	// Strong host connections must not be reused from pool
	useConnPool := !isStrongHostMode
	log.Infof("execLowhttp: 使用连接池: %v", useConnPool)
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
		log.Infof("execLowhttp: 使用自定义连接池")
	} else {
		if p.connPool != nil {
			log.Infof("execLowhttp: 连接池存在但未使用 (Strong Host Mode: %v)", isStrongHostMode)
		} else {
			log.Infof("execLowhttp: 未使用自定义连接池")
		}
	}

	if p.dialer != nil {
		opts = append(opts, lowhttp.WithDialer(p.dialer))
		log.Infof("execLowhttp: 使用自定义 Dialer")
	} else {
		log.Infof("execLowhttp: 未使用自定义 Dialer")
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

	connectedPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort)
	if connectedPort > 0 {
		opts = append(opts, lowhttp.WithPort(connectedPort))
		log.Infof("execLowhttp: 设置连接端口: %d", connectedPort)
	} else {
		log.Infof("execLowhttp: 未设置连接端口，使用默认端口")
	}

	connectedHost := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost)
	log.Infof("execLowhttp: ConnectedToHost: %s", connectedHost)

	// Determine the hostname to use for strong host mode
	if connectedHost != "" {
		opts = append(opts, lowhttp.WithHost(connectedHost))
		log.Infof("execLowhttp: 设置连接主机: %s", connectedHost)
		if isStrongHostMode {
			log.Infof("execLowhttp: 使用 strong host mode，使用原始主机: %s", connectedHost)
		}
	} else {
		log.Infof("execLowhttp: 未设置 ConnectedToHost，使用 URL 中的主机: %s", host)
	}

	// In strong host mode, get localAddr from httpctx request context
	// The strong host mode configuration IP is the localAddr, which must be a local IP address
	if isStrongHostMode {
		// Get localAddr from httpctx - this is set from WrapperedConn's metaInfo
		localAddrIP := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_StrongHostLocalAddr)
		log.Infof("execLowhttp: Strong Host Mode - localAddrIP: %s", localAddrIP)

		// Validate that localAddr is an IP address (not a hostname)
		if localAddrIP != "" {
			// Extract IP from host:port format if needed
			host, _, err := utils.ParseStringToHostPort(localAddrIP)
			if err == nil {
				localAddrIP = host
				log.Infof("execLowhttp: 从 host:port 格式提取 IP: %s", localAddrIP)
			}
			// Validate it's an IP address
			ip := net.ParseIP(utils.FixForParseIP(localAddrIP))
			if ip != nil {
				// Pass strong host mode with localAddr IP to netx dial layer
				// DialX_WithStrongHostMode expects the local IP address to bind to
				opts = append(opts, lowhttp.WithExtendDialXOption(netx.DialX_WithStrongHostMode(localAddrIP)))
				log.Infof("execLowhttp: Strong Host Mode 已设置 localAddr: %s", localAddrIP)
			} else {
				log.Warnf("execLowhttp: Strong Host Mode localAddr '%s' 不是有效的 IP 地址，忽略", localAddrIP)
			}
		} else {
			log.Warnf("execLowhttp: Strong Host Mode 已启用但未在 httpctx 中找到 localAddr")
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

	log.Infof("execLowhttp: ====== 开始执行 HTTPWithoutRedirect ======")
	log.Infof("execLowhttp: 目标地址 - Host: %s, Port: %d, HTTPS: %v", host, port, isHttps)
	if connectedHost != "" {
		log.Infof("execLowhttp: 实际连接地址 - Host: %s, Port: %d", connectedHost, connectedPort)
	}

	lowHttpResp, err := lowhttp.HTTPWithoutRedirect(opts...)
	if err != nil {
		log.Errorf("execLowhttp: HTTPWithoutRedirect 执行失败: %v", err)
		log.Errorf("execLowhttp: 错误详情 - 目标: %s:%d, HTTPS: %v, StrongHostMode: %v", host, port, isHttps, isStrongHostMode)
		req.RemoteAddr = ""
		httpctx.SetRemoteAddr(req, "")
		return nil, err
	}

	log.Infof("execLowhttp: HTTPWithoutRedirect 执行成功")
	log.Infof("execLowhttp: 响应原始数据长度: %d", len(lowHttpResp.RawPacket))
	if len(lowHttpResp.RawPacket) > 0 {
		// 尝试解析响应状态码
		if len(lowHttpResp.RawPacket) > 12 {
			statusLine := string(lowHttpResp.RawPacket[:min(100, len(lowHttpResp.RawPacket))])
			log.Infof("execLowhttp: 响应状态行预览: %s", statusLine)
		}
	}

	// set trace info
	httpctx.SetResponseTraceInfo(req, lowHttpResp.TraceInfo)
	if lowHttpResp.TraceInfo != nil {
		log.Infof("execLowhttp: TraceInfo 已设置")
	}

	if lowHttpResp.RemoteAddr != "" {
		httpctx.SetRemoteAddr(req, lowHttpResp.RemoteAddr)
		req.RemoteAddr = lowHttpResp.RemoteAddr
		log.Infof("execLowhttp: RemoteAddr: %s", lowHttpResp.RemoteAddr)
	} else {
		log.Warnf("execLowhttp: RemoteAddr 为空")
	}

	log.Infof("execLowhttp: ====== 开始解析响应 ======")
	rsp, err := lowhttp.ParseBytesToHTTPResponse(lowHttpResp.RawPacket)
	if err != nil {
		log.Errorf("execLowhttp: 解析 HTTP 响应失败: %v", err)
		log.Errorf("execLowhttp: 响应数据长度: %d", len(lowHttpResp.RawPacket))
		if len(lowHttpResp.RawPacket) > 0 {
			preview := string(lowHttpResp.RawPacket[:min(200, len(lowHttpResp.RawPacket))])
			log.Errorf("execLowhttp: 响应数据预览: %s", preview)
		}
		return nil, err
	}

	if rsp != nil {
		rsp.Request = req
		log.Infof("execLowhttp: 响应解析成功 - 状态码: %d, 状态: %s", rsp.StatusCode, rsp.Status)
		log.Infof("execLowhttp: 响应头数量: %d", len(rsp.Header))
		for key, values := range rsp.Header {
			log.Infof("execLowhttp: 响应头 - %s: %v", key, values)
		}
		if rsp.ContentLength > 0 {
			log.Infof("execLowhttp: 响应内容长度: %d", rsp.ContentLength)
		}
	} else {
		log.Warnf("execLowhttp: 解析后的响应为 nil")
	}

	utils.FixHTTPResponseForGolangNativeHTTPClient(rsp)
	log.Infof("execLowhttp: ====== 请求执行完成 ======")
	return rsp, err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
