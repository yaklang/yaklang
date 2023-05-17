// Package newcrawlerx
// @Author bcy2007  2023/3/23 10:47
package newcrawlerx

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type Temp struct {
	username string
	password string
}

type HttpRequest struct {
	url            string
	method         string
	params         map[string]string
	files          map[string]string
	defaultHeaders map[string]string

	proxy  *url.URL
	client *http.Client

	req *http.Request
	res *http.Response
}

func CreateGetRequest(url string) *HttpRequest {
	r := HttpRequest{
		url: url,
	}
	r.init()
	return &r
}

func CreateFileRequest(url, method string, params, files map[string]string) *HttpRequest {
	r := HttpRequest{
		url:    url,
		method: method,
		params: params,
		files:  files,
	}
	r.init()
	return &r
}

func (request *HttpRequest) init() {
	transport := http.Transport{}
	if request.proxy != nil {
		transport.Proxy = http.ProxyURL(request.proxy)
	}
	client := &http.Client{
		Transport: &transport,
	}
	request.client = client
}

func (request *HttpRequest) Request() error {
	if request.method == "POST" {
		if len(request.files) > 0 {
			return request.MultiPartRequest()
		} else {
			return request.PostRequest()
		}
	} else if request.method == "GET" {
		return request.GetRequest()
	}
	return utils.Errorf("error request method: %s", request.method)
}

func (request *HttpRequest) GetRequest() error {
	//paramsToStr(request.params)
	req, err := http.NewRequest("GET", request.url, nil)
	if err != nil {
		return utils.Errorf("[get request]create http new request error: %s", err)
	}
	for k, v := range request.defaultHeaders {
		req.Header.Set(k, v)
	}
	request.req = req
	return nil
}

func (request *HttpRequest) PostRequest() error {
	paramJson := paramsToBytes(request.params)
	reader := bytes.NewReader(paramJson)
	req, err := http.NewRequest(request.method, request.url, reader)
	if err != nil {
		return utils.Errorf("[post request]create http new request error: %s", err)
	}
	for k, v := range request.defaultHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.req = req
	return nil
}

func (request *HttpRequest) MultiPartRequest() error {
	buffer := bytes.Buffer{}
	writer := multipart.NewWriter(&buffer)
	for fileName, filePath := range request.files {
		err := writeFile(writer, fileName, filePath)
		if err != nil {
			writer.Close()
			return utils.Errorf("write file error: %s", err)
		}
	}
	for key, value := range request.params {
		err := writeField(writer, key, value)
		if err != nil {
			writer.Close()
			return utils.Errorf("write field error: %s", err)
		}
	}
	writer.Close()
	req, err := http.NewRequest(request.method, request.url, &buffer)
	if err != nil {
		return utils.Errorf("create http new request error: %s", err)
	}
	for k, v := range request.defaultHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	request.req = req
	return nil
}

func (request *HttpRequest) Do() error {
	if request.req == nil {
		return utils.Errorf("null req")
	}
	if request.client == nil {
		return utils.Errorf("null client")
	}
	res, err := request.client.Do(request.req)
	if err != nil {
		return utils.Errorf("client do send request error: %s", err)
	}
	request.res = res
	return nil
}

func (request *HttpRequest) Show() (string, error) {
	//fmt.Println(request.res.)
	bodyBytes, err := ioutil.ReadAll(request.res.Body)
	if err != nil {
		return "", utils.Errorf("read response body error: %s", err)
	}
	//fmt.Println(string(bodyBytes))
	return string(bodyBytes), nil
}

func writeFile(writer *multipart.Writer, filename, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return utils.Errorf("file open error: %s", err)
	}
	defer f.Close()
	formFile, err := writer.CreateFormFile(filename, filePath)
	if err != nil {
		return utils.Errorf("writer create form file error: %s", err)
	}
	_, err = io.Copy(formFile, f)
	if err != nil {
		return utils.Errorf("io copy error: %s", err)
	}
	return nil
}

func writeField(writer *multipart.Writer, key, value string) error {
	formField, err := writer.CreateFormField(key)
	if err != nil {
		return utils.Errorf("writer create form field error: %s", err)
	}
	_, err = formField.Write([]byte(value))
	if err != nil {
		return utils.Errorf("write bytes error: %s", err)
	}
	return nil
}

func paramsToStr(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	var items []string
	for k, v := range params {
		items = append(items, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(items, "&")
}

func paramsToBytes(params map[string]string) []byte {
	return []byte(paramsToStr(params))
}
