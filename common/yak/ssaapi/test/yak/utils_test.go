package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	ssatest.SetLanguage(ssaconfig.Yak)
}
