package crep

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian"
	martian "github.com/yaklang/yaklang/common/minimartian"
	"github.com/yaklang/yaklang/common/minimartian/fifo"
	"github.com/yaklang/yaklang/common/minimartian/header"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func (m *MITMServer) setHijackHandler(rootCtx context.Context) {
	group := fifo.NewGroup()

	wsModifier := &WebSocketModifier{
		websocketHijackMode:            m.websocketHijackMode,
		forceTextFrame:                 m.forceTextFrame,
		websocketRequestHijackHandler:  m.websocketRequestHijackHandler,
		websocketResponseHijackHandler: m.websocketResponseHijackHandler,
		websocketRequestMirror:         m.websocketRequestMirror,
		websocketResponseMirror:        m.websocketResponseMirror,
		ProxyGetter:                    m.GetMartianProxy,
		RequestHijackCallback: func(req *http.Request) error {
			var isHttps bool
			switch req.URL.Scheme {
			case "https", "HTTPS":
				isHttps = true
			case "http", "HTTP":
				isHttps = false
			}
			hijackedRaw, err := utils.HttpDumpWithBody(req, true)
			if err != nil {
				log.Errorf("mitm-hijack marshal request to bytes failed: %s", err)
				return nil
			}
			m.requestHijackHandler(isHttps, req, hijackedRaw)
			return nil
		},
		ResponseHijackCallback: func(req *http.Request, rsp *http.Response, rspRaw []byte) []byte {
			return m.responseHijackHandler(httpctx.GetRequestHTTPS(req), req, rsp, rspRaw, httpctx.GetRemoteAddr(req))
		},
	}
	if m.proxyUrl != nil {
		wsModifier.ProxyStr = m.proxyUrl.String()
	}
	group.AddRequestModifier(NewRequestModifier(m.buildHijackRequestHandler(rootCtx, wsModifier)))
	group.AddResponseModifier(NewResponseModifier(m.hijackResponseHandler))
	m.proxy.SetRequestModifier(group)
	m.proxy.SetResponseModifier(group)
}

func (m *MITMServer) buildHijackRequestHandler(rootCtx context.Context, wsModifier *WebSocketModifier) func(r *http.Request) error {
	return func(r *http.Request) error {
		return m.hijackRequestHandler(rootCtx, wsModifier, r)
	}
}

func (m *MITMServer) hijackRequestHandler(rootCtx context.Context, wsModifier *WebSocketModifier, req *http.Request) error {
	defer func() {
		if err := recover(); err != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
			// DO NOT PANIC!!!
		}
	}()

	/*
	 use buildin cert domains
	*/
	if utils.StringArrayContains(defaultBuildinDomains, req.URL.Hostname()) {
		ctx := martian.NewContext(req, m.GetMartianProxy())
		if ctx != nil {
			ctx.SkipRoundTrip()
		}
		return nil
	}

	/*
		handle websocket
	*/
	if utils.IContains(req.Header.Get("Connection"), "upgrade") && req.Header.Get("Upgrade") == "websocket" {
		return wsModifier.ModifyRequest(req)
	}

	// remove proxy-connection like!
	err := header.NewHopByHopModifier().ModifyRequest(req)
	if err != nil {
		log.Errorf("remove hop by hop header failed: %s", err)
	}
	if !httpctx.GetRequestViaCONNECT(req) {
		// 不是通过 CONNECT 方法的代理，一般常见非 HTTPS 代理，这种情况下
		// Dump 出来的数据包 URI 不包含 http://
		raw, err := utils.DumpHTTPRequest(req, true)
		if err != nil {
			log.Errorf("dump request failed: %s", err)
		}
		if funk.NotEmpty(raw) {
			httpctx.SetBareRequestBytes(req, raw)
		}
	}

	if req.Method == "CONNECT" {
		return nil
	}

	/*
		handle hijack
	*/
	var isHttps bool
	switch req.URL.Scheme {
	case "https", "HTTPS":
		isHttps = true
	case "http", "HTTP":
		isHttps = false
	}
	httpctx.SetRequestHTTPS(req, isHttps)

	if m.requestHijackHandler != nil {
		hijackedRaw := httpctx.GetBareRequestBytes(req)
		if hijackedRaw == nil || len(hijackedRaw) == 0 {
			hijackedRaw, err = utils.DumpHTTPRequest(req, true)
			if err != nil {
				log.Errorf("mitm-hijack marshal request to bytes failed: %s", err)
				return nil
			}
			httpctx.SetBareRequestBytes(req, hijackedRaw)
		}

		/*
			ctx control
		*/
		select {
		case <-rootCtx.Done():
			reqContext := martian.NewContext(req, m.proxy)
			reqContext.SkipRoundTrip()
			return utils.Error("request hijacker error: MITM Proxy Context Canceled")
		default:
		}
		//urlInstance, _ := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
		//if urlInstance != nil {
		//	log.Infof("hijack url [%v]: %v", req.Method, urlInstance.String())
		//}
		hijackedRequestRaw := m.requestHijackHandler(isHttps, req, hijackedRaw)
		select {
		case <-rootCtx.Done():
			reqContext := martian.NewContext(req, m.proxy)
			reqContext.SkipRoundTrip()
			return utils.Error("request hijacker error: MITM Proxy Context Canceled")
		default:
		}
		if hijackedRequestRaw == nil {
			httpctx.SetContextValueInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsDropped, true)
		} else {
			hijackedRaw = hijackedRequestRaw
			hijackedReq, err := lowhttp.ParseBytesToHttpRequest(hijackedRequestRaw)
			if err != nil {
				log.Errorf("mitm-hijacked request to http.Request failed: %s", err)
				return nil
			}
			if isHttps {
				hijackedReq.TLS = req.TLS
			}
			hijackedReq.RemoteAddr = req.RemoteAddr
			if req.ProtoMajor != 2 {
				hijackedReq.Proto = "HTTP/1.1"
				hijackedReq.ProtoMajor = 1
				hijackedReq.ProtoMinor = 1
			}

			*req = *hijackedReq.WithContext(req.Context())

			// fix new request: Host n Schema
			if req.URL.Host == "" {
				req.URL.Host = req.Host
			}

			if req.URL.Host == "" && req.Host == "" {
				req.URL.Host = httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedTo)
				req.Host = req.URL.Host
			}

			if req.URL.Scheme == "" && (req.TLS != nil || isHttps) {
				req.URL.Scheme = "https"
			} else {
				req.URL.Scheme = "http"
			}
		}
	}
	return nil
}

func (m *MITMServer) hijackResponseHandler(rsp *http.Response) error {
	defer func() {
		if err := recover(); err != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
			// DO NOT PANIC!!!
		}
	}()

	requestOrigin := rsp.Request
	rsp.TLS = requestOrigin.TLS

	if requestOrigin.Method == "CONNECT" {
		return nil
	}

	/*
		return the ca certs
	*/
	if utils.StringArrayContains(defaultBuildinDomains, requestOrigin.URL.Hostname()) {
		return handleBuildInMITMDefaultPageResponse(rsp)
	}

	var (
		responseBytes []byte
		dropped       = utils.NewBool(false)
	)

	tooLarge := httpctx.GetResponseTooLarge(requestOrigin)

	// response hijacker
	if m.responseHijackHandler != nil {
		responseBytes = httpctx.GetBareResponseBytes(requestOrigin)
		if len(responseBytes) <= 0 {
			var err error
			responseBytes, err = utils.DumpHTTPResponse(rsp, !tooLarge)
			if err != nil {
				log.Errorf("mitm-hijack marshal response to bytes failed: %s", err)
				return nil
			}
			httpctx.SetBareResponseBytes(requestOrigin, responseBytes)
		}

		isHttps := httpctx.GetRequestHTTPS(rsp.Request)
		result := m.responseHijackHandler(isHttps, requestOrigin, rsp, responseBytes, httpctx.GetRemoteAddr(requestOrigin))
		if result == nil {
			dropped.Set()
			rsp = proxyutil.NewResponse(200, strings.NewReader("响应被用户丢弃"), requestOrigin)
		} else {
			responseBytes = make([]byte, len(result))
			copy(responseBytes, result)

			resultRsp, err := utils.ReadHTTPResponseFromBytes(responseBytes, nil)
			if err != nil {
				log.Errorf("parse fixed response to body failed: %s", err)
				return utils.Errorf("hijacking modified response parsing failed: %s", err)
			}
			*rsp = *resultRsp
			rsp.Request = requestOrigin
			rsp.TLS = requestOrigin.TLS
		}
	}

	if m.httpFlowMirror != nil {
		if len(responseBytes) <= 0 {
			var err error
			responseBytes, err = utils.HttpDumpWithBody(rsp, !tooLarge)
			if err != nil {
				log.Errorf("dump response mirror failed: %s", err)
				return nil
			}
			httpctx.SetBareResponseBytes(requestOrigin, responseBytes)
		}

		reqRawBytes := httpctx.GetRequestBytes(requestOrigin)
		if reqRawBytes != nil {
			start := time.Now()
			m.httpFlowMirror(httpctx.GetRequestHTTPS(requestOrigin), requestOrigin, rsp, start.Unix())
			end := time.Now()
			cost := end.Sub(start)
			if cost.Milliseconds() > 600 {
				log.Infof(`m.httpFlowMirror cost: %v`, cost)
			}
		} else {
			log.Errorf("request raw bytes is nil")
		}
	}
	if dropped.IsSet() {
		return minimartian.IsDroppedError
	}
	return nil
}

func handleBuildInMITMDefaultPageResponse(rsp *http.Response) error {
	if strings.HasPrefix(rsp.Request.URL.Path, "/static") {
		filePath := strings.TrimPrefix(rsp.Request.URL.Path, "/static/")
		data, err := staticFS.ReadFile("static/" + filePath)
		if err != nil {
			log.Errorf("read static file failed: %s", err)
			return nil
		}

		if strings.HasSuffix(filePath, ".css") {
			rsp.Header.Set("Content-Type", "text/css")
		} else if strings.HasSuffix(filePath, ".ico") {
			rsp.Header.Set("Content-Type", "image/x-icon")
		}

		rsp.Body = io.NopCloser(bytes.NewReader(data))
		rsp.ContentLength = int64(len(data))
		rsp.StatusCode = http.StatusOK
		return nil
	}
	if rsp.Request.URL.Path == "/download-mitm-crt" {
		// 返回mitm-server.crt内容
		body := defaultCA
		rsp.Body = io.NopCloser(bytes.NewReader(body))
		rsp.ContentLength = int64(len(body))
		rsp.Header.Set("Content-Disposition", `attachment; filename="mitm-server.crt"`)
		rsp.Header.Set("Content-Type", "octet-stream")
		return nil
	}

	rsp.Body = io.NopCloser(bytes.NewReader(htmlContent))
	rsp.ContentLength = int64(len(htmlContent))
	rsp.Header.Set("Content-Type", "text/html; charset=utf-8")
	return nil
}
