package crep

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"net/http"
	"net/http/httptrace"
)

type httpTraceTransport struct {
	*http.Transport
	config []lowhttp.LowhttpOpt
}

func (t *httpTraceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	*req = *req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			addr := info.Conn.RemoteAddr()
			httpctx.SetRemoteAddr(req, addr.String())
			req.RemoteAddr = addr.String()
		},
	}))

	https := httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsHttps)
	if connectedPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort); connectedPort > 0 {
		var noModified = false
		if (connectedPort == 80 && !https) || (connectedPort == 443 && https) {
			noModified = true
		}
		if !noModified {
			connected := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedTo)
			if connected != "" {
				log.Debugf("origin %v => %v", req.Host, connected)
				req.Host = connected
				if req.URL.Host != "" {
					log.Debugf("origin %v => %v", req.URL.Host, connected)
					req.URL.Host = connected
				}
			}
		}
	}

	// Transport is golang native function call request
	// handling transfer-encoding,
	// do some hack to make sure packet is right

	bareBytes := httpctx.GetRequestBytes(req)
	reqBytes := lowhttp.FixHTTPRequest(bareBytes)
	ishttps := httpctx.GetRequestHTTPS(req)
	addr, port, _ := utils.ParseStringToHostPort(req.Host)

	opts := append(t.config, lowhttp.WithRequest(reqBytes), lowhttp.WithHttps(ishttps), lowhttp.WithConnPool(true), lowhttp.WithHost(addr), lowhttp.WithPort(port))
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
