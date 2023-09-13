// Package crawlerx
// @Author bcy2007  2023/7/17 11:01
package crawlerx

import (
	"bytes"
	"fmt"
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func (starter *BrowserStarter) HttpPostFile(element *rod.Element) error {
	formElement, err := element.Parent()
	if err != nil {
		return utils.Errorf("get element parent error: %s", err)
	}
	// get post url
	var postUrl string
	baseUrlObj, err := formElement.Eval(`()=>document.URL`)
	if err != nil {
		return utils.Errorf("cannot get page url: %s", err)
	}
	baseUrl := baseUrlObj.Value.String()
	action, _ := getAttribute(formElement, "action")
	if action == "" {
		return utils.Errorf("cannot get file post url")
	} else if action == "#" {
		postUrl = baseUrl
	} else {
		baseUrlParse, _ := url.Parse(baseUrl)
		postUrlParse, _ := baseUrlParse.Parse(action)
		postUrl = postUrlParse.String()
	}
	// get post params
	inputElements, err := formElement.Elements("input")
	formValues := make(map[string]string)
	fileValues := make(map[string]string)
	for _, inputElement := range inputElements {
		name, _ := getAttribute(inputElement, "name")
		if name == "" {
			continue
		}
		value, _ := getAttribute(inputElement, "value")
		if value != "" {
			formValues[name] = value
			continue
		}
		elementType, _ := getAttribute(inputElement, "type")
		if elementType == "file" {
			fileValues[name] = starter.GetUploadFile(element)
		} else if elementType == "reset" || elementType == "submit" {
			continue
		} else if StringArrayContains(inputStringElementTypes, elementType) {
			formValues[name] = starter.GetFormFill(element)
		}
	}
	// do post
	// tbc
	r := CreateFileRequest(postUrl, "POST", formValues, fileValues)
	r.Request()
	r.Do()
	return nil
}

func (starter *BrowserStarter) GetFormFill(element *rod.Element) string {
	keywords := getAllKeywords(element)
	for k, v := range starter.formFill {
		if strings.Contains(keywords, k) {
			return v
		}
	}
	return "test"
}

func (starter *BrowserStarter) GetUploadFile(element *rod.Element) string {
	keywords := getAllKeywords(element)
	for k, v := range starter.fileUpload {
		if strings.Contains(keywords, k) {
			return v
		}
	}
	v, ok := starter.fileUpload["default"]
	if !ok {
		return ""
	}
	return v
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

func (request *HttpRequest) init() {
	if request.proxy != nil {
		request.client = netx.NewDefaultHTTPClient(request.proxy.String())
	} else {
		request.client = netx.NewDefaultHTTPClient()
	}
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
	var reader *bytes.Reader
	if len(request.params) == 0 {
		reader = nil
	} else {
		paramJson := paramsToBytes(request.params)
		reader = bytes.NewReader(paramJson)
	}
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
	bodyBytes, err := io.ReadAll(request.res.Body)
	if err != nil {
		return "", utils.Errorf("read response body error: %s", err)
	}
	return request.res.Request.URL.String() + " " + string(bodyBytes), nil
}

func (request *HttpRequest) GetUrl() string {
	return request.res.Request.URL.String()
}

func CreateRequest() *HttpRequest {
	return &HttpRequest{}
}

func CreateGetRequest(url string) *HttpRequest {
	r := HttpRequest{
		url:            url,
		method:         "GET",
		defaultHeaders: defaultChromeHeaders,
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
