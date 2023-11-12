package minimartian

import (
	"bytes"
	"crypto/tls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"io"
	"net/url"
)

// proxyH2 proxies HTTP/2 traffic between a client connection, `cc`, and the HTTP/2 `url` assuming
// h2 is being used. Since no browsers use h2c, it's safe to assume all traffic uses TLS.
// Revision this func from martian h2 package since it was not compatible with martian modifier style
func (p *Proxy) proxyH2(closing chan bool, cc *tls.Conn, url *url.URL) error {
	log.Infof("Proxying %v with HTTP/2", url)
	go func() {
		select {
		case <-closing:
		}
		cc.Close()
	}()

	return lowhttp.ServeHTTP2Connection(cc, func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error) {
		var reqBytes = bytes.NewBuffer(header)
		io.Copy(reqBytes, body)
		req, err := utils.ReadHTTPRequestFromBytes(reqBytes.Bytes())
		if err != nil {
			return nil, nil, err
		}
		httpctx.SetRequestHTTPS(req, true)
		if req.URL != nil {
			req.URL.Scheme = "https"
		}
		if err := p.reqmod.ModifyRequest(req); err != nil {
			log.Errorf("mitm: error modifying request: %v", err)
			proxyutil.Warning(req.Header, err)
			return nil, nil, err
		}
		if httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsDropped) {
			return []byte(`HTTP/2 200 OK
Content-Type: text/html
`), io.NopCloser(bytes.NewBufferString(proxyutil.GetErrorRspBody("请求被用户丢弃"))), nil
		} else {
			rsp, err := p.execLowhttp(req)
			if err != nil {
				log.Errorf("mitm: error requesting to remote server: %v", err)
				return nil, nil, err
			}
			defer func() {
				if rsp != nil && rsp.Body != nil {
					rsp.Body.Close()
				}
			}()

			if err := p.resmod.ModifyResponse(rsp); err != nil {
				log.Errorf("mitm: error modifying response: %v", err)
				proxyutil.Warning(req.Header, err)
				return nil, nil, err
			}

			rspBytes, err := utils.DumpHTTPResponse(rsp, true)
			if err != nil {
				return nil, nil, err
			}
			header, body := lowhttp.SplitHTTPPacketFast(rspBytes)
			return []byte(header), io.NopCloser(bytes.NewBuffer(body)), nil
		}
	})
}
