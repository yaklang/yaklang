package twofa

import (
	"encoding/base32"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
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

// GetUTCCode in twofa lib will receive the secret and return the verify code with utc time
func GetUTCCode(secret string) string {
	return NewTOTPConfig(secret).GetToptUTCCodeString()
}

// VerifyUTCCode in twofa lib will receive the secret and code, then return the verify result
func VerifyUTCCode(secret string, code any) bool {
	result, err := NewTOTPConfig(secret).Authenticate(code)
	if err != nil {
		log.Warnf("error happened in verifying code: %v", err)
		return false
	}
	return result
}

var Exports = map[string]any{
	"TOTPCode":      GetUTCCode,
	"TOTPVerify":    VerifyUTCCode,
	"GetUTCCode":    GetUTCCode,
	"VerifyUTCCode": VerifyUTCCode,

	"poc": WithTwoFa,
}

// poc 是一个请求选项，设置 Y-T-Verify-Code 的值为 secret 计算出的 UTC 时间验证码，适配于 poc 包
func WithTwoFa(secret string) poc.PocConfigOption {
	return poc.WithReplaceHttpPacketHeader("Y-T-Verify-Code", GetUTCCode(secret))
}
