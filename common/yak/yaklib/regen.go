package yaklib

import "github.com/yaklang/yaklang/common/utils/regen"

var RegenExports = map[string]interface{}{
	"Generate":     regen.Generate,
	"MustGenerate": regen.MustGenerate,
}
