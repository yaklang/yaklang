package yaklib

import "github.com/yaklang/yaklang/common/utils/regen"

var RegenExports = map[string]interface{}{
	"Generate":               regen.Generate,
	"GenerateOne":            regen.GenerateOne,
	"GenerateVisibleOne":     regen.GenerateVisibleOne,
	"MustGenerate":           regen.MustGenerate,
	"MustGenerateOne":        regen.MustGenerateOne,
	"MustGenerateVisibleOne": regen.MustGenerateVisibleOne,
}
