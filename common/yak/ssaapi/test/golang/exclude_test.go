package ssaapi

import (
	"github.com/gobwas/glob"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
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
	prog, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaapi.GO))
	require.NoError(t, err)
	prog.Show()
	check(prog, 3)
	gb, err := glob.Compile(`*.pb.go`)
	require.NoError(t, err)
	prog, err = ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaapi.GO), ssaapi.WithExcludeFile(func(path, filename string) bool {
		a := filename
		_ = a
		return gb.Match(filename)
	}))
	require.NoError(t, err)
	prog.Show()
	check(prog, 1)
}
