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
	PocOpts []any // poc.PocConfigOption
}

func NewHTTPConfig() *HTTPConfig {
	return &HTTPConfig{
		PocOpts: make([]any, 0),
	}
}

func (c *HTTPConfig) AppendPocOpts(opt any) {
	c.PocOpts = append(c.PocOpts, opt)
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
