package loop_yaklangcode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyYakScriptRunPolicy_HookMissingSelfTest(t *testing.T) {
	code := `mirrorNewWebsitePath = func(isHttps, url, req, rsp, body) { PATHS[url] = true }
PATHS = {}`
	p := ClassifyYakScriptRunPolicy(code)
	assert.Equal(t, YakScriptKindHookHotpatch, p.Kind)
	assert.True(t, p.BlockExitNoSelfTest)
	assert.False(t, p.ShouldExecuteRun)
}

func TestClassifyYakScriptRunPolicy_HookWithSelfTest(t *testing.T) {
	code := `mirrorNewWebsitePath = func(isHttps, url, req, rsp, body) {}
func runSelfTest() { mirrorNewWebsitePath(false, "http://t", []byte(""), []byte(""), []byte("")) }
if YAK_MAIN { runSelfTest() }`
	p := ClassifyYakScriptRunPolicy(code)
	assert.Equal(t, YakScriptKindHookHotpatch, p.Kind)
	assert.True(t, p.ShouldExecuteRun)
	assert.False(t, p.BlockExitNoSelfTest)
}

func TestClassifyYakScriptRunPolicy_CodecPlugin(t *testing.T) {
	code := `handle = func(input) { return input }
if YAK_MAIN { runSelfTest() }
func runSelfTest() { assert handle("x") == "x" }`
	p := ClassifyYakScriptRunPolicy(code)
	assert.Equal(t, YakScriptKindCodecPlugin, p.Kind)
	assert.True(t, p.ShouldExecuteRun)
}

func TestClassifyYakScriptRunPolicy_CLIToolSkipsLiveRun(t *testing.T) {
	code := `target = cli.String("target", cli.setRequired(true))
cli.check()
servicescan.Scan(target, "80")`
	p := ClassifyYakScriptRunPolicy(code)
	assert.Equal(t, YakScriptKindCLITool, p.Kind)
	assert.False(t, p.ShouldExecuteRun)
	assert.False(t, p.BlockExitNoSelfTest)
}

func TestClassifyYakScriptRunPolicy_CLIToolWithLogicBlocksWithoutSelfTest(t *testing.T) {
	code := `target = cli.String("target", cli.setRequired(true))
loadAndExec = func(path) { dyn.Eval(file.ReadAll(path)~) }
cli.check()
for { loadAndExec(target) }`
	p := ClassifyYakScriptRunPolicy(code)
	assert.Equal(t, YakScriptKindCLITool, p.Kind)
	assert.True(t, p.BlockExitNoSelfTest)
}

func TestClassifyYakScriptRunPolicy_NativePluginMissingGuard(t *testing.T) {
	code := `func runPlugin() { cli.check() }
func runSelfTest() { assert true }`
	p := ClassifyYakScriptRunPolicy(code)
	assert.Equal(t, YakScriptKindNativePlugin, p.Kind)
	assert.True(t, p.BlockExitNoSelfTest)

	p2 := ClassifyYakScriptRunPolicy(`func runSelfTest() { assert 1==1 }`)
	assert.Equal(t, YakScriptKindPureLogicScript, p2.Kind)
	assert.True(t, p2.BlockExitNoSelfTest)
}

func TestShouldAutoRunYakSelfTest_Basic(t *testing.T) {
	assert.False(t, ShouldAutoRunYakSelfTest("yakit.AutoInitYakit()\ncli.check()"))
	assert.True(t, ShouldAutoRunYakSelfTest(`mirrorX = func() {}
func runSelfTest() {}
if YAK_MAIN { runSelfTest() }`))
}
