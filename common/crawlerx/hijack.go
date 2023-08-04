// Package crawlerx
// @Author bcy2007  2023/8/1 11:09
package crawlerx

import (
	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
	"net/http"
	"net/url"
)

type HijackRequest interface {
	Type() proto.NetworkResourceType
	Method() string
	URL() *url.URL
	Header(key string) string
	Headers() proto.NetworkHeaders
	Body() string
	JSONBody() gson.JSON
	Req() *http.Request
}

type HijackResponse interface {
	Payload() *proto.FetchFulfillRequest
	Body() string
	Headers() http.Header
}

type TestHijackRequest struct {
	resourceType proto.NetworkResourceType
	method       string
	url          *url.URL
	headers      proto.NetworkHeaders
	body         gson.JSON
	req          *http.Request
}

func (testHijackRequest *TestHijackRequest) Type() proto.NetworkResourceType {
	return testHijackRequest.resourceType
}

func (testHijackRequest *TestHijackRequest) Method() string {
	return testHijackRequest.method
}

func (testHijackRequest *TestHijackRequest) URL() *url.URL {
	return testHijackRequest.url
}

func (testHijackRequest *TestHijackRequest) Header(key string) string {
	item, ok := testHijackRequest.headers[key]
	if !ok {
		return ""
	}
	return item.String()
}

func (testHijackRequest *TestHijackRequest) Headers() proto.NetworkHeaders {
	return testHijackRequest.headers
}

func (testHijackRequest *TestHijackRequest) Body() string {
	return testHijackRequest.body.String()
}

func (testHijackRequest *TestHijackRequest) JSONBody() gson.JSON {
	return testHijackRequest.body
}

func (testHijackRequest *TestHijackRequest) Req() *http.Request {
	return testHijackRequest.req
}
