package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaproject"
)

// YakExports is the merged export map registered as the yaklang "ssa" module.
var YakExports = lo.Assign(Exports, ssaproject.Exports, ssaconfig.Exports)
