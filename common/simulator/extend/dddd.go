// Package extend
// @Author bcy2007  2023/6/8 11:17
package extend

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/simulator/web"
	"strings"
)

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

func (dddd *DDDDCaptcha) GeneratorData() interface{} {
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
func (dddd *DDDDResult) GetSuccess() bool {
	if dddd.Result == "" {
		return false
	}
	return true
}

func TestDDDDOcr() {
	url := "http://192.168.0.115:9898/ocr/b64/json"
	//url := "http://192.168.0.58:8008/runtime/text/invoke"
	b64 := "iVBORw0KGgoAAAANSUhEUgAAACwAAAAUCAIAAAB5z0iWAAACFElEQVRIicWWz0sbQRTHvxNWECyEhMDqYenJ3YVC1FNBzSWnlp7E/gmWgnjx0D/BU/+CgvQsVCjYQ26BukEQemmgsK6HEFapA2HDggFBcTw8HSezP5piMF/2sPPmzb7Pe/NmWLbz5ScmLQPAp43aBAk+73rGiK6s9CLVLvqXT+fIhPBaDXVYSwTLwhonRG31bRbQ2DUEYZaL2jSPYq/VUIFIVIbDH9+g8CXdUtUbNCsz9XQIIuBRrFrMcnH/YC+VQPQvZT+ngmZJIwBQyPHmUbx/sKd9vTdoYrgfvVZjdf2NMAUAYQp6tE8JU/QGTTmrOTxWgkcxpU7DxZUlAFvbm0HoA7iI/s6W57D5EUDU/oXQty0XDzUQuCdgnEkUeicxziqog98DaYiPEEHoe7+P5dC23MXqy433H2iDbMuVuyD9T07bznw1CH2xDPvIkVEZZyqH2gREoPIBMKam+5RrbeE1hntClUZAWLzb4d0OAHRF8K7thFWya2vzCQAY11clWvYn/P7KWtMOSBYTSbaLOBc4B/vKqEIA5uFozlkEULejMlPPCTn6zUgpnSz7ABy4/yRAzmWliQ5F8nRlohw5eKiK3CC1JVWgUSHywydTJIsNl1BsrjeKqrx7YlyyLTcIfapKUkXj9jkgiINQUmefCUKipHKkQ1Ab/q9GWaVxnHUu4psCm8jv3dR0//qqBKBo3MY3hTt3I/ynYQuPcgAAAABJRU5ErkJggg=="
	d := DDDDCaptcha{}
	d.InputBase64(b64)
	//reqBody := strings.NewReader(b64)
	//httpReq, _ := http.NewRequest("POST", url, reqBody)
	//httpResp, _ := http.DefaultClient.Do(httpReq)
	//respBody, _ := ioutil.ReadAll(httpResp.Body)
	//fmt.Println("result: ", string(respBody), respBody)
	//req := &CaptchaRequest{
	//	Project_name: "common_alphanumeric",
	//	Image:        b64,
	//}
	resp, err := web.Do_Post(url, d)
	byteData := []byte(resp)
	capResult := DDDDResult{}
	if err = json.Unmarshal(byteData, &capResult); err != nil {
		fmt.Printf("unmarshal captcha result error: %s", err)
	}
	if !capResult.GetSuccess() {
		fmt.Printf("get captcha result success false: %s", string(capResult.GetErrorInfo()))
	}
	fmt.Println(capResult.GetResult())
}
