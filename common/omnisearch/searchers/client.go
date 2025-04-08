package searchers

import (
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func Request(method, url string, headers map[string]string, query map[string]string, body []byte, opts ...lowhttp.LowhttpOpt) ([]byte, error) {
	isHttps, req, err := lowhttp.ParseUrlToHttpRequestRaw(method, url)
	if err != nil {
		return nil, err
	}
	req = lowhttp.ReplaceAllHTTPPacketHeaders(req, headers)
	req = lowhttp.ReplaceAllHTTPPacketQueryParams(req, query)
	if body != nil {
		req = lowhttp.ReplaceHTTPPacketBodyRaw(req, body, true)
	}
	newOpts := []lowhttp.LowhttpOpt{}
	newOpts = append(newOpts, lowhttp.WithPacketBytes(req))
	newOpts = append(newOpts, lowhttp.WithHttps(isHttps))
	newOpts = append(newOpts, opts...)
	raw, err := lowhttp.HTTP(newOpts...)
	if err != nil {
		return nil, err
	}
	return raw.RawPacket, nil
}
