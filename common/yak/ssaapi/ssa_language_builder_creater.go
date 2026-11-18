//go:build !irify_exclude

package ssaapi

import (
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/c2ssa"
	"github.com/yaklang/yaklang/common/yak/python/python2ssa"
	"github.com/yaklang/yaklang/common/yak/typescript/ts2ssa"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"

	//js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/bash/bash2ssa"
	cpp2ssa "github.com/yaklang/yaklang/common/yak/cpp/cpp2ssa"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/kotlin/kotlin2ssa"
	"github.com/yaklang/yaklang/common/yak/lua/lua2ssa"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/rust/rust2ssa"
	"github.com/yaklang/yaklang/common/yak/scala/scala2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var LanguageBuilderCreater = map[ssaconfig.Language]ssa.CreateBuilder{
	ssaconfig.Yak:    yak2ssa.CreateBuilder,
	ssaconfig.JS:     ts2ssa.CreateBuilder,
	ssaconfig.PHP:    php2ssa.CreateBuilder,
	ssaconfig.JAVA:   java2ssa.CreateBuilder,
	ssaconfig.GO:     go2ssa.CreateBuilder,
	ssaconfig.C:      c2ssa.CreateBuilder,
	ssaconfig.CPP:    cpp2ssa.CreateBuilder,
	ssaconfig.TS:     ts2ssa.CreateBuilder,
	ssaconfig.PYTHON: python2ssa.CreateBuilder,
	ssaconfig.CSHARP: csharp2ssa.CreateBuilder,
	ssaconfig.BASH:   bash2ssa.CreateBuilder,
	ssaconfig.LUA:    lua2ssa.CreateBuilder,
	ssaconfig.RUST:   rust2ssa.CreateBuilder,
	ssaconfig.KOTLIN: kotlin2ssa.CreateBuilder,
	ssaconfig.SCALA:  scala2ssa.CreateBuilder,
}

// ArchiveCodeSourceOpener opens a code source that is itself an archive.
type ArchiveCodeSourceOpener func(local string, recursive bool) (fi.FileSystem, error)

// archiveCodeSourceOpeners lets a language open its own archive. A jar is one
// opaque file to a walk, and only the language that produced it knows how its
// classes become readable source; a language without archives is absent here.
var archiveCodeSourceOpeners = map[ssaconfig.Language]ArchiveCodeSourceOpener{
	ssaconfig.JAVA: java2ssa.OpenArchiveCodeSource,
}
