package syntaxflow

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
)

// YakExports is the merged export map registered as the yaklang "syntaxflow" module.
var YakExports = lo.Assign(Exports, syntaxflow_scan.Exports)
