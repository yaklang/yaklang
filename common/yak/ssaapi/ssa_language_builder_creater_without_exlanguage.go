//go:build irify_exclude

package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/yak2ssa"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// LanguageBuilderCreater keeps Yak SSA for script completion and static checks.
// irify_exclude removes the other language builders and their compiler frontends;
// it does not remove all SSA/SyntaxFlow internals used by shared packages.
// Use gzip_embed,irify_exclude for the release slim build. Reproduce size and
// dependency measurements with scripts/binary-size; do not infer memory or
// startup savings from the executable's file size.
var LanguageBuilderCreater = map[ssaconfig.Language]ssa.CreateBuilder{
	ssaconfig.Yak: yak2ssa.CreateBuilder,
}
