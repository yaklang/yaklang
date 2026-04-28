package reactloops

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// 测试帮手：构造一个最小的 ReActLoop，用于把 ReActLoopOption 应用到上面、再校验状态。
// 仅初始化 actions / streamFields / aiTagFields / vars / loopActions 字段，
// 这是 buildActionOptionFromDict 等代码路径会触碰到的全部字段。
//
// 关键词: minimal ReActLoop for option apply test
func newMinimalReActLoopForOptionTest() *ReActLoop {
	return &ReActLoop{
		actions:      omap.NewEmptyOrderedMap[string, *LoopAction](),
		loopActions:  omap.NewEmptyOrderedMap[string, LoopActionFactory](),
		streamFields: omap.NewEmptyOrderedMap[string, *LoopStreamField](),
		aiTagFields:  omap.NewEmptyOrderedMap[string, *LoopAITagField](),
		vars:         omap.NewEmptyOrderedMap[string, any](),
	}
}

// ParseFocusModeActionOptions：基本类型解析覆盖。
// 关键词: yak focus mode action options parsing test
func TestParseFocusModeActionOptions_Basic(t *testing.T) {
	items := []any{
		map[string]any{
			"name": "target", "type": "string",
			"description": "target host",
			"required":    true,
		},
		map[string]any{
			"name": "port", "type": "integer",
			"description": "port number", "min": 1, "max": 65535,
		},
		map[string]any{
			"name": "fast", "type": "bool",
			"description": "fast mode",
		},
		map[string]any{
			"name": "tags", "type": "string_array",
			"description": "tag list",
		},
	}

	opts := ParseFocusModeActionOptions(items)
	require.Len(t, opts, 4)

	schema := renderToolOptionsAsObjectSchema(opts)
	require.Contains(t, schema, "\"target\"")
	require.Contains(t, schema, "\"port\"")
	require.Contains(t, schema, "\"fast\"")
	require.Contains(t, schema, "\"tags\"")
}

// 缺失 name 的项被跳过。
// 关键词: yak focus mode action options skip missing name
func TestParseFocusModeActionOptions_SkipMissingName(t *testing.T) {
	items := []any{
		map[string]any{"type": "string"},
		map[string]any{"name": "ok", "type": "string"},
	}
	opts := ParseFocusModeActionOptions(items)
	require.Len(t, opts, 1)
}

// enum / pattern / 长度约束在 string 类型下生效。
// 关键词: yak focus mode action options string constraints
func TestParseFocusModeActionOptions_StringConstraints(t *testing.T) {
	items := []any{
		map[string]any{
			"name": "color", "type": "string",
			"description": "favorite color",
			"enum":        []any{"red", "blue", "green"},
			"pattern":     "^[a-z]+$",
			"max_length":  10,
			"min_length":  3,
		},
	}
	opts := ParseFocusModeActionOptions(items)
	require.Len(t, opts, 1)

	schema := renderToolOptionsAsObjectSchema(opts)
	require.Contains(t, schema, "\"color\"")
	require.True(t,
		strings.Contains(schema, "red") &&
			strings.Contains(schema, "blue") &&
			strings.Contains(schema, "green"),
		"enum strings should appear in schema: %s", schema)
}

// 未知 type 时回退为 string，不会丢字段。
// 关键词: yak focus mode action options unknown type fallback
func TestParseFocusModeActionOptions_UnknownTypeFallback(t *testing.T) {
	items := []any{
		map[string]any{"name": "weird", "type": "weird_type", "description": "x"},
	}
	opts := ParseFocusModeActionOptions(items)
	require.Len(t, opts, 1)

	schema := renderToolOptionsAsObjectSchema(opts)
	require.Contains(t, schema, "\"weird\"")
}

// renderToolOptionsAsObjectSchema 把 []aitool.ToolOption 转 []any 后渲染 schema，
// 仅在测试中使用。
//
// 关键词: tool options to schema for test
func renderToolOptionsAsObjectSchema(opts []aitool.ToolOption) string {
	anyOpts := make([]any, 0, len(opts))
	for _, o := range opts {
		anyOpts = append(anyOpts, o)
	}
	return aitool.NewObjectSchema(anyOpts...)
}

// CollectFocusModeActionOptions：__ACTIONS__ 注册一条普通 action，
// 应用到 fresh ReActLoop 后 actions 表里能查到，且 verifier 能调用。
//
// 关键词: yak focus mode collect action options register
func TestCollectFocusModeActionOptions_RegisterAction(t *testing.T) {
	code := `
__ACTIONS__ = [
    {
        "type": "scan_target",
        "description": "scan a target host",
        "options": [
            {"name": "target", "type": "string", "required": true, "description": "target"},
        ],
        "stream_fields": [
            {"field": "summary", "node_id": "scan-summary"},
        ],
        "verifier": func(loop, action) {
            target = action.GetString("target")
            if target == "" {
                return "target required"
            }
            return nil
        },
        "handler": func(loop, action, operator) {
            operator.Feedback("scanned " + action.GetString("target"))
        },
    },
]
`
	caller, err := NewFocusModeYakHookCaller("scan.ai-focus.yak", code)
	require.NoError(t, err)
	defer caller.Close()

	opts := CollectFocusModeActionOptions(caller, nil)
	require.NotEmpty(t, opts)

	loop := newMinimalReActLoopForOptionTest()
	for _, opt := range opts {
		opt(loop)
	}

	action, ok := loop.actions.Get("scan_target")
	require.True(t, ok, "scan_target should be registered")
	require.Equal(t, "scan a target host", action.Description)
	require.NotNil(t, action.ActionVerifier)
	require.NotNil(t, action.ActionHandler)
	require.Len(t, action.Options, 1)
	require.Len(t, action.StreamFields, 1)
	require.Equal(t, "summary", action.StreamFields[0].FieldName)

	// verifier 真实调用
	verifyAction := aicommon.NewSimpleAction("scan_target", aitool.InvokeParams{"target": "example.com"})
	require.NoError(t, action.ActionVerifier(nil, verifyAction))

	missing := aicommon.NewSimpleAction("scan_target", aitool.InvokeParams{"target": ""})
	err = action.ActionVerifier(nil, missing)
	require.Error(t, err)
	require.Contains(t, err.Error(), "target required")
}

// __ACTIONS__ 项缺少 handler 时整条被跳过。
// 关键词: yak focus mode action missing handler skip
func TestCollectFocusModeActionOptions_MissingHandlerSkipped(t *testing.T) {
	code := `
__ACTIONS__ = [
    {
        "type": "no_handler",
        "description": "should be skipped",
        "verifier": func(loop, action) { return nil },
    },
]
`
	caller, err := NewFocusModeYakHookCaller("nh.ai-focus.yak", code)
	require.NoError(t, err)
	defer caller.Close()

	opts := CollectFocusModeActionOptions(caller, nil)
	loop := newMinimalReActLoopForOptionTest()
	for _, opt := range opts {
		opt(loop)
	}
	_, ok := loop.actions.Get("no_handler")
	require.False(t, ok, "action without handler should be skipped")
}

// __ACTIONS__ 项 async 与 output_examples 字段被回填。
// 关键词: yak focus mode action async patch
func TestCollectFocusModeActionOptions_AsyncAndExamples(t *testing.T) {
	code := `
__ACTIONS__ = [
    {
        "type": "long_task",
        "description": "long running task",
        "async": true,
        "output_examples": "example output rendered here",
        "handler": func(loop, action, operator) {
            operator.Feedback("done")
        },
    },
]
`
	caller, err := NewFocusModeYakHookCaller("async.ai-focus.yak", code)
	require.NoError(t, err)
	defer caller.Close()

	opts := CollectFocusModeActionOptions(caller, nil)
	loop := newMinimalReActLoopForOptionTest()
	for _, opt := range opts {
		opt(loop)
	}
	action, ok := loop.actions.Get("long_task")
	require.True(t, ok)
	require.True(t, action.AsyncMode)
	require.Equal(t, "example output rendered here", action.OutputExamples)
}

// __OVERRIDE_ACTIONS__: 替换内置同名 action。
// 关键词: yak focus mode override action
func TestCollectFocusModeActionOptions_OverrideAction(t *testing.T) {
	code := `
__OVERRIDE_ACTIONS__ = [
    {
        "type": "directly_answer",
        "description": "custom directly_answer",
        "verifier": func(loop, action) { return nil },
        "handler": func(loop, action, operator) {
            operator.Feedback("custom answer")
        },
    },
]
`
	caller, err := NewFocusModeYakHookCaller("override.ai-focus.yak", code)
	require.NoError(t, err)
	defer caller.Close()

	opts := CollectFocusModeActionOptions(caller, nil)
	loop := newMinimalReActLoopForOptionTest()
	loop.actions.Set("directly_answer", &LoopAction{
		ActionType:  "directly_answer",
		Description: "builtin",
	})
	for _, opt := range opts {
		opt(loop)
	}
	action, ok := loop.actions.Get("directly_answer")
	require.True(t, ok)
	require.Equal(t, "custom directly_answer", action.Description, "should be replaced by override")
	require.NotNil(t, action.ActionHandler)
}

// __ACTIONS_FROM_TOOLS__: 通过 toolLookup 解析工具名。
// 关键词: yak focus mode actions from tools
func TestCollectFocusModeActionOptions_ActionsFromTools(t *testing.T) {
	code := `
__ACTIONS_FROM_TOOLS__ = ["greet", "ping"]
`
	caller, err := NewFocusModeYakHookCaller("fromtools.ai-focus.yak", code)
	require.NoError(t, err)
	defer caller.Close()

	noopCallback := func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
		return nil, nil
	}
	greetTool, err := aitool.New("greet",
		aitool.WithDescription("say hello"),
		aitool.WithStringParam("name"),
		aitool.WithSimpleCallback(noopCallback),
	)
	require.NoError(t, err)

	pingTool, err := aitool.New("ping",
		aitool.WithDescription("ping host"),
		aitool.WithStringParam("host"),
		aitool.WithSimpleCallback(noopCallback),
	)
	require.NoError(t, err)

	tools := map[string]*aitool.Tool{
		"greet": greetTool,
		"ping":  pingTool,
	}
	lookup := func(name string) *aitool.Tool { return tools[name] }

	opts := CollectFocusModeActionOptions(caller, lookup)
	require.Len(t, opts, 2)

	loop := newMinimalReActLoopForOptionTest()
	for _, opt := range opts {
		opt(loop)
	}
	_, ok := loop.actions.Get("greet")
	require.True(t, ok)
	_, ok = loop.actions.Get("ping")
	require.True(t, ok)
}

// __ACTIONS_FROM_TOOLS__: lookup 返回 nil 时跳过该项，不报错。
// 关键词: yak focus mode actions from tools missing lookup
func TestCollectFocusModeActionOptions_ActionsFromToolsMissingLookup(t *testing.T) {
	code := `
__ACTIONS_FROM_TOOLS__ = ["does_not_exist"]
`
	caller, err := NewFocusModeYakHookCaller("missing.ai-focus.yak", code)
	require.NoError(t, err)
	defer caller.Close()

	opts := CollectFocusModeActionOptions(caller, func(name string) *aitool.Tool { return nil })
	require.Empty(t, opts)
}

// __ACTIONS_FROM_LOOPS__: 把已注册 loop 包装成 sub-action factory。
// 关键词: yak focus mode actions from loops
func TestCollectFocusModeActionOptions_ActionsFromLoops(t *testing.T) {
	subName := "yakfm_subloop_" + utils.RandStringBytes(6)
	require.NoError(t, RegisterYakFocusMode(subName, `__VERBOSE_NAME__ = "Sub Loop"`))

	code := `
__ACTIONS_FROM_LOOPS__ = ["` + subName + `"]
`
	caller, err := NewFocusModeYakHookCaller("subloop.ai-focus.yak", code)
	require.NoError(t, err)
	defer caller.Close()

	opts := CollectFocusModeActionOptions(caller, nil)
	require.Len(t, opts, 1)

	loop := newMinimalReActLoopForOptionTest()
	for _, opt := range opts {
		opt(loop)
	}
	_, ok := loop.loopActions.Get(subName)
	require.True(t, ok)
}

// 当 caller == nil 时返回 nil，不 panic。
// 关键词: yak focus mode actions nil safety
func TestCollectFocusModeActionOptions_NilSafety(t *testing.T) {
	require.Nil(t, CollectFocusModeActionOptions(nil, nil))
}

// handler 抛 panic 时被 caller 捕获并以 operator.Fail 上报。
// 关键词: yak focus mode handler panic recovered
func TestCollectFocusModeActionOptions_HandlerPanic(t *testing.T) {
	code := `
__ACTIONS__ = [
    {
        "type": "boom",
        "description": "panicking handler",
        "verifier": func(loop, action) { return nil },
        "handler": func(loop, action, operator) {
            panic("boom from yak")
        },
    },
]
`
	caller, err := NewFocusModeYakHookCaller("boom.ai-focus.yak", code)
	require.NoError(t, err)
	defer caller.Close()

	opts := CollectFocusModeActionOptions(caller, nil)
	loop := newMinimalReActLoopForOptionTest()
	for _, opt := range opts {
		opt(loop)
	}
	action, ok := loop.actions.Get("boom")
	require.True(t, ok)

	op := NewActionHandlerOperator(nil)
	require.NotPanics(t, func() {
		action.ActionHandler(nil, aicommon.NewSimpleAction("boom", aitool.InvokeParams{}), op)
	})
	terminated, errVal := op.IsTerminated()
	require.True(t, terminated, "handler panic should terminate via Fail")
	require.Error(t, errVal)
}
