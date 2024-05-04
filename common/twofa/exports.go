package twofa

import (
	"encoding/base32"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/url"
	"rsc.io/qr"
	"strings"
)

func NewTOTPConfig(secret string) *OTPConfig {
	return &OTPConfig{
		Secret:      codec.EncodeBase32(secret),
		WindowSize:  1,
		HotpCounter: 0, // 0 means totp
		UTC:         true,
	}
}

func GenerateQRCode(name, account string, token string) (*url.URL, []byte, error) {
	urlBase, _ := url.Parse("otpauth://totp")
	urlBase.Path += "/" + url.PathEscape(name) + ":" + url.PathEscape(account)
	params := url.Values{}
	params.Add("secret", base32.StdEncoding.EncodeToString([]byte(token)))
	params.Add("issuer", name)
	urlBase.RawQuery = params.Encode()
	code, err := qr.Encode(strings.TrimSpace(urlBase.String()), qr.Q)
	if err != nil {
		return nil, nil, err
	}
	b := code.PNG()
	return urlBase, b, nil
}
