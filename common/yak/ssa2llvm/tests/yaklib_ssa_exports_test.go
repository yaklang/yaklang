package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYaklibSSA_NewConfigMultiReturn(t *testing.T) {
	code := `
yakit.AutoInitYakit()
config, err = ssa.NewConfig(ssa.ModeAll, ssa.withProgramName("probe"), ssa.withLanguage("php"))
if err != nil { die("err: %v", err) }
if config == nil { die("config nil") }
jsonStr, err2 = config.ToJSONString()
if err2 != nil { die("json err: %v", err2) }
if len(jsonStr) == 0 { die("empty json") }
println("ok")
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, strings.TrimSpace(output), "ok")
}

func TestYaklibSSA_ModeAllConstant(t *testing.T) {
	code := `
yakit.AutoInitYakit()
if ssa.ModeAll == 0 { die("ModeAll is zero") }
println(ssa.ModeAll)
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.NotContains(t, output, "ModeAll is zero")
	require.Contains(t, output, "127")
}

func TestYaklibSSA_MultiReturnThreeValues(t *testing.T) {
	code := `
yakit.AutoInitYakit()
f = func() { return 1, 2, 3 }
a, b, c = f()
if a != 1 || b != 2 || c != 3 { die("tuple unpack failed: %v %v %v", a, b, c) }
println("ok")
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, strings.TrimSpace(output), "ok")
}

func TestYaklibSSA_MultiReturnWithError(t *testing.T) {
	code := `
yakit.AutoInitYakit()
f = func() { return "data", nil }
a, err = f()
if a != "data" { die("bad data: %v", a) }
if err != nil { die("expected nil err, got %v", err) }
println("ok")
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, strings.TrimSpace(output), "ok")
}

func TestYaklibSSA_WithExcludeFileEmptySlice(t *testing.T) {
	code := `
yakit.AutoInitYakit()
config, err = ssa.NewConfig(ssa.ModeAll, ssa.withProgramName("t"), ssa.withLanguage("php"), ssa.withExcludeFile([]))
if err != nil { die("err: %v", err) }
if config == nil { die("config nil") }
println("ok")
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, strings.TrimSpace(output), "ok")
}

func TestYaklibSSA_SyncWaitGroupRegression(t *testing.T) {
	code := `
yakit.AutoInitYakit()
wg = sync.NewWaitGroup()
if wg == nil { die("waitgroup nil") }
println("ok")
`
	output := runBinaryWithEnv(t, code, "", nil)
	require.Contains(t, strings.TrimSpace(output), "ok")
}
