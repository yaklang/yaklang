package extend

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/simulator/core"
	"github.com/yaklang/yaklang/common/simulator/web"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

type requestStructor interface {
	InputBase64(string)
	InputMode(string)
	GetBase64() string
	GeneratorData() interface{}
}

type CaptchaRequest struct {
	Project_name string `json:"project_name"`
	Image        string `json:"image"`
}

func (req *CaptchaRequest) InputBase64(b64 string) {
	req.Image = b64
}

func (req *CaptchaRequest) InputMode(mode string) {
	req.Project_name = mode
}

func (req *CaptchaRequest) GetBase64() string {
	return req.Image
}

func (req *CaptchaRequest) GeneratorData() interface{} {
	return &req
}

type responseStructor interface {
	GetResult() string
	GetErrorInfo() string
	GetSuccess() bool
}

type CaptchaResult struct {
	Uuid    string `json:"uuid"`
	Data    string `json:"data"`
	Success bool   `json:"success"`
}

func (resp *CaptchaResult) GetResult() string {
	return resp.Data
}

func (resp *CaptchaResult) GetErrorInfo() string {
	return resp.Data
}

func (resp *CaptchaResult) GetSuccess() bool {
	return resp.Success
}

type CaptchaIdentifier struct {
	identifierUrl  string
	identifierMode string
	requestStruct  requestStructor
	responseStruct responseStructor
}

func (identifier *CaptchaIdentifier) SetIdentifyUrl(url string) {
	identifier.identifierUrl = url
}

func (identifier *CaptchaIdentifier) SetRequestStruct(req requestStructor) {
	identifier.requestStruct = req
}

func (identifier *CaptchaIdentifier) SetResponseStruct(resp responseStructor) {
	identifier.responseStruct = resp
}

func (identifier *CaptchaIdentifier) SetIdentifyMode(mode string) {
	identifier.identifierMode = mode
}

func (identifier *CaptchaIdentifier) Detect(generalElement *core.GeneralElement) (string, error) {
	if identifier.identifierUrl == "" {
		return "", utils.Errorf("identifier url not exist")
	}
	if identifier.requestStruct == nil || identifier.responseStruct == nil {
		identifier.requestStruct = &CaptchaRequest{}
		identifier.responseStruct = &CaptchaResult{}
	}
	propertyStr, err := generalElement.GetProperty("tagName")
	if err != nil {
		err = generalElement.Redirect()
		if err != nil {
		}
		propertyStr, _ = generalElement.GetProperty("tagName")
	}
	if propertyStr != "img" {
		return "", utils.Errorf("captcha element %s tag error: %s", generalElement, propertyStr)
	}
	imgSrc, err := generalElement.GetAttributeOrigin("src")
	if err != nil {
		return "", utils.Errorf("get attribute src error: %s", err)
	}
	if imgSrc == "" {
		return "", utils.Errorf("element without src")
	}
	var imgBase64 string
	if strings.HasPrefix(imgSrc, "data:image") {
		imgBase64 = imgSrc
	} else {
		imgBase64 = generalElement.Eval(GETIMGB64STR)
	}
	req := identifier.requestStruct
	req.InputBase64(imgBase64)
	if identifier.identifierMode != "" {
		req.InputMode(identifier.identifierMode)
	} else {
		req.InputMode("common_alphanumeric")
	}
	resp, err := web.Do_Post(identifier.identifierUrl, req.GeneratorData())
	if err != nil {
		return "", utils.Errorf("post captcha req error: %s", err)
	}
	byteData := []byte(resp)
	capResult := identifier.responseStruct
	if err = json.Unmarshal(byteData, &capResult); err != nil {
		return "", utils.Errorf("unmarshal captcha result error: %s", err)
	}
	if !capResult.GetSuccess() {
		return "", utils.Errorf("get captcha result success false: %s", string(capResult.GetErrorInfo()))
	}
	return capResult.GetResult(), nil
}

func CreateCaptcha() *CaptchaIdentifier {
	identify := &CaptchaIdentifier{}
	return identify
}
