package minimartian

import (
	"context"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// isServerSentEventContentType reports whether value is a text/event-stream
// media type, tolerating charset and other parameters.
func isServerSentEventContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	return strings.EqualFold(mediaType, "text/event-stream")
}

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

func (p *Proxy) execLowhttp(ctx *Context, req *http.Request) (*http.Response, error) {
	// A cancellable context for the upstream request. When the downstream
	// client disconnects during a long-lived streaming response (e.g. SSE),
	// cancelling this context tears down the upstream connection so the
	// blocking body reader can return.
	upstreamContext, cancelUpstream := context.WithCancel(req.Context())
	defer cancelUpstream()

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
		lowhttp.WithContext(upstreamContext),
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
	}

	isStrongHostMode = httpctx.GetIsStrongHostMode(req)
	upstreamPortModified = httpctx.GetUpstreamPortIsModified(req)

	//	if connectedPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort); connectedPort > 0 {
	//		portValid := (connectedPort == 443 && isHttps) || (connectedPort == 80 && !isHttps)
	//		if !portValid {
	//			// 修复host和port
	//			if host := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost); host != "" {
	//				opts = append(opts, lowhttp.WithHost(host))
	//			}
	//			opts = append(opts, lowhttp.WithPort(connectedPort))
	//		}
	//	}

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
		// Forwarding is decided once: the first header that signals a
		// streaming response wins and registers the single shared
		// callback. makeStreamResponseCallback inspects the fully-parsed
		// response to choose the trigger timing (immediate vs buffered),
		// so this site only decides *whether* to stream plus the few
		// flags that must be set before the body is read.
		if httpctx.GetResponseHeaderCallback(req) != nil {
			return
		}
		bwr := httpctx.GetMITMFrontendReadWriter(req)
		if bwr == nil {
			return
		}

		switch key {
		case "content-type":
			// SSE: long-lived stream. NoBodyBuffer must be set here, before
			// the builder reads it, so the infinite body is never buffered
			// into memory. The trigger timing is decided inside the callback.
			if isServerSentEventContentType(value) {
				httpctx.SetNoBodyBuffer(req, true)
				httpctx.SetResponseReadTooSlow(req, true)
				httpctx.SetResponseHeaderCallback(req, p.makeStreamResponseCallback(isHttps, req, bwr, cancelUpstream))
				return
			}
			if ret := httpctx.GetResponseContentTypeFiltered(req); ret != nil && ret(value) {
				httpctx.SetResponseHeaderCallback(req, p.makeStreamResponseCallback(isHttps, req, bwr, cancelUpstream))
				return
			}
		case "content-length":
			if contentLength := codec.Atoi(value); contentLength >= int(MaxContentLength) {
				httpctx.SetResponseHeaderCallback(req, p.makeStreamResponseCallback(isHttps, req, bwr, cancelUpstream))
			}
		case "transfer-encoding":
			if utils.IContains(value, "chunked") {
				httpctx.SetResponseHeaderCallback(req, p.makeStreamResponseCallback(isHttps, req, bwr, cancelUpstream))
			}
		}
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

// makeStreamResponseCallback builds a ResponseHeaderCallback that relays the
// response body to the downstream client via a TriggerWriter + pipe + goroutine
// IOCopy. This is the single forwarding primitive shared by all streaming
// responses.
//
// Unlike the previous design where the caller picked the trigger timing by
// passing an `immediate` flag, the callback now inspects the fully-parsed
// response (available here as `rsp`) and decides the streaming kind itself:
//
//   - SSE (text/event-stream): immediate trigger; a downstream write failure
//     cancels the upstream request so the blocking body reader can return.
//   - content-type filtered:   immediate trigger; upstream is not cancelled.
//   - chunked / large body:     buffered trigger (fires on MaxContentLength or
//     maxReadWaitTime); upstream is not cancelled.
//
// immediate = NewTriggerWriterImmediate fires on the first body chunk so the
// header is written and forwarding starts without delay — essential for SSE
// where the body is long-lived. buffered = NewTriggerWriterEx waits until the
// size or time threshold is exceeded.
//
// When p.streamRecorder is set, an optional best-effort recorder is created
// to persist body chunks to a spill file for history/audit. The recorder is
// only created for SSE responses (see the kind == streamKindSSE guard below):
// SSE disables body buffering, so the spill file is the only way to persist
// the long-lived body. Recorder write errors never break or delay forwarding.
func (p *Proxy) makeStreamResponseCallback(
	isHTTPS bool,
	req *http.Request,
	bwr io.ReadWriter,
	cancelUpstream context.CancelFunc,
) httpctx.ResponseHeaderCallbackType {
	return func(rsp *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error) {
		// Decide the streaming kind from the fully-parsed response. This
		// keeps the "immediate vs buffered" trigger choice inside the
		// forwarding mechanism instead of at every call site.
		kind := p.classifyStreamResponse(req, rsp)
		if kind == streamKindNone {
			// Not actually a streaming response (e.g. the registered header
			// was a false positive). Don't interfere with the body reader.
			return bodyReader, nil
		}
		immediate := kind == streamKindSSE || kind == streamKindFiltered
		// Only SSE cancels the upstream on downstream write failure; the
		// filtered/chunked paths preserve the original no-cancel behavior.
		var cancelOnWriteFail context.CancelFunc
		if kind == streamKindSSE {
			cancelOnWriteFail = cancelUpstream
		}

		// Create an optional best-effort recorder for incremental persistence.
		// The recorder is set up before the trigger fires so the goroutine
		// can tee body chunks into it.
		//
		// Only SSE responses need a recorder: SSE sets NoBodyBuffer so the
		// response builder never buffers the long-lived body, and the only
		// way to persist it for history/audit is the spill file the recorder
		// writes incrementally. The recorder also marks the flow as
		// read-too-slow so History/API reconstruct the response from the
		// spill file instead of the (empty) DB body.
		//
		// Filtered and chunked/large responses do NOT set NoBodyBuffer, so
		// the response builder still buffers the full body and the ordinary
		// mirror path persists it. Creating a recorder for them would
		// unconditionally mark the flow read-too-slow and attach spill files
		// even when the body is smaller than MaxContentLength (e.g. a chunked
		// 4 MB response under a 5 MB limit), which is incorrect. The
		// too-large decision for those kinds stays with the response builder,
		// which judges by the actual body size — matching the pre-SSE
		// behavior.
		var recorder io.WriteCloser
		var closeOnce sync.Once
		closeRecorder := func() {
			closeOnce.Do(func() {
				if recorder != nil {
					if err := recorder.Close(); err != nil {
						log.Warnf("mitm: finalize stream recorder failed: %v", err)
					}
				}
			})
		}
		if kind == streamKindSSE && p.streamRecorder != nil {
			recorderRsp, err := utils.ReadHTTPResponseFromBytes(headerBytes, nil)
			if err != nil {
				log.Warnf("mitm: parse response header for recorder failed: %v", err)
			} else {
				recorderRsp.Request = req
				recorder, err = p.streamRecorder(isHTTPS, req, recorderRsp, headerBytes)
				if err != nil {
					log.Warnf("mitm: create stream recorder failed: %v", err)
					recorder = nil
				} else if recorder != nil {
					httpctx.SetResponseStreamRecorder(req, recorder)
				}
			}
		}

		// Build the trigger callback: write header, flush, then start a
		// goroutine that drains the pipe and forwards body chunks to the
		// downstream client (and recorder).
		triggerHandler := func(buffer io.ReadCloser, triggerEvent string) {
			httpctx.SetContextValueInfoFromRequest(req, triggerEvent, true)
			httpctx.SetMITMSkipFrontendFeedback(req, true)
			if _, err := bwr.Write(headerBytes); err != nil {
				if cancelOnWriteFail != nil {
					cancelOnWriteFail()
				}
				return
			}
			utils.FlushWriter(bwr)
			go func() {
				writers := []io.Writer{utils.WriterAutoFlush(bwr)}
				if recorder != nil {
					// bestEffortStreamRecorder swallows recorder errors, so a
					// non-EOF error from the MultiWriter can only come from
					// the downstream writer — i.e. the client disconnected.
					writers = append(writers, &bestEffortStreamRecorder{writer: recorder})
				}
				_, err := utils.IOCopy(io.MultiWriter(writers...), buffer, nil)
				utils.FlushWriter(bwr)
				if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
					log.Errorf("io.Copy error: %s", err)
					// Downstream write failure (client gone): cancel the
					// upstream request so the blocking body reader returns.
					// Only SSE sets cancelOnWriteFail; other kinds leave it
					// nil to preserve the original no-cancel behavior.
					if cancelOnWriteFail != nil {
						cancelOnWriteFail()
					}
				}
			}()
		}

		// Choose the trigger based on the streaming kind.
		MaxContentLength := int(consts.GetGlobalMaxContentLength())
		if p.GetMaxContentLength() != 0 {
			MaxContentLength = p.maxContentLength
		}
		var writerCloser *utils.TriggerWriter
		if immediate {
			writerCloser = utils.NewTriggerWriterImmediate(triggerHandler)
		} else {
			writerCloser = utils.NewTriggerWriterEx(uint64(MaxContentLength), p.maxReadWaitTime, triggerHandler)
		}

		httpctx.SetResponseFinishedCallback(req, func() {
			httpctx.SetResponseTooLargeSize(req, writerCloser.GetCount())
			utils.FlushWriter(bwr)
			closeRecorder()
			writerCloser.Close()
		})
		return io.TeeReader(bodyReader, writerCloser), nil
	}
}

// streamKind classifies a streaming response for forwarding purposes.
type streamKind int

const (
	streamKindNone streamKind = iota
	// streamKindSSE is a text/event-stream response: immediate trigger,
	// downstream disconnect cancels the upstream connection.
	streamKindSSE
	// streamKindFiltered is a content-type-filtered response: immediate
	// trigger, upstream is not cancelled on downstream write failure.
	streamKindFiltered
	// streamKindChunkedLarge is a chunked or content-length-too-large
	// response: buffered trigger (size/timeout), upstream not cancelled.
	streamKindChunkedLarge
)

// classifyStreamResponse inspects the fully-parsed response and the request
// context to determine which streaming forwarding kind applies. It is the
// single place that decides trigger timing, so adding a new streaming type
// (e.g. streaming JSON) only needs a new case here.
func (p *Proxy) classifyStreamResponse(req *http.Request, rsp *http.Response) streamKind {
	if rsp == nil {
		return streamKindNone
	}
	ct := rsp.Header.Get("Content-Type")
	if isServerSentEventContentType(ct) {
		return streamKindSSE
	}
	if ret := httpctx.GetResponseContentTypeFiltered(req); ret != nil && ret(ct) {
		return streamKindFiltered
	}
	// chunked transfer-encoding
	for _, te := range rsp.TransferEncoding {
		if utils.IContains(te, "chunked") {
			return streamKindChunkedLarge
		}
	}
	// content-length too large
	MaxContentLength := int(consts.GetGlobalMaxContentLength())
	if p.GetMaxContentLength() != 0 {
		MaxContentLength = p.maxContentLength
	}
	if rsp.ContentLength >= int64(MaxContentLength) {
		return streamKindChunkedLarge
	}
	return streamKindNone
}

// bestEffortStreamRecorder wraps an io.Writer and never returns an error,
// ensuring that recorder failures do not break or delay response forwarding.
type bestEffortStreamRecorder struct {
	writer io.Writer
}

func (w *bestEffortStreamRecorder) Write(p []byte) (int, error) {
	if w == nil || w.writer == nil {
		return len(p), nil
	}
	if n, err := w.writer.Write(p); err != nil || n != len(p) {
		log.Warnf("mitm: persist streaming response chunk failed: wrote=%d expected=%d error=%v", n, len(p), err)
	}
	return len(p), nil
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
