package reactloops

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 测试目标: __SCENARIO_TOOLS__ dunder 能被 CollectFocusModeStaticOptions 解析,
// 转化为 WithScenarioToolWhitelist 选项, 应用到 ReActLoop 后 scenarioToolWhitelist
// 字段能正确拿到, GetScenarioToolWhitelist() 返回相同值.
//
// 同时覆盖三种声明形式 (slice / 单字符串 / 逗号分隔字符串) 与 default 行为
// (没有声明时, 应用后 whitelist 为 nil), 确保 yak 作者用任意一种都能落到
// 一致的 Go 内部状态.
//
// 关键词: __SCENARIO_TOOLS__ dunder test, scenario whitelist parsing,
//        WithScenarioToolWhitelist, hidden tool pattern, CollectFocusModeStaticOptions

// TestScenarioToolsDunder_Slice yak 声明为字符串切片时的标准用法.
// 关键词: __SCENARIO_TOOLS__ slice, common case
func TestScenarioToolsDunder_Slice(t *testing.T) {
	code := `
__VERBOSE_NAME__ = "ScenarioToolsSlice"
__SCENARIO_TOOLS__ = ["ssa-grep", "ssa-read-file", "  ", ""]
`
	caller, err := NewFocusModeYakHookCaller(
		"scenario_tools_slice.ai-focus.yak", code,
		WithFocusModeCallerCallTimeout(2*time.Second),
	)
	require.NoError(t, err)
	defer caller.Close()

	loop := newMinimalReActLoopForOptionTest()
	for _, opt := range CollectFocusModeStaticOptions(caller) {
		opt(loop)
	}

	require.Equal(t,
		[]string{"ssa-grep", "ssa-read-file"},
		loop.GetScenarioToolWhitelist(),
		"slice dunder should keep order and drop empty entries",
	)
}

// TestScenarioToolsDunder_CommaSeparated yak 声明为逗号分隔字符串时的兜底用法.
// 关键词: __SCENARIO_TOOLS__ comma string, fallback form
func TestScenarioToolsDunder_CommaSeparated(t *testing.T) {
	code := `
__VERBOSE_NAME__ = "ScenarioToolsComma"
__SCENARIO_TOOLS__ = "ssa-grep, ssa-list-files , "
`
	caller, err := NewFocusModeYakHookCaller(
		"scenario_tools_comma.ai-focus.yak", code,
		WithFocusModeCallerCallTimeout(2*time.Second),
	)
	require.NoError(t, err)
	defer caller.Close()

	loop := newMinimalReActLoopForOptionTest()
	for _, opt := range CollectFocusModeStaticOptions(caller) {
		opt(loop)
	}

	require.Equal(t,
		[]string{"ssa-grep", "ssa-list-files"},
		loop.GetScenarioToolWhitelist(),
		"comma string dunder should trim and drop empties",
	)
}

// TestScenarioToolsDunder_SingleString 仅一个单工具名时, 行为应等价于
// 单元素 slice. 顺便覆盖 GetScenarioToolWhitelist 在 receiver 非 nil 时
// 返回精确值.
// 关键词: __SCENARIO_TOOLS__ single string
func TestScenarioToolsDunder_SingleString(t *testing.T) {
	code := `
__VERBOSE_NAME__ = "ScenarioToolsSingle"
__SCENARIO_TOOLS__ = "ssa-grep"
`
	caller, err := NewFocusModeYakHookCaller(
		"scenario_tools_single.ai-focus.yak", code,
		WithFocusModeCallerCallTimeout(2*time.Second),
	)
	require.NoError(t, err)
	defer caller.Close()

	loop := newMinimalReActLoopForOptionTest()
	for _, opt := range CollectFocusModeStaticOptions(caller) {
		opt(loop)
	}

	require.Equal(t,
		[]string{"ssa-grep"},
		loop.GetScenarioToolWhitelist(),
		"single string should behave as single-element whitelist",
	)
}

// TestScenarioToolsDunder_Absent 没声明 dunder 时, 应用后 whitelist 必须保持
// nil/empty, 不能莫名拉回 scenario 工具. 这是默认 inventory 不出现 ssa-*
// 的关键保证.
// 关键词: __SCENARIO_TOOLS__ absent, default no whitelist
func TestScenarioToolsDunder_Absent(t *testing.T) {
	code := `
__VERBOSE_NAME__ = "ScenarioToolsAbsent"
__MAX_ITERATIONS__ = 3
`
	caller, err := NewFocusModeYakHookCaller(
		"scenario_tools_absent.ai-focus.yak", code,
		WithFocusModeCallerCallTimeout(2*time.Second),
	)
	require.NoError(t, err)
	defer caller.Close()

	loop := newMinimalReActLoopForOptionTest()
	for _, opt := range CollectFocusModeStaticOptions(caller) {
		opt(loop)
	}

	require.Empty(t, loop.GetScenarioToolWhitelist(),
		"absent dunder should NOT populate the whitelist")
}

// TestScenarioToolsDunder_NilLoopGetterSafe getter 在 receiver 为 nil 时
// 必须返回 nil, 给 GetLoopPromptBaseMaterials 链式调用兜底.
// 关键词: GetScenarioToolWhitelist nil-safe
func TestScenarioToolsDunder_NilLoopGetterSafe(t *testing.T) {
	var loop *ReActLoop
	require.Nil(t, loop.GetScenarioToolWhitelist())
}
