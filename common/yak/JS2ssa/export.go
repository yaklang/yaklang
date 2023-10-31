package js2ssa

import "github.com/yaklang/yaklang/common/yak/ssa"

var Exports = map[string]any{
	"ParseSSA": 		func(src string, opt ...Option) *ssa.Program { 
		return ParseSSA(src, opt...) },
	"WithExternValue":	WithExternValue,
	"WithExternLib":	WithExternLib,	
	"WithTypeMethod":	WithTypeMethod,
}