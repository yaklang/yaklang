package tests

import (
	_ "embed"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/typescript/js2ssa"
	"testing"
	"time"
)

//go:embed test.js
var largeJS string

func TestJS_ASTLargeText(t *testing.T) {
	start := time.Now()

	log.Infof("start to build ast via parser")
	_, err := js2ssa.Frontend(largeJS, false)
	require.Nil(t, err)
	log.Infof("finish to build ast via parser cost: %v", time.Now().Sub(start))

	start = time.Now()
	prog, err := ssaapi.Parse(largeJS,
		ssaapi.WithLanguage("js"),
	)
	require.NoError(t, err)

	// 生成函数的控制流图
	dot := ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0])
	log.Infof("finish parse+ast2ssa cost: %v", time.Now().Sub(start))
	log.Infof("函数控制流图DOT: \n%s", dot)
}
