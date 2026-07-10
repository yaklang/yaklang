//go:build windows

package wintun

import (
	_ "embed"
	"runtime"

	"github.com/yaklang/yaklang/common/utils"
)

//go:embed wintun_amd64.dll.gz
var wintunDLLAMD64 []byte

//go:embed wintun_arm64.dll.gz
var wintunDLLARM64 []byte

func GetWintunDLLData() []byte {
	var compressed []byte
	switch runtime.GOARCH {
	case "arm64":
		compressed = wintunDLLARM64
	default:
		compressed = wintunDLLAMD64
	}
	buf, _ := utils.GzipDeCompress(compressed)
	return buf
}
