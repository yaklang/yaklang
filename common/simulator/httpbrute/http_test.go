// Package httpbrute
// @Author bcy2007  2023/7/31 14:09
package httpbrute

import (
	"net/url"
	"testing"
)

func TestConnectTest(t *testing.T) {
	type args struct {
		urlStr string
		proxy  *url.URL
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"captcha", args{urlStr: "http://192.168.3.20:8008/runtime/text/invoke", proxy: nil}, false},
		{"normal", args{urlStr: "http://192.168.0.68/#/login", proxy: nil}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, msg := ConnectTest(tt.args.urlStr, tt.args.proxy)
			t.Logf(`connect result: %v, connect info: %v`, got, msg)
			if got == true && msg != "" {
				t.Errorf("connect %v but got msg %v", got, msg)
			}
			if got == false && msg == "" {
				t.Errorf("connect %v but got msg %v", got, msg)
			}
		})
	}
}
