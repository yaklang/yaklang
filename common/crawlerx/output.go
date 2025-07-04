// Package crawlerx
// @Author bcy2007  2023/7/14 11:07
package crawlerx

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/crawlerx/tools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"strconv"
	"strings"
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

// OutputResult 将channel中输出的爬虫结果保存在本地
//
// 第一个参数为需要存储的结果 第二个参数为保存的本地路径 请确保本地文件可以正常写入
//
// Examples:
//
//		```
//			targetUrl = "http://testphp.vulnweb.com/"
//			ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.pageTimeout(30), crawlerx.concurrent(3))
//			resultList = []
//			for item = range ch {
//				yakit.Info(item.Method() + " " + item.Url())
//				resultList = append(resultList, item)
//			}
//			err = crawlerx.OutputResult(resultList, "test.txt")
//			if err != nil {
//	            println(err)
//			}
//
//		```
func OutputData(data []interface{}, outputFile string) error {
	var result []*OutputResult
	for _, item := range data {
		temp, ok := item.(ReqInfo)
		if !ok {
			continue
		}
		outputResult := GeneratorOutput(temp)
		if outputResult != nil {
			result = append(result, outputResult)
		}
	}
	resultBytes, _ := json.MarshalIndent(result, "", "\t")
	return tools.WriteFile(outputFile, resultBytes)
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
		if k == "Content-Type" && !checkContentType(v) {
			return nil
		}
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

var contentTypeReg = regexp.MustCompile(`/json|/java|/xml|encoded`)

func checkContentType(contentType string) bool {
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	if contentTypeReg.FindString(contentType) != "" {
		return true
	}
	return false
}
