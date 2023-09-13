package netx

import (
	"net/http"
	"time"
)

func NewDefaultHTTPClient(
	proxy ...string,
) *http.Client {
	tr := NewDefaultHTTPTransport(proxy...)
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}
	return client
}
