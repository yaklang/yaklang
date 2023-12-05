package crawler

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
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

func (r *Req) Url() string {
	if r.url != "" {
		return r.url
	}
	return r.request.URL.String()
}

func (r *Req) Request() *http.Request {
	reqIns, err := utils.ReadHTTPRequestFromBytes(r.requestRaw)
	if err != nil {
		log.Errorf("read request failed: %s", err)
	}
	return reqIns
}

func (r *Req) RequestRaw() []byte {
	return r.requestRaw
}

func (r *Req) ResponseRaw() []byte {
	return r.responseRaw
}

func (r *Req) ResponseBody() []byte {
	return r.responseBody
}

func (r *Req) IsHttps() bool {
	return r.https
}

func (r *Req) Response() (*http.Response, error) {
	if r.response == nil {
		if r.err == nil {
			return nil, utils.Errorf("BUG: crawler.Req no response and error")
		}
		return nil, r.err
	}
	return r.response, nil
}
