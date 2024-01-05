package behinder

import (
	"encoding/base64"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func Encryption(binCode, key []byte, script string) ([]byte, error) {
	var result []byte
	switch script {
	case ypb.ShellScript_JSPX.String():
		fallthrough
	case ypb.ShellScript_JSP.String():
		encCls, err := payloads.EncryptForJava(binCode, key)
		if err != nil {
			return nil, err
		}
		result = []byte(base64.StdEncoding.EncodeToString(encCls))
	case ypb.ShellScript_PHP.String():
		// 冰蝎很烦的是不确定用 aes 还是 xor
		encPhp, err := payloads.EncryptForPhp(binCode, key)
		if err != nil {
			return nil, err
		}
		result = []byte(base64.StdEncoding.EncodeToString(encPhp))
	case ypb.ShellScript_ASPX.String():
		encDll, err := payloads.EncryptForCSharp(binCode, key)
		if err != nil {
			return nil, err
		}
		result = encDll
	case ypb.ShellScript_ASP.String():
		result = payloads.Xor(binCode, key)
	}
	return result, nil
}

func Decryption(body, key []byte, script string) ([]byte, error) {
	var result []byte
	var err error
	switch script {
	case ypb.ShellScript_JSPX.String():
		fallthrough
	case ypb.ShellScript_JSP.String():
		// 冰蝎4 的 AES 加密结果套了一层 base64
		deCode, err := base64.StdEncoding.DecodeString(string(body))
		if err != nil {
			return nil, err
		}
		result, err = payloads.DecryptForJava(deCode, key)

	case ypb.ShellScript_PHP.String():
		result, err = payloads.DecryptForPhp(body, key)
	case ypb.ShellScript_ASPX.String():
		result, err = payloads.DecryptForCSharp(body, key)
	case ypb.ShellScript_ASP.String():
		result = payloads.Xor(body, key)
	}
	return result, err
}
