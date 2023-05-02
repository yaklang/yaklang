package core

import (
	"github.com/go-rod/rod/lib/proto"
	"net/http"
	netUrl "net/url"
	"strings"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
)

type ReqInfo interface {
	Url() string
	Method() string

	RequestHeaders() map[string]string
	RequestBody() string

	ResponseHeaders() map[string]string
	ResponseBody() string

	Req() *http.Request

	Tag() []string
	SetTag([]string)
	GetData(string) interface{}
}

type RequestInfo struct {
	url           string
	requestMethod string

	requestHeaders *proto.NetworkHeaders
	requestBody    string

	responseHeaders *http.Header
	responseBody    string

	req *http.Request

	tag []string
}

func (requestInfo *RequestInfo) Url() string {
	return requestInfo.url
}

func (requestInfo *RequestInfo) RequestHeaders() map[string]string {
	target := make(map[string]string, 0)
	for k, v := range *requestInfo.requestHeaders {
		target[k] = v.Str()
	}
	return target
}

func (requestInfo *RequestInfo) RequestBody() string {
	return requestInfo.requestBody
}

func (requestInfo *RequestInfo) Method() string {
	return requestInfo.requestMethod
}

func (requestInfo *RequestInfo) ResponseHeaders() map[string]string {
	tempHeaders := requestInfo.responseHeaders
	if tempHeaders == nil {
		return nil
	}
	result := make(map[string]string, 0)
	for k, v := range *tempHeaders {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

func (requestInfo *RequestInfo) ResponseBody() string {
	return requestInfo.responseBody
}

func (requestInfo *RequestInfo) Req() *http.Request {
	return requestInfo.req
}

func (requestInfo *RequestInfo) Tag() []string {
	return requestInfo.tag
}

func (requestInfo *RequestInfo) SetTag(tags []string) {
	requestInfo.tag = append(requestInfo.tag, tags...)
}

func (requestInfo *RequestInfo) GetData(dataType string) interface{} {
	switch dataType {
	case "response.url":
		return requestInfo.Url()
	case "response.html":
		return requestInfo.ResponseBody()
	case "response.responseHeader":
		return requestInfo.ResponseHeaders()
	case "response.url_param":
		url := requestInfo.Url()
		parsed, err := netUrl.Parse(url)
		if err != nil {
			return ""
		}
		return parsed.RawQuery
	case "response.path":
		url := requestInfo.Url()
		parsed, err := netUrl.Parse(url)
		if err != nil {
			return ""
		}
		return parsed.Path
	case "response.requestData":
		if strings.ToLower(requestInfo.Method()) == "post" {
			return requestInfo.RequestBody()
		}
		return ""
	default:
		return ""
	}
}

type SimpleRequest struct {
	url string
}

func (simpleReq *SimpleRequest) Url() string {
	return simpleReq.url
}

func (simpleReq *SimpleRequest) Method() string {
	return "get"
}

func (simpleReq *SimpleRequest) RequestHeaders() map[string]string {
	return nil
}

func (simpleReq *SimpleRequest) RequestBody() string {
	return ""
}

func (simpleReq *SimpleRequest) ResponseHeaders() map[string]string {
	return nil
}

func (simpleReq *SimpleRequest) ResponseBody() string {
	return ""
}

func (simpleReq *SimpleRequest) Req() *http.Request {
	return nil
}

func (simpleReq *SimpleRequest) Tag() []string {
	return nil
}

func (simpleReq *SimpleRequest) SetTag([]string) {}

func (simpleReq *SimpleRequest) GetData(dataType string) interface{} {
	switch dataType {
	case "response.url":
		return simpleReq.Url()
	case "response.url_param":
		url := simpleReq.Url()
		if strings.Contains(url, "?") {
			blocks := strings.Split(url, "?")
			if len(blocks) > 1 {
				return blocks[1]
			}
		}
		return ""
	default:
		return ""
	}
}

func (crawler *CrawlerX) SimpleCheckSend(urls ...string) {
	for _, url := range urls {
		repeatStr := crawler.checkRepeat(url, "get")
		hashStr := codec.Sha256(repeatStr)
		if crawler.sent.Exist(hashStr) {
			continue
		} else {
			crawler.sent.Insert(hashStr)
		}
		req := &SimpleRequest{}
		req.url = url
		if crawler.onRequest != nil {
			crawler.onRequest(req)
		} else if crawler.sendInfoChannel != nil {
			crawler.sendInfoChannel <- req
		} else {
			//log.Infof("get url: %s without request", req.Url())
		}
	}
}
