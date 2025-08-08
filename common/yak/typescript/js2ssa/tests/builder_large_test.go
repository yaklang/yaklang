package tests

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/typescript/js2ssa"
)

//go:embed test.js
var largeJS string

//go:embed member_nil.js
var nilMemberJS string

func TestJS_ASTLargeText(t *testing.T) {
	t.Skip("skip large js test, it is too slow to run in CI")

	start := time.Now()

	log.Infof("start to build ast via parser")
	_, err := js2ssa.Frontend(largeJS)
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

func TestJS_Nil_Member(t *testing.T) {
	prog, err := ssaapi.Parse(nilMemberJS,
		ssaapi.WithLanguage("js"),
	)
	require.NoError(t, err)
	prog.Show()
	prog.Program.EachFunction(func(function *ssa.Function) {
		dot := ssaapi.FunctionDotGraph(function)
		log.Infof("函数控制流图DOT: \n%s", dot)
	})
	result, err := prog.SyntaxFlowWithError(`
    .ajax(* #-> as $ajax_info)
    .open(* as $openParams)
    $openParams<slice(start=0)> #-> as $xml_http_method
    $openParams<slice(start=1)> #-> as $xml_http_url

    /(?i)([axios\.](get)|(post)|(patch)|(delete)|(put)|)/(* as $axiosparams)
    $axiosparams<slice(start=0)> #-> as $ajax_get_url
    $axiosparams<slice(start=1)> #-> *?{!opcode: call,function}<getMembers> as $dollar_get_member
    axios(* #-> *<getMembers> as $_axios_info)
    fetch(* #-> * as $fetch_url)
    `)
	require.NoError(t, err)
	t.Log(result.GetValues("ajax_info"))
	t.Log(result.GetValues("xml_http_url"))
	t.Log(result.GetValues("ajax_get_url"))
	t.Log(result.GetValues("_axios_info"))
	t.Log(result.GetValues("fetch_url"))
}
