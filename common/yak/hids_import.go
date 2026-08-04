//go:build hids

package yak

import (
	"github.com/yaklang/yaklang/common/hids"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func initHIDSLib() {
	yaklang.Import("hids", hids.Exports)
}
