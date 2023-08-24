// Package simulator
// @Author bcy2007  2023/8/21 13:25
package simulator

import (
	"reflect"
	"testing"
)

func TestCaptcha(t *testing.T) {
	img := `iVBORw0KGgoAAAANSUhEUgAAACwAAAAUCAIAAAB5z0iWAAACFElEQVRIicWWz0sbQRTHvxNWECyEhMDqYenJ3YVC1FNBzSWnlp7E/gmWgnjx0D/BU/+CgvQsVCjYQ26BukEQemmgsK6HEFapA2HDggFBcTw8HSezP5piMF/2sPPmzb7Pe/NmWLbz5ScmLQPAp43aBAk+73rGiK6s9CLVLvqXT+fIhPBaDXVYSwTLwhonRG31bRbQ2DUEYZaL2jSPYq/VUIFIVIbDH9+g8CXdUtUbNCsz9XQIIuBRrFrMcnH/YC+VQPQvZT+ngmZJIwBQyPHmUbx/sKd9vTdoYrgfvVZjdf2NMAUAYQp6tE8JU/QGTTmrOTxWgkcxpU7DxZUlAFvbm0HoA7iI/s6W57D5EUDU/oXQty0XDzUQuCdgnEkUeicxziqog98DaYiPEEHoe7+P5dC23MXqy433H2iDbMuVuyD9T07bznw1CH2xDPvIkVEZZyqH2gREoPIBMKam+5RrbeE1hntClUZAWLzb4d0OAHRF8K7thFWya2vzCQAY11clWvYn/P7KWtMOSBYTSbaLOBc4B/vKqEIA5uFozlkEULejMlPPCTn6zUgpnSz7ABy4/yRAzmWliQ5F8nRlohw5eKiK3CC1JVWgUSHywydTJIsNl1BsrjeKqrx7YlyyLTcIfapKUkXj9jkgiINQUmefCUKipHKkQ1Ab/q9GWaVxnHUu4psCm8jv3dR0//qqBKBo3MY3hTt3I/ynYQuPcgAAAABJRU5ErkJggg==`
	type args struct {
		url  string
		mode string
		req  requestStructor
		res  responseStructor
		arg  string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "normal",
			args: args{
				url:  "http://192.168.3.20:8008/runtime/text/invoke",
				mode: "common_alphanumeric",
				req:  &NormalCaptchaRequest{},
				res:  &NormalCaptchaResponse{},
				arg:  img,
			},
			want: "s7nz",
		},
		{
			name: "dddd",
			args: args{
				url:  "http://192.168.0.115:9898/ocr/b64/json",
				mode: "",
				req:  &DDDDCaptcha{},
				res:  &DDDDResult{},
				arg:  img,
			},
			want: "s7nz",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identifier := CaptchaIdentifier{
				identifierUrl:  tt.args.url,
				identifierMode: tt.args.mode,
				identifierReq:  tt.args.req,
				identifierRes:  tt.args.res,
			}
			got, err := identifier.detect(tt.args.arg)
			if err != nil {
				t.Error(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf(`identifier detect = %v, want %v`, got, tt.want)
			}
		})
	}
}
