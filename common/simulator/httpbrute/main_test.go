// Package httpbrute
// @Author bcy2007  2023/6/21 14:04
package httpbrute

import (
	"testing"
)

func TestHttpBruteForce(t *testing.T) {
	urlStr := "http://192.168.0.203/#/login"
	captchaUrl := "http://192.168.3.20:8008/runtime/text/invoke"
	opts := make([]BruteConfigOpt, 0)
	opts = append(opts,
		WithCaptchaUrl(captchaUrl),
		WithCaptchaMode("common_arithmetic"),
		WithUsername("admin"),
		WithPassword("admin", "luckyadmin123"),
	)
	ch, _ := HttpBruteForce(urlStr, opts...)
	for item := range ch {
		t.Logf(`[bruteforce] %s:%s login %v with info: %s`, item.Username(), item.Password(), item.Status(), item.Info())
		if item.Status() == true {
			t.Log(item.Base64())
		}
	}
}
