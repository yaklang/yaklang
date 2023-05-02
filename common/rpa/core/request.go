package core

import (
	"bufio"
	"bytes"
	"net/http"
	"strings"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
)

type RequestIf interface {
	Url() string
	Request() *http.Request
	ResponseBody() []byte
	Response() (*http.Response, error)
	RequestHeader() map[string][]string
	ResponseHeader() map[string][]string
}

type MakeReq struct {
	url string
}

func (r *MakeReq) Url() string {
	return r.url
}

func (r *MakeReq) Request() *http.Request {
	return nil
}

func (r *MakeReq) ResponseBody() []byte {
	return nil
}

func (r *MakeReq) Response() (*http.Response, error) {
	return nil, nil
}

func (r *MakeReq) RequestHeader() map[string][]string {
	return map[string][]string{}
}

func (r *MakeReq) ResponseHeader() map[string][]string {
	return map[string][]string{}
}

func (r *Req) Url() string {
	return r.request.URL.String()
}

func (r *Req) IsHttps() bool {
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(r.request.URL.String())), "https://")
}

func (r *Req) Request() *http.Request {
	reqIns, err := http.ReadRequest(bufio.NewReader(bytes.NewBuffer(r.requestRaw)))
	if err != nil {
		log.Errorf("read request failed: %s", err)
	}
	return reqIns
}

func (r *Req) RequestRaw() []byte {
	return r.requestRaw
}

func (r *Req) ResponseBody() []byte {
	return r.responseBody
}

func (r *Req) Response() (*http.Response, error) {
	if r.response == nil {
		if r.err == nil {
			return nil, utils.Errorf("BUG: crawler.req no response and error")
		}
		return nil, r.err
	}
	return r.response, nil
}

func (r *Req) RequestHeader() map[string][]string {
	return r.request.Header
}

func (r *Req) ResponseHeader() map[string][]string {
	return *r.responseHeaders
}
