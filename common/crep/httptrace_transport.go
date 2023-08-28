package crep

import (
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"net/http"
	"net/http/httptrace"
)

type httpTraceTransport struct {
	*http.Transport
	cache *ttlcache.Cache
}

func (t *httpTraceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	*req = *req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			addr := info.Conn.RemoteAddr()
			host, port, _ := utils.ParseStringToHostPort(fmt.Sprintf("%v://%v", req.URL.Scheme, req.Host))
			key := utils.HostPort(host, port)
			if key == "" {
				host = req.Host
			}
			//log.Infof("remote addr: %v(%v)", addr, key)
			if t.cache != nil {
				t.cache.Set(key, addr)
			}
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

	if utils.StringArrayContains(req.TransferEncoding, "chunked") {
		req.TransferEncoding = nil
	}
	rsp, err := t.Transport.RoundTrip(req)
	return rsp, err
}
