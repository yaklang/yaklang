package totpproxy

import (
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/twofa"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// verifyTOTP 验证请求中的 TOTP 验证码
func verifyTOTP(config *ServerConfig, reqRaw []byte) error {
	totpCode := lowhttp.GetHTTPPacketHeader(reqRaw, config.TOTPHeader)
	if totpCode == "" {
		if config.Debug {
			log.Warnf("[totpproxy] missing TOTP code in header: %s", config.TOTPHeader)
		}
		return errors.New("missing TOTP verification code")
	}

	if !twofa.VerifyUTCCode(config.TOTPSecret, totpCode) {
		if config.Debug {
			log.Warnf("[totpproxy] invalid TOTP code: %s", totpCode)
		}
		return errors.New("invalid TOTP verification code")
	}

	if config.Debug {
		log.Infof("[totpproxy] TOTP verification passed")
	}
	return nil
}

// isPathAllowed 检查请求路径是否在白名单中
func isPathAllowed(config *ServerConfig, path string) bool {
	// 如果没有配置白名单，允许所有路径
	if len(config.AllowedPaths) == 0 {
		return true
	}

	for _, allowedPath := range config.AllowedPaths {
		if strings.HasPrefix(path, allowedPath) {
			return true
		}
	}
	return false
}

// GetTOTPCode 获取当前的 TOTP 验证码
func GetTOTPCode(secret string) string {
	return twofa.GetUTCCode(secret)
}

// VerifyTOTPCode 验证 TOTP 验证码
func VerifyTOTPCode(secret string, code any) bool {
	return twofa.VerifyUTCCode(secret, code)
}
