//go:build windows

package wintun

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/utils"
)

//go:embed wintun.dll.gz
var wintunDLL []byte

func GetWintunDLLData() []byte {
	buf, _ := utils.GzipDeCompress(wintunDLL)
	return buf
}
