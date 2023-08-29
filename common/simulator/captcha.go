// Package simulator
// @Author bcy2007  2023/8/21 10:59
package simulator

import (
	"encoding/json"
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"net/http"
	"net/url"
	"strings"
)

const getImgB64Str = `
()=>{
	canvas = document.createElement("canvas");
	context = canvas.getContext("2d");
	canvas.height = this.naturalHeight;
	canvas.width = this.naturalWidth;
	context.drawImage(this, 0, 0, this.naturalWidth, this.naturalHeight);
	base64Str = canvas.toDataURL();
	return base64Str;
}`

type requestStructr interface {
	InputBase64(string)
	InputMode(string)
	Generate() interface{}
}

type responseStructr interface {
	GetResult() string
	GetErrorInfo() string
	GetStatus() bool
}

type NormalCaptchaRequest struct {
	ProjectName string `json:"project_name"`
	Image       string `json:"image"`
}

func (captchaRequest *NormalCaptchaRequest) InputBase64(b64 string) {
	captchaRequest.Image = b64
}

func (captchaRequest *NormalCaptchaRequest) InputMode(mode string) {
	captchaRequest.ProjectName = mode
}

func (captchaRequest *NormalCaptchaRequest) Generate() interface{} {
	return &captchaRequest
}

type NormalCaptchaResponse struct {
	Uuid    string `json:"uuid"`
	Data    string `json:"data"`
	Success bool   `json:"success"`
}

func (captchaResponse *NormalCaptchaResponse) GetResult() string {
	return captchaResponse.Data
}

func (captchaResponse *NormalCaptchaResponse) GetErrorInfo() string {
	return captchaResponse.Data
}

func (captchaResponse *NormalCaptchaResponse) GetStatus() bool {
	return captchaResponse.Success
}

type DDDDCaptcha struct {
	b64 string
}

func (dddd *DDDDCaptcha) InputBase64(b64 string) {
	var b64Code string
	if strings.HasPrefix(b64, "data:") && strings.Contains(b64, ",") {
		b64Code = strings.Split(b64, ",")[1]
	} else {
		b64Code = b64
	}
	dddd.b64 = b64Code
}

func (dddd *DDDDCaptcha) InputMode(string) {}

func (dddd *DDDDCaptcha) GetBase64() string {
	return dddd.b64
}

func (dddd *DDDDCaptcha) Generate() interface{} {
	return dddd.b64
}

type DDDDResult struct {
	Status  int    `json:"status"`
	Result  string `json:"result"`
	Message string `json:"msg"`
}

func (dddd *DDDDResult) GetResult() string {
	return dddd.Result
}

func (dddd *DDDDResult) GetErrorInfo() string {
	return dddd.Message
}
func (dddd *DDDDResult) GetStatus() bool {
	if dddd.Result == "" {
		return false
	}
	return true
}

type CaptchaIdentifier struct {
	identifierUrl  string
	identifierMode string
	identifierReq  requestStructr
	identifierRes  responseStructr
	proxy          *url.URL
}

func (identifier *CaptchaIdentifier) SetUrl(url string) {
	identifier.identifierUrl = url
}

func (identifier *CaptchaIdentifier) SetMode(mode string) {
	identifier.identifierMode = mode
}

func (identifier *CaptchaIdentifier) SetRequest(req requestStructr) {
	identifier.identifierReq = req
}

func (identifier *CaptchaIdentifier) SetResponse(res responseStructr) {
	identifier.identifierRes = res
}

func (identifier *CaptchaIdentifier) SetProxy(proxy *url.URL) {
	identifier.proxy = proxy
}

func (identifier *CaptchaIdentifier) elementDetect(page *rod.Page, elementSelector string) (*rod.Element, error) {
	elements, err := page.Elements(elementSelector)
	if err != nil {
		return nil, utils.Error(err)
	}
	if elements.Empty() {
		return nil, utils.Error(`element selector not found in page`)
	}
	return elements.First(), nil
}

func (identifier *CaptchaIdentifier) b64Detect(element *rod.Element) (string, error) {
	tagName, err := GetProperty(element, "tagName")
	if err != nil {
		return "", utils.Error(err)
	}
	if strings.ToLower(tagName) != "img" {
		return "", utils.Errorf(`captcha element tag error: %v`, tagName)
	}
	src, err := GetAttribute(element, "src")
	if err != nil {
		return "", utils.Error(err)
	}
	if src == "" {
		return "", utils.Error(`element without src`)
	}
	var imgB64 string
	if strings.HasPrefix(src, "data:image") {
		imgB64 = src
	} else {
		obj, err := element.Eval(getImgB64Str)
		if err != nil {
			return "", utils.Error(err)
		}
		imgB64 = obj.Value.String()
	}
	return imgB64, nil
}

func (identifier *CaptchaIdentifier) detect(imgB64 string) (string, error) {
	if identifier.identifierUrl == "" {
		return "", utils.Error(`identifier url not exist`)
	}
	if identifier.identifierReq == nil || identifier.identifierRes == nil {
		identifier.identifierReq = &NormalCaptchaRequest{}
		identifier.identifierRes = &NormalCaptchaResponse{}
	}
	identifier.identifierReq.InputBase64(imgB64)
	identifier.identifierReq.InputMode(identifier.identifierMode)
	reqBody, err := json.Marshal(identifier.identifierReq.Generate())
	if err != nil {
		return "", utils.Error(err)
	}
	request, err := http.NewRequest("POST", identifier.identifierUrl, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", utils.Error(err)
	}
	request.Header.Add("Content-Type", "application/json")
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithRequest(request),
	}
	if identifier.proxy != nil {
		opts = append(opts, lowhttp.WithProxy(identifier.proxy.String()))
	}
	response, err := lowhttp.HTTP(opts...)
	if err != nil {
		return "", utils.Error(err)
	}
	_, resBody := lowhttp.SplitHTTPPacketFast(response.RawPacket)
	if err = json.Unmarshal(resBody, &identifier.identifierRes); err != nil {
		return "", utils.Error(err)
	}
	if !identifier.identifierRes.GetStatus() {
		return "", utils.Error(identifier.identifierRes.GetErrorInfo())
	}
	return identifier.identifierRes.GetResult(), nil

}

func (identifier *CaptchaIdentifier) Detect(page *rod.Page, elementSelector string) (string, error) {
	element, err := identifier.elementDetect(page, elementSelector)
	if err != nil {
		return "", utils.Error(err)
	}
	b64, err := identifier.b64Detect(element)
	if err != nil {
		return "", utils.Error(err)
	}
	result, err := identifier.detect(b64)
	if err != nil {
		return "", utils.Error(err)
	}
	return result, nil
}
