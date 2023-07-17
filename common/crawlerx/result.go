// Package crawlerx
// @Author bcy2007  2023/7/12 16:42
package crawlerx

import (
	"github.com/go-rod/rod"
	"strings"
)

type ReqInfo interface {
	Type() string

	Url() string
	Method() string

	RequestHeaders() map[string]string
	RequestBody() string

	StatusCode() int
	ResponseHeaders() map[string]string
	ResponseBody() string

	Screenshot() string

	From() string
}

type RequestResult struct {
	request  *rod.HijackRequest
	response *rod.HijackResponse
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
