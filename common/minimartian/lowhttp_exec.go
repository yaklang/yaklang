package minimartian

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"net/http"
	"strings"
)

func (p *Proxy) doHTTPRequest(ctx *Context, req *http.Request) (*http.Response, error) {
	if ctx.SkippingRoundTrip() {
		log.Debugf("mitm: skipping round trip")
		return proxyutil.NewResponse(200, nil, req), nil
	}
	if httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsDropped) {
		log.Debugf("mitm: skipping round trip due to user manually drop")
		return proxyutil.NewResponse(200, strings.NewReader(proxyutil.GetErrorRspBody("请求被用户丢弃")), req), nil
	}

	httpctx.SetRequestHTTPS(req, ctx.GetSessionBoolValue(httpctx.REQUEST_CONTEXT_KEY_IsHttps))
	inherit := func(i string) {
		httpctx.SetContextValueInfoFromRequest(req, i, ctx.GetSessionStringValue(i))
	}
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedTo)
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort)
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost)
	return p.execLowhttp(req)
}

func (p *Proxy) execLowhttp(req *http.Request) (*http.Response, error) {
	bareBytes := httpctx.GetRequestBytes(req)
	reqBytes := lowhttp.FixHTTPRequest(bareBytes)

	var isHttps = httpctx.GetRequestHTTPS(req)

	var isGmTLS = p.gmTLS && isHttps

	opts := append(p.lowhttpConfig,
		lowhttp.WithRequest(reqBytes),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithGmTLS(isGmTLS),
		lowhttp.WithConnPool(true),
		lowhttp.WithSaveHTTPFlow(false),
	)

	if connectedPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort); connectedPort > 0 {
		portValid := (connectedPort == 443 && isHttps) || (connectedPort == 80 && !isHttps)
		if !portValid {
			//修复host和port
			if host := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost); host != "" {
				opts = append(opts, lowhttp.WithHost(host))
			}
			opts = append(opts, lowhttp.WithPort(connectedPort))
		}
	}

	lowHttpResp, err := lowhttp.HTTPWithoutRedirect(opts...)
	if err != nil {
		return nil, err
	}

	if lowHttpResp.RemoteAddr != "" {
		httpctx.SetRemoteAddr(req, lowHttpResp.RemoteAddr)
		req.RemoteAddr = lowHttpResp.RemoteAddr
	}

	rsp, err := lowhttp.ParseBytesToHTTPResponse(lowHttpResp.RawPacket)
	if rsp != nil {
		rsp.Request = req
	}

	//utils.FixHTTPRequestForGolangNativeHTTPClient(req)
	//rsp, err := t.Transport.RoundTrip(req)

	utils.FixHTTPResponseForGolangNativeHTTPClient(rsp)
	return rsp, err
}
