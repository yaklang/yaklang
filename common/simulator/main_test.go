// Package simulator
// @Author bcy2007  2023/8/23 14:49
package simulator

import "testing"

func TestHttpBruteForce(t *testing.T) {
	targetUrl := "http://192.168.0.68/#/login"
	opts := []BruteConfigOpt{
		WithExePath(`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`),
		WithCaptchaUrl(`http://192.168.3.20:8008/runtime/text/invoke`),
		WithCaptchaMode(`common_arithmetic`),
		WithUsernameList("admin"),
		WithPasswordList("admin", "admin123321"),
		WithExtraWaitLoadTime(1000),
	}
	ch, err := HttpBruteForce(targetUrl, opts...)
	if err != nil {
		t.Error(err)
	}
	for item := range ch {
		t.Logf(`[bruteforce] %s:%s login %v with url: %s`, item.Username(), item.Password(), item.Status(), item.Info())
		if item.Status() == true {
			t.Log(item.Base64())
		}
	}
}
