// Package crawlerx
// @Author bcy2007  2023/7/14 11:07
package crawlerx

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strconv"
)

type OutputResults struct {
	results []*OutputResult
}

type OutputResult struct {
	Url      string         `json:"url"`
	Request  OutputRequest  `json:"request"`
	Response OutputResponse `json:"response"`
}

type OutputRequest struct {
	Url     string          `json:"url"`
	Method  string          `json:"method"`
	Headers []*OutputHeader `json:"headers"`
	Body    OutputBody      `json:"body"`
	HTTPRaw string          `json:"http_raw"`
}

type OutputResponse struct {
	StatusCode int             `json:"status_code"`
	Headers    []*OutputHeader `json:"headers"`
	Body       OutputBody      `json:"body"`
}

type OutputHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type OutputBody struct {
	Size string `json:"size"`
	Md5  string `json:"md5"`
	Data string `json:"data"`
}

func GeneratorOutput(reqInfo ReqInfo) *OutputResult {
	requestHeaders := reqInfo.RequestHeaders()
	tempRequestHeaders := make([]*OutputHeader, 0)
	for k, v := range requestHeaders {
		tempRequestHeaders = append(tempRequestHeaders, &OutputHeader{k, v})
	}
	responseHeaders := reqInfo.ResponseHeaders()
	tempResponseHeaders := make([]*OutputHeader, 0)
	for k, v := range responseHeaders {
		tempResponseHeaders = append(tempResponseHeaders, &OutputHeader{k, v})
	}
	httpRaw, err := reqInfo.RequestRaw()
	if err != nil {
		log.Errorf("get http raw error: %v", err)
	}
	result := OutputResult{
		Url: reqInfo.Url(),
		Request: OutputRequest{
			Url:     reqInfo.Url(),
			Method:  reqInfo.Method(),
			Headers: tempRequestHeaders,
			Body: OutputBody{
				Md5:  codec.Md5(reqInfo.RequestBody()),
				Size: strconv.Itoa(len(reqInfo.RequestBody())),
				Data: reqInfo.RequestBody(),
			},
			HTTPRaw: string(httpRaw),
		},
		Response: OutputResponse{
			StatusCode: reqInfo.StatusCode(),
			Headers:    tempResponseHeaders,
			Body: OutputBody{
				Md5:  codec.Md5(reqInfo.ResponseBody()),
				Size: strconv.Itoa(len(reqInfo.ResponseBody())),
				Data: reqInfo.ResponseBody(),
			},
		},
	}
	return &result
}
