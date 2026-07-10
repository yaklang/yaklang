package dingtalk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// hmacSHA256 计算 HMAC-SHA256。
func hmacSHA256(key, msg []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	return mac.Sum(nil)
}

// base64Std 标准base64编码。
func base64Std(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
