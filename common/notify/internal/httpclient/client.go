package httpclient

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// Request 是所有平台共享的 HTTP helper，对标 omnisearch/searchers/client.go。
//
// 它把 method/url/headers/query/body 渲染成一个完整的 HTTP 报文，
// 再通过 lowhttp.HTTP 发出，返回原始响应报文（含状态行+头+体）。
// 调用方用 lowhttp.GetHTTPPacketBody / GetStatusCodeFromResponse 解析。
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
	newOpts := []lowhttp.LowhttpOpt{
		lowhttp.WithPacketBytes(req),
		lowhttp.WithHttps(isHttps),
	}
	newOpts = append(newOpts, opts...)
	raw, err := lowhttp.HTTP(newOpts...)
	if err != nil {
		return nil, err
	}
	return raw.RawPacket, nil
}

// JSONRequest 是 Request 的 JSON 专用封装：自动 marshal body、设置 Content-Type，
// 并在非 2xx 时返回带响应体的错误，调用方直接拿到反序列化前的 body。
func JSONRequest(method, url string, headers map[string]string, query map[string]string, body any, opts ...lowhttp.LowhttpOpt) ([]byte, error) {
	var rawBody []byte
	if body != nil {
		switch v := body.(type) {
		case []byte:
			rawBody = v
		case string:
			rawBody = []byte(v)
		default:
			b, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal request body: %w", err)
			}
			rawBody = b
		}
	}
	if headers == nil {
		headers = map[string]string{}
	}
	if _, ok := headers["Content-Type"]; !ok && rawBody != nil {
		headers["Content-Type"] = "application/json; charset=utf-8"
	}
	return Request(method, url, headers, query, rawBody, opts...)
}

// Result 表示一次 HTTP 请求的解析结果。
type Result struct {
	StatusCode int
	Body       []byte
	Raw        []byte
}

// Do 执行请求并解析成 Result。
func Do(method, url string, headers map[string]string, query map[string]string, body any, opts ...lowhttp.LowhttpOpt) (*Result, error) {
	raw, err := JSONRequest(method, url, headers, query, body, opts...)
	if err != nil {
		return nil, err
	}
	return &Result{
		StatusCode: lowhttp.GetStatusCodeFromResponse(raw),
		Body:       lowhttp.GetHTTPPacketBody(raw),
		Raw:        raw,
	}, nil
}
