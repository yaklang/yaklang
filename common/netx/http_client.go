package netx

import (
	"context"
	"net"
	"net/http"
	"time"
)

func NewDefaultHTTPClient(
	proxy ...string,
) *http.Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return DialContext(ctx, addr, proxy...)
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}
	return client
}
