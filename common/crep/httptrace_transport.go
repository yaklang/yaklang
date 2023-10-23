package crep

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"net/http"
)

type httpTraceTransport struct {
	*http.Transport
	config []lowhttp.LowhttpOpt
}

func (t *httpTraceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	//*req = *req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
	//	GotConn: func(info httptrace.GotConnInfo) {
	//		addr := info.Conn.RemoteAddr()
	//		httpctx.SetRemoteAddr(req, addr.String())
	//		req.RemoteAddr = addr.String()
	//	},
	//}))

	bareBytes := httpctx.GetRequestBytes(req)
	reqBytes := lowhttp.FixHTTPRequest(bareBytes)

	ishttps := httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsHttps)
	opts := append(t.config,
		lowhttp.WithRequest(reqBytes),
		lowhttp.WithHttps(ishttps),
		lowhttp.WithConnPool(true),
		lowhttp.WithSaveHTTPFlow(false))

	if connectedPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort); connectedPort > 0 {
		var noModified = false
		if (connectedPort == 80 && !ishttps) || (connectedPort == 443 && ishttps) {
			noModified = true
		}
		if !noModified {
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

	rsp, err := lowhttp.ParseBytesToHTTPResponse(lowHttpResp.RawPacket)
	if rsp == nil {
		//utils.PrintCurrentGoroutineRuntimeStack()
		//spew.Dump(lowHttpResp)
	}
	if rsp != nil {
		rsp.Request = req
	}

	//utils.FixHTTPRequestForGolangNativeHTTPClient(req)
	//rsp, err := t.Transport.RoundTrip(req)

	utils.FixHTTPResponseForGolangNativeHTTPClient(rsp)
	return rsp, err
}
