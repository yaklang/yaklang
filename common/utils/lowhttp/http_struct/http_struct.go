package http_struct

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type HTTPConfig struct {
	Timeout    float64
	Proxies    []string
	Redirector func(i *http.Request, reqs []*http.Request) bool
	Session    interface{}
	GetParams  map[string]string
	Body       []byte
	Headers    map[string]string
}

func NewHTTPConfig() *HTTPConfig {
	return &HTTPConfig{
		Timeout:   15,
		GetParams: make(map[string]string),
		Headers:   make(map[string]string),
	}
}

type HttpOption func(req *HTTPConfig)

type YakHttpRequest struct {
	*http.Request
	Config *HTTPConfig
}

type YakHttpResponse struct {
	*http.Response
}

func (y *YakHttpResponse) Json() interface{} {
	data := y.Data()
	if data == "" {
		return nil
	}
	var i interface{}
	err := json.Unmarshal([]byte(data), &i)
	if err != nil {
		log.Errorf("parse %v to json failed: %v", strconv.Quote(data), err)
		return ""
	}
	return i
}

func (y *YakHttpResponse) Data() string {
	if y.Response == nil {
		log.Error("response empty")
		return ""
	}

	if y.Response.Body == nil {
		return ""
	}

	body, _ := ioutil.ReadAll(y.Response.Body)
	y.Response.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	return string(body)
}

func (y *YakHttpResponse) GetHeader(key string) string {
	return y.Response.Header.Get(key)
}

func (y *YakHttpResponse) Raw() []byte {
	raw, _ := utils.DumpHTTPResponse(y.Response, true)
	return raw
	// raw, _ := dumpWithBody(y, true)
	// return raw
}
