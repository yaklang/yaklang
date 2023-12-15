package crawler

import (
	"net/http"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type RequestIf interface {
	Url() string
	Request() *http.Request
	ResponseBody() []byte
	Response() (*http.Response, error)
	IsHttps() bool
	ResponseRaw() []byte
	RequestRaw() []byte
}

// Url 返回当前请求的URL字符串
// Example:
// ```
// req.Url()
// ```
func (r *Req) Url() string {
	if r.url != "" {
		return r.url
	}
	return r.request.URL.String()
}

// Request 返回当前请求的原始请求结构体引用
// Example:
// ```
// req.Request()
// ```
func (r *Req) Request() *http.Request {
	reqIns, err := utils.ReadHTTPRequestFromBytes(r.requestRaw)
	if err != nil {
		log.Errorf("read request failed: %s", err)
	}
	return reqIns
}

// RequestRaw 返回当前请求的原始请求报文
// Example:
// ```
// req.RequestRaw()
// ```
func (r *Req) RequestRaw() []byte {
	return r.requestRaw
}

// Response 返回当前请求的原始响应结构体引用与错误
// Example:
// ```
// resp, err = req.Response()
// ```
func (r *Req) Response() (*http.Response, error) {
	if r.response == nil {
		if r.err == nil {
			return nil, utils.Errorf("BUG: crawler.Req no response and error")
		}
		return nil, r.err
	}
	return r.response, nil
}

// ResponseRaw 返回当前请求的原始响应报文
// Example:
// ```
// req.ResponseRaw()
// ```
func (r *Req) ResponseRaw() []byte {
	return r.responseRaw
}

// ResponseBody 返回当前请求的原始响应体
// Example:
// ```
// req.ResponseBody()
// ```
func (r *Req) ResponseBody() []byte {
	return r.responseBody
}

// IsHttps 返回当前请求是否是https请求
// Example:
// ```
// req.IsHttps()
// ```
func (r *Req) IsHttps() bool {
	return r.https
}
