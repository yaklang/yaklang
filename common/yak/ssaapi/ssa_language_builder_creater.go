//go:build !without_exlanguage

package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/c2ssa"
	"github.com/yaklang/yaklang/common/yak/typescript/ts2ssa"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"

	//js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var LanguageBuilderCreater = map[ssaconfig.Language]ssa.CreateBuilder{
	ssaconfig.Yak:  yak2ssa.CreateBuilder,
	ssaconfig.JS:   ts2ssa.CreateBuilder,
	ssaconfig.PHP:  php2ssa.CreateBuilder,
	ssaconfig.JAVA: java2ssa.CreateBuilder,
	ssaconfig.GO:   go2ssa.CreateBuilder,
	ssaconfig.C:    c2ssa.CreateBuilder,
	ssaconfig.TS:   ts2ssa.CreateBuilder,
}
