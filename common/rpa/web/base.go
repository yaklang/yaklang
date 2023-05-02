package web

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
)

type HTTPRspBody struct {
	Result Results `json:"Result"`
}

type Results struct {
	RequestID     string   `json:"RequestID"`
	HasError      bool     `json:"HasError"`
	ResponseItems ErrorMsg `json:"ResponseItems"`
}

type ErrorMsg struct {
	ErrorMsg string `json:"ErrorMsg"`
}

var postHeaders = []map[string]string{
	{"key": "Content-Type", "value": "application/json"},
}

func Do_Post(url string, v interface{}) (string, error) {
	reqParam, err := json.Marshal(v)
	if err != nil {
		return "", utils.Errorf("marshal data error:%s", err)
	}
	reqBody := strings.NewReader(string(reqParam))
	httpReq, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		return "", utils.Errorf("post request url: %s, req body: %s, error: %s", url, reqBody, err)
	}
	for _, h := range postHeaders {
		httpReq.Header.Add(h["key"], h["value"])
	}
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", utils.Errorf("do httpreq error:%s", err)
	}
	respBody, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return "", utils.Errorf("read http resp body %s error:%s", respBody, err)
	}
	var result HTTPRspBody
	if err = json.Unmarshal(respBody, &result); err != nil {
		return "", utils.Errorf("unmarshal data error:%s", err)
	}
	if result.Result.HasError {
		return "", utils.Errorf("post response: %s result error: %s", string(respBody), err)
	}
	return string(respBody), nil
}

func GetMainDomain(url string) string {
	regMainDomain, err := regexp.Compile("http(s??)://.+?/")
	if err != nil {
		log.Errorf("regexp get maindomain error: %s", err)
		return ""
	}
	maindomains := regMainDomain.FindAllString(url, -1)
	if len(maindomains) > 0 {
		return maindomains[0]
	}
	log.Errorf("url %s maindomain not found.", url)
	return ""
}

func HttpHandle(method, urlStr, data string) (*http.Response, error) {
	// fmt.Printf("%s:%s\n", method, urlStr)
	// client := &http.Client{}
	var req *http.Request
	var err error

	if data == "" {
		urlArr := strings.Split(urlStr, "?")
		if len(urlArr) == 2 {
			urlStr = urlArr[0] + "?" + getParseParam(urlArr[1])
		}
		req, err = http.NewRequest(method, urlStr, nil)
	} else {
		req, err = http.NewRequest(method, urlStr, strings.NewReader(data))
	}
	if err != nil {
		return nil, utils.Errorf("http new request error: %s", err)
	}
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36")
	resp, err := http.DefaultClient.Do(req)
	return resp, err
}

func getParseParam(param string) string {
	return url.PathEscape(param)
}
