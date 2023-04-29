package crep

import (
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"net/http"
	"net/http/httptrace"
	"yaklang/common/utils"
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
	rsp, err := t.Transport.RoundTrip(req)
	return rsp, err
}
