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

// GetUTCCode 根据给定的密钥(secret)计算当前 UTC 时间下的 TOTP 动态验证码
// 该函数等价于 twofa.TOTPCode，生成的验证码为 6 位数字字符串，每 30 秒变化一次
// 参数:
//   - secret: 用于生成 TOTP 验证码的密钥字符串
//
// 返回值:
//   - 当前时间窗口内的 6 位数字验证码字符串
//
// Example:
// ```
// secret = "hello-yak-secret"
// code = twofa.GetUTCCode(secret)
// // 验证码恒为 6 位数字
// assert len(code) == 6, "TOTP code should be 6 digits"
// // 同一密钥同一时间生成的验证码可被成功校验
// ok = twofa.VerifyUTCCode(secret, code)
// println(ok)   // OUT: true
// assert ok == true, "freshly generated code should verify"
// ```
func GetUTCCode(secret string) string {
	return NewTOTPConfig(secret).GetToptUTCCodeString()
}

// VerifyUTCCode 校验给定的验证码(code)是否与密钥(secret)在当前 UTC 时间窗口内匹配
// 该函数等价于 twofa.TOTPVerify，校验失败或发生错误时返回 false
// 参数:
//   - secret: 用于生成 TOTP 验证码的密钥字符串
//   - code: 待校验的验证码，可以是字符串或整数
//
// 返回值:
//   - 校验是否通过的布尔值
//
// Example:
// ```
// secret = "hello-yak-secret"
// // 用同一密钥生成验证码并立即校验，往返必然成功
// code = twofa.GetUTCCode(secret)
// ok = twofa.VerifyUTCCode(secret, code)
// println(ok)   // OUT: true
// assert ok == true, "round-trip verify should pass"
// ```
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

// WithTwoFa 返回一个 poc 请求选项，自动把请求头 Y-T-Verify-Code 设置为
// 由 secret 计算出的当前 UTC 时间 TOTP 验证码，便于对需要二次验证的接口发起请求
// 该函数在 yak 中通过 twofa.poc 调用
// 参数:
//   - secret: 用于生成 TOTP 验证码的密钥字符串
//
// 返回值:
//   - 可传入 poc 系列函数的请求配置选项
//
// Example:
// ```
// // 该示例为示意性用法：把 TOTP 验证码自动注入到请求头中
// raw = "GET /api/profile HTTP/1.1\r\nHost: example.com\r\n\r\n"
// rsp, req = poc.HTTP(raw, twofa.poc("hello-yak-secret"), poc.timeout(5))~
// ```
func WithTwoFa(secret string) poc.PocConfigOption {
	return poc.WithReplaceHttpPacketHeader("Y-T-Verify-Code", GetUTCCode(secret))
}
