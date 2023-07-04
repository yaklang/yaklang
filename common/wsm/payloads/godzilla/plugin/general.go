package plugin

import "github.com/yaklang/yaklang/common/yak/yaklib/codec"

func GetScanWebshellByteCode() []byte {
	code, _ := codec.DecodeBase64("")
	return code
}
