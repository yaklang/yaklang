package reactloops

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	aicommonmock "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/utils"
)

type registerTestLegionResultRuntime struct{ target string }

func (r registerTestLegionResultRuntime) AuthorizedTarget() string { return r.target }

func (registerTestLegionResultRuntime) Execute(string, map[string]any) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}

type registerTestFocusConfig struct {
	aicommon.AICallerConfigIf
	runtime aicommon.LegionResultRuntime
}

func (c *registerTestFocusConfig) GetLegionResultRuntime() aicommon.LegionResultRuntime { return c.runtime }

// 验证 RegisterYakFocusModeFromBundle 的 boot/run 双相行为：
//   - boot 期：从 yak 脚本中提取 metadata，注册到全局表
//   - run 期：能通过 GetLoopFactory 拿到 factory，能通过 GetLoopMetadata 拿到正确字段
//
// 关键词: yak focus mode register test, boot phase, run phase, metadata
func TestRegisterYakFocusMode_BootPhaseMetadata(t *testing.T) {
	name := "yakfm_test_boot_" + utils.RandStringBytes(6)
	code := `
__VERBOSE_NAME__    = "Boot Test"
__VERBOSE_NAME_ZH__ = "启动测试"
__DESCRIPTION__     = "boot phase metadata extraction test"
__DESCRIPTION_ZH__  = "启动期 metadata 提取测试"
__USAGE_PROMPT__    = "use this for boot test"
__OUTPUT_EXAMPLE__  = "example_output"
__IS_HIDDEN__       = true
`
	bundle := &FocusModeBundle{
		Name:      name,
		EntryFile: name + FocusModeFileSuffix,
		EntryCode: code,
	}
	err := RegisterYakFocusModeFromBundle(bundle)
	require.NoError(t, err)

	meta, ok := GetLoopMetadata(name)
	require.True(t, ok, "expect metadata registered for %s", name)
	require.Equal(t, "Boot Test", meta.VerboseName)
	require.Equal(t, "启动测试", meta.VerboseNameZh)
	require.Equal(t, "boot phase metadata extraction test", meta.Description)
	require.Equal(t, "启动期 metadata 提取测试", meta.DescriptionZh)
	require.Equal(t, "use this for boot test", meta.UsagePrompt)
	require.Equal(t, "example_output", meta.OutputExamplePrompt)
	require.True(t, meta.IsHidden)

	cached, ok := GetYakFocusModeBundle(name)
	require.True(t, ok)
	require.Equal(t, name, cached.Name)

	_, ok = GetLoopFactory(name)
	require.True(t, ok, "expect loop factory registered for %s", name)
}

// 验证 __NAME__ 显式指定时优先级高于文件 stem。
// 关键词: yak focus mode register test, explicit __NAME__ override
func TestRegisterYakFocusMode_ExplicitName(t *testing.T) {
	defaultName := "yakfm_default_" + utils.RandStringBytes(6)
	overrideName := "yakfm_override_" + utils.RandStringBytes(6)
	code := `
__NAME__ = "` + overrideName + `"
__VERBOSE_NAME__ = "Explicit Name Test"
`
	bundle := &FocusModeBundle{
		EntryFile: defaultName + FocusModeFileSuffix,
		EntryCode: code,
	}
	err := RegisterYakFocusModeFromBundle(bundle)
	require.NoError(t, err)

	_, found := GetLoopFactory(overrideName)
	require.True(t, found, "expect factory registered under %s", overrideName)

	_, foundDefault := GetLoopFactory(defaultName)
	require.False(t, foundDefault, "default name should not be used when __NAME__ is set")
}

func TestRegisterYakFocusMode_FixedNameRejectsScriptOverride(t *testing.T) {
	defaultName := "yakfm_fixed_" + utils.RandStringBytes(6)
	overrideName := "yakfm_override_" + utils.RandStringBytes(6)
	err := RegisterYakFocusModeFromBundle(&FocusModeBundle{
		Name:      defaultName,
		FixedName: true,
		EntryFile: defaultName + FocusModeFileSuffix,
		EntryCode: `__NAME__ = "` + overrideName + `"`,
	})
	require.Error(t, err)
	_, found := GetLoopFactory(defaultName)
	require.False(t, found)
	_, found = GetLoopFactory(overrideName)
	require.False(t, found)
}

// 验证同名重复注册失败，且失败时不会污染 yakFocusBundles。
// 关键词: yak focus mode register test, duplicate registration rejection
func TestRegisterYakFocusMode_DuplicateRejected(t *testing.T) {
	name := "yakfm_dup_" + utils.RandStringBytes(6)
	code := `__VERBOSE_NAME__ = "first"`

	err := RegisterYakFocusModeFromBundle(&FocusModeBundle{
		Name: name, EntryFile: name + FocusModeFileSuffix, EntryCode: code,
	})
	require.NoError(t, err)

	err = RegisterYakFocusModeFromBundle(&FocusModeBundle{
		Name: name, EntryFile: name + FocusModeFileSuffix, EntryCode: code,
	})
	require.Error(t, err, "expect error on duplicate registration")
}

// 验证 RegisterYakFocusMode 简化封装等价于带空 sidekick 的 bundle。
// 关键词: yak focus mode register test, simple wrapper
func TestRegisterYakFocusMode_SimpleWrapper(t *testing.T) {
	name := "yakfm_simple_" + utils.RandStringBytes(6)
	code := `
__VERBOSE_NAME__ = "Simple Wrapper"
__MAX_ITERATIONS__ = 5
`
	err := RegisterYakFocusMode(name, code)
	require.NoError(t, err)

	meta, ok := GetLoopMetadata(name)
	require.True(t, ok)
	require.Equal(t, "Simple Wrapper", meta.VerboseName)
}

// 验证 boot 期 yak 脚本编译失败时，错误能正确返回，且 bundle 不会进入缓存。
// 关键词: yak focus mode register test, boot eval failure, no leak
func TestRegisterYakFocusMode_BootFailureNoLeak(t *testing.T) {
	name := "yakfm_boot_fail_" + utils.RandStringBytes(6)
	bad := `
this is not valid yak code at all !!!
unmatched ( bracket
`
	err := RegisterYakFocusModeFromBundle(&FocusModeBundle{
		Name: name, EntryFile: name + FocusModeFileSuffix, EntryCode: bad,
	})
	require.Error(t, err)

	_, found := GetLoopFactory(name)
	require.False(t, found, "factory should not be registered when boot eval fails")

	_, found = GetYakFocusModeBundle(name)
	require.False(t, found, "bundle should not be cached when boot eval fails")
}

// 验证 sidekick 拼接后的 bundle 能完整从 boot 期解析 dunder。
// 关键词: yak focus mode register test, sidekick visible to main
func TestRegisterYakFocusMode_BundleWithSidekick(t *testing.T) {
	name := "yakfm_sidekick_" + utils.RandStringBytes(6)
	main := `
__VERBOSE_NAME__ = computeName()
`
	sidekick := `
computeName = func() {
    return "computed via sidekick"
}
`
	err := RegisterYakFocusModeFromBundle(&FocusModeBundle{
		Name:      name,
		EntryFile: name + FocusModeFileSuffix,
		EntryCode: main,
		Sidekicks: []FocusModeSidekick{
			{Path: "side.yak", Content: sidekick},
		},
		CallTimeout: 5 * time.Second,
	})
	require.NoError(t, err)

	meta, ok := GetLoopMetadata(name)
	require.True(t, ok)
	require.Equal(t, "computed via sidekick", meta.VerboseName)
}

func TestRegisterYakFocusMode_InjectsRunScopedLegionResultRuntime(t *testing.T) {
	code := `
readAuthorizedTarget = func() {
    return focusRuntime.AuthorizedTarget()
}
`
	baseConfig := aicommonmock.NewMockedAIConfig(context.Background())
	invoker := aicommonmock.NewMockInvoker(context.Background())
	invoker.SetConfig(&registerTestFocusConfig{
		AICallerConfigIf: baseConfig,
		runtime:          registerTestLegionResultRuntime{target: "https://example.test/authorized"},
	})
	caller, err := NewFocusModeYakHookCaller(
		"runtime.ai-focus.yak",
		code,
		focusModeRunCallerOptions(invoker, time.Second)...,
	)
	require.NoError(t, err)
	defer caller.Close()
	target, err := caller.CallByName("readAuthorizedTarget")
	require.NoError(t, err)
	require.Equal(t, "https://example.test/authorized", target)
}
