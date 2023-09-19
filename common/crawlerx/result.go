// Package crawlerx
// @Author bcy2007  2023/7/12 16:42
package crawlerx

import (
	"context"
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"strings"
)

type ReqInfo interface {
	Type() string

	Url() string
	Method() string

	RequestHeaders() map[string]string
	RequestBody() string
	RequestRaw() ([]byte, error)

	StatusCode() int
	ResponseHeaders() map[string]string
	ResponseBody() string

	Screenshot() string

	From() string
}

type RequestResult struct {
	// request  *rod.HijackRequest
	// response *rod.HijackResponse

	request  HijackRequest
	response HijackResponse
	from     string
}

func (result *RequestResult) Url() string {
	return result.request.URL().String()
}

func (result *RequestResult) Method() string {
	return result.request.Method()
}

func (result *RequestResult) RequestHeaders() map[string]string {
	headers := make(map[string]string, 0)
	tempHeaders := result.request.Headers()
	for k, v := range tempHeaders {
		headers[k] = v.String()
	}
	return headers
}

func (result *RequestResult) RequestBody() string {
	return result.request.Body()
}

// getLength returns length of a Reader efficiently
func getLength(x io.Reader) (int64, error) {
	len, err := io.Copy(io.Discard, x)
	return len, err
}
func (result *RequestResult) RequestRaw() ([]byte, error) {
	resplen := int64(0)
	dumpbody := true
	clone := result.request.Req().Clone(context.TODO())
	if clone.Body != nil {
		resplen, _ = getLength(clone.Body)
	}
	if resplen == 0 {
		dumpbody = false
		clone.ContentLength = 0
		clone.Body = nil
		delete(clone.Header, "Content-length")
	}
	dumpBytes, err := utils.DumpHTTPRequest(clone, dumpbody)
	if err != nil {
		return nil, err
	}
	return dumpBytes, nil
}
func (result *RequestResult) ResponseHeaders() map[string]string {
	headers := make(map[string]string, 0)
	tempHeaders := result.response.Headers()
	for k, v := range tempHeaders {
		headers[k] = strings.Join(v, "; ")
	}
	return headers
}

func (result *RequestResult) ResponseBody() string {
	return result.response.Body()
}

func (result *RequestResult) Screenshot() string {
	return ""
}

func (result *RequestResult) StatusCode() int {
	return result.response.Payload().ResponseCode
}

func (result *RequestResult) Type() string {
	return "hijack_result"
}

func (result *RequestResult) From() string {
	return result.from
}

type SimpleResult struct {
	url        string
	screenshot string
	resultType string
	method     string
	request    *rod.HijackRequest
	from       string
}

func (simpleResult *SimpleResult) Url() string {
	if simpleResult.request != nil {
		return simpleResult.request.URL().String()
	}
	return simpleResult.url
}

func (simpleResult *SimpleResult) Method() string {
	if simpleResult.request != nil {
		return simpleResult.request.Method()
	}
	if simpleResult.method == "" {
		return "GET"
	}
	return simpleResult.method
}

func (simpleResult *SimpleResult) RequestHeaders() map[string]string {
	if simpleResult.request != nil {
		headers := make(map[string]string)
		tempHeaders := simpleResult.request.Headers()
		for k, v := range tempHeaders {
			headers[k] = v.String()
		}
		return headers
	}
	return nil
}

func (simpleResult *SimpleResult) RequestBody() string {
	if simpleResult.request != nil {
		return simpleResult.request.Body()
	}
	return ""
}
func (simpleResult *SimpleResult) RequestRaw() ([]byte, error) {
	return nil, nil
}

func (simpleResult *SimpleResult) ResponseHeaders() map[string]string {
	return nil
}

func (simpleResult *SimpleResult) ResponseBody() string {
	return ""
}

func (simpleResult *SimpleResult) Screenshot() string {
	return simpleResult.screenshot
}

func (simpleResult *SimpleResult) Type() string {
	return simpleResult.resultType
}

func (*SimpleResult) StatusCode() int {
	return 0
}

func (simpleResult *SimpleResult) From() string {
	return simpleResult.from
}
