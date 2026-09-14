package lowhttp

import (
	"context"
	"net/http"

	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// h3Transport implements Transport for HTTP/3 (QUIC).
type h3Transport struct{}

// NewH3Transport returns a Transport that executes requests over HTTP/3.
func NewH3Transport() Transport { return &h3Transport{} }

func (t *h3Transport) RoundTrip(ctx context.Context, tr *transportRequest) (*transportResult, error) {
	http3Conn, err := getHTTP3Conn(ctx, tr.originAddr, tr.dialOpts...)
	if err != nil {
		return nil, err
	}
	resp, responsePacket, err := doHttp3Request(ctx, http3Conn, tr.packet)
	_ = resp
	if tr.reqIns != nil {
		httpctx.SetBareResponseBytes(tr.reqIns, responsePacket)
	}
	return &transportResult{
		rawBytes:      responsePacket,
		remoteAddr:    tr.originAddr,
		portIsOpen:    true,
		firstResponse: resp,
	}, err
}

func (t *h3Transport) CanRetry(req *http.Request, err error) bool {
	// H3 has no connection pool; a failed dial is a failed request.
	return false
}

func (t *h3Transport) ShouldDowngrade(err error) bool {
	return false
}
