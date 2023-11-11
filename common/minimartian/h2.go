package minimartian

import (
	"crypto/tls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/lowhttp/lowhttp2"
	"io"
	"net/http"
	"net/url"
)

type H2Handler struct {
	reqmod     RequestModifier
	resmod     ResponseModifier
	proxy      *Proxy
	serverHost string
}

func makeNewH2Handler(reqmod RequestModifier, resmod ResponseModifier, serverHost string, proxy *Proxy) *H2Handler {
	return &H2Handler{reqmod: reqmod, resmod: resmod, serverHost: serverHost, proxy: proxy}
}

func (h *H2Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := h.reqmod.ModifyRequest(req); err != nil {
		log.Errorf("mitm: error modifying request: %v", err)
		proxyutil.Warning(req.Header, err)
		return
	}
	if httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsDropped) {
		w.WriteHeader(200)
		w.Write([]byte(proxyutil.GetErrorRspBody("请求被用户丢弃")))
	} else {
		rsp, err := h.proxy.execLowhttp(req)
		if err != nil {
			log.Errorf("mitm: error requesting to remote server: %v", err)
			return
		}
		defer rsp.Body.Close()

		if err := h.resmod.ModifyResponse(rsp); err != nil {
			log.Errorf("mitm: error modifying response: %v", err)
			proxyutil.Warning(req.Header, err)
			return
		}

		for k, v := range rsp.Header {
			w.Header().Set(k, v[0])
		}
		w.WriteHeader(rsp.StatusCode)
		rspBody, _ := io.ReadAll(rsp.Body)
		w.Write(rspBody)
	}
}

// proxyH2 proxies HTTP/2 traffic between a client connection, `cc`, and the HTTP/2 `url` assuming
// h2 is being used. Since no browsers use h2c, it's safe to assume all traffic uses TLS.
// Revision this func from martian h2 package since it was not compatible with martian modifier style
func (p *Proxy) proxyH2(closing chan bool, cc *tls.Conn, url *url.URL) error {
	if p.mitm.H2Config().EnableDebugLogs {
		log.Infof("\u001b[1;35mProxying %v with HTTP/2\u001b[0m", url)
	}

	go func() {
		select {
		case <-closing:
		}
		cc.Close()
	}()

	proxyClient := lowhttp2.Server{
		PermitProhibitedCipherSuites: true,
	}
	handler := makeNewH2Handler(p.reqmod, p.resmod, url.Host, p)
	proxyClientConfig := &lowhttp2.ServeConnOpts{Handler: handler}
	proxyClient.ServeConn(cc, proxyClientConfig)
	return nil
}
