package ssaapi

import (
	"fmt"
	"testing"

	"github.com/gobwas/glob"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestExclude(t *testing.T) {
	check := func(program ssaapi.Programs, num int) {
		result, err := program.SyntaxFlowWithError(`println() as $param`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		values := result.GetValues("param")
		require.True(t, values.Len() == num)
	}
	fs := filesys.NewVirtualFs()
	fs.AddFile("/yaklang/common/yakgrpc/ypb/yakgrpc.pb.go", `package main

func main(){
	println(1);
}
`)
	fs.AddFile("/yaklang/common/yakgrpc/ypb/yakgrpc_grpc.pb.go", `package main

func main2(){
	println(2);
}
`)
	fs.AddFile("/yaklang/a.go", `package a

func main(){
	println(3);
}
`)
	prog, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaconfig.GO))
	require.NoError(t, err)
	prog.Show()
	check(prog, 3)
	gb, err := glob.Compile(`*.pb.go`)
	require.NoError(t, err)
	prog, err = ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaconfig.GO), ssaapi.WithExcludeFile(func(path, filename string) bool {
		a := filename
		_ = a
		return gb.Match(filename)
	}))
	require.NoError(t, err)
	prog.Show()
	check(prog, 1)
}

func TestFsString(t *testing.T) {
	vf := filesys.NewVirtualFs()
	// VirtualFS{src/main/go/A/test1.go}
	vf.AddFile("src/main/go/A/test1.go", `
package main

import (
    "html"
	"go-sec-code/utils"
	"html/template"
	"io/ioutil"

	beego "github.com/beego/beego/v2/server/web"
)

type XSSVuln1Controller struct {
	beego.Controller
}

func (c *XSSVuln1Controller) Get() {
	xss := c.GetString("xss", "hello")
	c.Ctx.ResponseWriter.Header().Set("Content-Type", "text/html")
	safeOutput := html.EscapeString(xss)
	c.Ctx.ResponseWriter.Write([]byte(safeOutput))
}
	`)

	str := fmt.Sprintf("%v", vf)
	require.Equal(t, str, "VirtualFS{src/main/go/A/test1.go}")
}

func TestWithExcludeFunc_GlobPatternCompilation(t *testing.T) {
	patterns := []string{
		"**/vendor/**",
		"vendor/**",
		"**/classes/**",
		"**/target/**",
		"**include/**",
		"**caches/**",
		"**cache/**",
		"**tmp/**",
		"**alipay/**",
		"**includes/**",
		"**temp/**",
		"**zh_cn/**",
		"**zh_en/**",
		"**plugins/**",
		"**PHPExcel/**",
		"*.pb.go",
		"*_test.go",
	}

	var failedPatterns []string
	var successPatterns []string

	for _, pattern := range patterns {
		_, err := glob.Compile(pattern)
		if err != nil {
			failedPatterns = append(failedPatterns, pattern)
			t.Logf("Pattern '%s' failed to compile: %v", pattern, err)
		} else {
			successPatterns = append(successPatterns, pattern)
		}
	}

	t.Logf("Successfully compiled patterns: %v", successPatterns)
	t.Logf("Failed to compile patterns: %v", failedPatterns)

	option := ssaapi.WithExcludeFunc(patterns)
	require.NotNil(t, option)

	config, err := ssaconfig.New(ssaconfig.ModeAll)
	require.NoError(t, err)
	require.NotNil(t, config)

	err = option(config)
	require.NoError(t, err)
}

func TestWithExcludeFunc_PartialFailure(t *testing.T) {
	validPatterns := []string{"*.pb.go", "*_test.go"}
	invalidPatterns := []string{"[invalid", "{unclosed"}

	allPatterns := append(validPatterns, invalidPatterns...)

	option := ssaapi.WithExcludeFunc(allPatterns)
	require.NotNil(t, option)

	config, err := ssaconfig.New(ssaconfig.ModeAll)
	require.NoError(t, err)
	require.NotNil(t, config)

	err = option(config)
	require.NoError(t, err)
}
