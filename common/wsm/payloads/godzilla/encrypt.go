package godzilla

import (
	"encoding/base64"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"regexp"
)

func Encryption(payload, key []byte, pass, encMode, script string, gzip bool) ([]byte, error) {
	var enPayload []byte
	var err error
	if script != ypb.ShellScript_ASP.String() && gzip {
		payload, err = utils.GzipCompress(payload)
		if err != nil {
			return nil, err
		}
	}
	switch script {
	case ypb.ShellScript_JSPX.String():
		fallthrough
	case ypb.ShellScript_JSP.String():
		enPayload, err = payloads.EncryptForJava(payload, key)
		if err != nil {
			return nil, err
		}
	case ypb.ShellScript_ASPX.String():
		enPayload, err = payloads.EncryptForCSharp(payload, key)
		if err != nil {
			return nil, err
		}
	case ypb.ShellScript_PHP.String():
		fallthrough
	case ypb.ShellScript_ASP.String():
		enPayload = payloads.Xor(payload, key)
	}

	switch encMode {
	case ypb.EncMode_Base64.String():
		up := url.QueryEscape(base64.StdEncoding.EncodeToString(enPayload))
		enPayload = []byte(pass + "=" + up)
	case ypb.EncMode_Raw.String():

	}
	return enPayload, nil
}

func Decryption(raw, key []byte, pass, encMode, script string) ([]byte, error) {
	var dePayload []byte
	var err error
	switch encMode {
	case ypb.EncMode_Base64.String():
		flag := codec.Md5(pass + string(key))
		cont := regexp.MustCompile(`(?s)(?i)` + flag[0:16] + `(.*?)` + flag[16:]).FindStringSubmatch(string(raw))
		if len(cont) != 2 {
			return nil, utils.Errorf("not find string sub match %s", flag)
		}
		raw, err = base64.StdEncoding.DecodeString(cont[1])
		if err != nil {
			return nil, err
		}
	case ypb.EncMode_Raw.String():

	}
	switch script {
	case ypb.ShellScript_JSPX.String():
		fallthrough
	case ypb.ShellScript_JSP.String():
		dePayload, err = payloads.DecryptForJava(raw, key)
		if err != nil {
			return nil, err
		}
	case ypb.ShellScript_ASPX.String():
		dePayload, err = payloads.DecryptForCSharp(raw, key)
		if err != nil {
			return nil, err
		}
	case ypb.ShellScript_PHP.String():
		fallthrough
	case ypb.ShellScript_ASP.String():
		dePayload = payloads.Xor(raw, key)
	}

	if script != ypb.ShellScript_ASP.String() {
		dePayload, err = utils.GzipDeCompress(dePayload)
		if err != nil {
			return nil, err
		}
	}
	return dePayload, nil
}
