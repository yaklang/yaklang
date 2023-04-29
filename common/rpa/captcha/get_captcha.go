package captcha

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"yaklang/common/rpa/web"
	"yaklang/common/utils"
	"strings"

	"github.com/go-rod/rod"
)

type BaseCaptcha interface {
	GetCaptcha() string
}

type TestCaptcha struct {
}

func (testCap *TestCaptcha) GetCaptcha() string {
	return "aaaa"
}

type Captcha struct {
	// captcha pic base64 code string
	Base64str string

	// captcha input element && captcha IMG element
	Feature_element *rod.Element
	cap_element     *rod.Element

	//necessary domain
	Domain string

	// search captcha url
	CaptchaUrl string
}

func (cap *Captcha) SetCapElement(c *rod.Element) {
	cap.cap_element = c
}

func (cap *Captcha) GetCaptcha() (string, error) {
	var err error
	if cap.cap_element == nil {
		err := cap.getCapElement()
		if err != nil {
			return "", utils.Errorf("search captcha element error:%s", err)
		}
	}
	err = cap.getBase64Captcha()
	if err != nil {
		return "", utils.Errorf("search captcha base64 string error:%s", err)
	}
	req := &CaptchaRequest{
		Project_name: "common_alphanumeric",
		Image:        cap.Base64str,
	}
	// fmt.Printf("base64Str: %s\n", cap.Base64str)
	if cap.CaptchaUrl == "" {
		return "", utils.Error("captcha detect url not found.")
	}
	resp, err := web.Do_Post(cap.CaptchaUrl, req)
	byteData := []byte(resp)
	var capResult CaptchaResult
	if err = json.Unmarshal(byteData, &capResult); err != nil {
		return "", utils.Errorf("unmarshal captcha result error:%s", err)
	}
	// fmt.Println("get captcha result: ", capResult)
	if !capResult.Success {
		return "", utils.Errorf("get captcha result success false:%s", string(capResult.Data))
	}
	return capResult.Data, nil
}

func (cap *Captcha) getCapElement() error {
	img_element, err := cap.Feature_element.ElementByJS(rod.Eval(`()=>{
		ele = this;
		for(var i = 0;i<3;i++){
			parent = ele.parentElement;
			imgs = parent.getElementsByTagName("img");
			if (imgs.length>0){
				break
			}
			ele = parent
		}
		if (imgs.length <=0){
			return undefined
		}else{
			return imgs[0]
		}
	}`))
	if err != nil {
		return utils.Errorf("get captcha element error:%s ", err)
	}
	cap.cap_element = img_element
	return nil
}

func (cap *Captcha) getBase64Captcha() error {
	urlStr, err := cap.cap_element.Attribute("src")
	if err != nil {
		return utils.Errorf("get captcha element src error:%s", err)
	}
	if urlStr == nil {
		return utils.Errorf("captcha element src nil.")
	}
	if strings.HasPrefix(*urlStr, "data:image/") {
		cap.Base64str = *urlStr
		return nil
	}
	if strings.HasPrefix(*urlStr, "/") {
		if cap.Domain == "" {
			return utils.Errorf("pic element just has sub path without maindomain cannot find path.")
		}
		*urlStr = cap.Domain + *urlStr
	}
	// fmt.Printf("get base64: %s\n", *urlStr)
	b64str, err := getBase64fromWebImage(*urlStr)
	if err != nil {
		return utils.Errorf("get web image base64 error:%s", err)
	}
	cap.Base64str = b64str
	return nil
}

func getBase64fromWebImage(urlStr string) (string, error) {
	// resp, err := http.Get(urlStr)
	resp, err := web.HttpHandle("GET", urlStr, "")
	if err != nil {
		return "", utils.Errorf("http get pic err:%s", err)
	}
	defer resp.Body.Close()
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", utils.Errorf("read body err:%s", err)
	}
	var base64Encoding string
	mimeType := http.DetectContentType(bytes)
	// fmt.Println("bytes: ", string(bytes), " mimeType: ", mimeType)
	switch mimeType {
	case "image/jpeg":
		base64Encoding += "data:image/jpeg;base64,"
	case "image/png":
		base64Encoding += "data:image/png;base64,"
	default:
		base64Encoding += "data:" + mimeType + ";base64,"
		// base64Encoding += "data:image/jpeg;base64,"
	}
	base64Encoding += base64.StdEncoding.EncodeToString(bytes)
	return base64Encoding, nil
}
