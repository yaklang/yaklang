package yaklib

import "github.com/yaklang/yaklang/common/utils/regen"

var RegenExports = map[string]interface{}{
	"Generate":                 regen.Generate,
	"GenerateStream":           regen.GenerateStream,
	"GenerateOne":              regen.GenerateOne,
	"GenerateOneStream":        regen.GenerateOneStream,
	"GenerateVisibleOne":       regen.GenerateVisibleOne,
	"GenerateVisibleOneStream": regen.GenerateVisibleOneStream,
	"MustGenerate":             regen.MustGenerate,
	"MustGenerateOne":          regen.MustGenerateOne,
	"MustGenerateVisibleOne":   regen.MustGenerateVisibleOne,
}
