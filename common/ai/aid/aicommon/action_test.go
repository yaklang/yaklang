package aicommon

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestSupperAction_SetAndGet(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(3)
	action := &Action{
		name:            "test_action",
		params:          make(aitool.InvokeParams),
		barrier:         utils.NewCondBarrierContext(ctx),
		generalParamKey: "whole",
	}
	action.Set("intKey", 42)
	action.Set("strKey", "hello")
	action.Set("floatKey", 3.14)
	action.Set("boolKey", true)
	action.Set("sliceKey", []string{"a", "b"})

	if got := action.GetInt("intKey"); got != 42 {
		t.Errorf("expected 42, got %v", got)
	}
	if got := action.GetString("strKey"); got != "hello" {
		t.Errorf("expected 'hello', got %v", got)
	}
	if got := action.GetFloat("floatKey"); got != 3.14 {
		t.Errorf("expected 3.14, got %v", got)
	}
	if got := action.GetBool("boolKey"); got != true {
		t.Errorf("expected true, got %v", got)
	}
	if got := action.GetStringSlice("sliceKey"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("expected slice ['a','b'], got %v", got)
	}
}

func TestSupperActionMaker_ReadFromReader(t *testing.T) {
	maker := &ActionMaker{
		actionName: "my_action",
		tagToKey:   map[string]string{"FINAL_ANSWER": "answer"},
		nonce:      "abc",
	}
	// JSON input with @action and some fields
	input := `{
		"@action": "my_action",
		"foo": "bar",
		"num": 123,
		
	}
<|FINAL_ANSWER_abc|>
the answer
<|FINAL_ANSWER_END_abc|>

`

	ctx := context.Background()
	action := maker.ReadFromReader(ctx, bytes.NewReader([]byte(input)))

	if action.Name() != "my_action" {
		t.Errorf("expected action name 'my_action', got %s", action.Name())
	}
	if got := action.GetString("foo"); got != "bar" {
		t.Errorf("expected foo='bar', got %v", got)
	}
	if got := action.GetInt("num"); got != 123 {
		t.Errorf("expected num=123, got %v", got)
	}
	if got := action.GetString("answer"); got != "the answer" {
		t.Errorf("expected answer='the answer', got %v", got)
	}
}

func TestSupperAction_Extractor(t *testing.T) {
	raw := `{
    "type": "object",
    "required": [
        "@action",
        "tasks",
        "main_task",
        "main_task_goal"
    ],
    "properties": {
        "@action": {
            "const": "plan"
        },
        "main_task": {
            "type": "string",
            "description": "对指定目标进行 XSS 漏洞检测，识别输入点并注入测试 payload，输出漏洞分析结论"
        },
        "main_task_goal": {
            "type": "string",
            "description": "完成目标的 XSS 漏洞扫描与验证，判断是否存在反射型、存储型或 DOM 型 XSS 漏洞，并输出有效 payload 和响应结果"
        },
        "tasks": []
    }
}
`
	ctx := context.Background()
	action, err := ExtractActionFromStream(ctx, strings.NewReader(raw), "plan")
	require.NoError(t, err)
	require.Equal(t, "plan", action.ActionType())
	params := action.GetParams()
	require.True(t, params.Has("main_task"))
	require.True(t, params.Has("main_task_goal"))
	require.True(t, params.Has("tasks"))
}

func TestSupperAction_WaitString(t *testing.T) {
	token := uuid.NewString()
	raw := `{
    "type": "object",
    "required": [
        "@action",
        "tasks",
        "main_task",
        "main_task_goal"
    ],
    "properties": {
        "@action": {
            "const": "plan"
        },
		"mytest": "` + token + `",
        "main_task": {
            "type": "string",
            "description": "对指定目标进行 XSS 漏洞检测，识别输入点并注入测试 payload，输出漏洞分析结论"
        },
        "main_task_goal": {
            "type": "string",
            "description": "完成目标的 XSS 漏洞扫描与验证，判断是否存在反射型、存储型或 DOM 型 XSS 漏洞，并输出有效 payload 和响应结果"
        },
        "tasks": []
    }
}
`
	ctx := context.Background()
	action, err := ExtractActionFromStream(ctx, strings.NewReader(raw), "plan")
	require.NoError(t, err)
	require.Equal(t, action.GetString("mytest"), token)
}

func TestSupperAction_WaitString_MissingParam(t *testing.T) {
	token := uuid.NewString()
	raw := `{ "type": "object", "required": [ "@action", "main_task" ], "properties": { "@action": {"const": "plan"}, "main_task": {"type": "string"}, "mytest": "` + token + `" } }`
	ctx := context.Background()
	action, err := ExtractActionFromStream(ctx, strings.NewReader(raw), "plan")
	require.NoError(t, err)
	require.Equal(t, action.GetString("mytest"), token)
	require.Equal(t, action.GetString("not_exist"), "")
}

func TestSupperAction_WaitObject(t *testing.T) {
	token := uuid.NewString()
	raw := `{ "type": "object", "required": [ "@action", "main_task", "info" ], "properties": { "@action": {"const": "plan"}, "main_task": {"type": "string"}, "info": { "type": "object", "mytest": "` + token + `", "age": 18 } } }`
	ctx := context.Background()
	action, err := ExtractActionFromStream(ctx, strings.NewReader(raw), "plan")
	require.NoError(t, err)
	info := action.GetInvokeParams("info")
	require.NotNil(t, info)
	require.Equal(t, info["mytest"], token)
	require.Equal(t, info["age"], 18)
}

func TestSupperAction_WaitStringSlice(t *testing.T) {
	raw := `{ "type": "object", "required": [ "@action", "items" ], "properties": { "@action": {"const": "plan"}, "items": ["a", "b", "c"] } }`
	ctx := context.Background()
	action, err := ExtractActionFromStream(ctx, strings.NewReader(raw), "plan")
	require.NoError(t, err)
	items := action.GetStringSlice("items")
	require.Len(t, items, 3)
	require.Contains(t, items, "a")
	require.Contains(t, items, "b")
	require.Contains(t, items, "c")
}

func TestSupperAction_Get_WaitForParam(t *testing.T) {
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	uuidString := uuid.NewString()
	go func() {
		// 构造嵌套 JSON，A.B 字段延迟写入
		pw.Write([]byte(`{ "@action": "plan", "A": { "B": "`))
		pw.Write([]byte(uuidString))
		pw.Write([]byte(`", "C": 123 }, "other": "value" }`))
		pw.Write([]byte("abc")) // 多写一些无关内容
		time.Sleep(20 * time.Second)
		pw.Close()
	}()

	go func() {
		action, err := ExtractActionFromStream(ctx, pr, "plan")
		require.NoError(t, err)
		val := action.GetString("A.B")
		require.Equal(t, val, uuidString)
		other := action.GetString("other")
		require.Equal(t, other, "value")
		c := action.GetInt("A.C")
		require.Equal(t, c, 123)
		close(done)
	}()

	select {
	case <-done:
		// 正常结束
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestSupperAction_TagToKey(t *testing.T) {
	tagName := "FINAL_ANSWER"
	keyName := "answer"
	once := uuid.NewString()
	tagData := uuid.NewString()
	input := fmt.Sprintf(`
	{
		"@action": "plan"
	}
<|FINAL_ANSWER_`+once+`|>
%s
<|FINAL_ANSWER_END_`+once+`|>
`, tagData)
	ctx := context.Background()
	maker := NewActionMaker("plan",
		WithActionTagToKey(tagName, keyName),
		WithActionNonce(once),
	)
	action := maker.ReadFromReader(ctx, strings.NewReader(input))
	require.Equal(t, action.GetString(keyName), tagData)
}

func TestSupperAction_FieldStreamHandler(t *testing.T) {
	var streamed bytes.Buffer
	fieldName := "stream_field"
	randomValue := uuid.NewString()
	input := fmt.Sprintf(`{
		"@action": "plan",
		"stream_field": "%s"
	}`, randomValue)

	wg := sync.WaitGroup{}
	ctx := context.Background()
	wg.Add(1)
	maker := NewActionMaker("plan",
		WithActionFieldStreamHandler([]string{fieldName}, func(key string, r io.Reader) {
			defer wg.Done()
			data, err := io.ReadAll(r)
			require.NoError(t, err)
			quote, err := codec.StrConvUnquote(string(data))
			require.NoError(t, err)
			streamed.Write([]byte(quote))
		}),
	)
	maker.ReadFromReader(ctx, strings.NewReader(input))
	wg.Wait()
	require.Equal(t, streamed.String(), randomValue)
}

func TestSupperAction_TagToKeyStream(t *testing.T) {
	tagName := "FINAL_ANSWER"
	keyName := "answer"
	once := uuid.NewString()
	tagData := uuid.NewString()
	checkTagStream := false
	input := fmt.Sprintf(`
	{
		"@action": "plan"
		"answer":"wait tag",
	}
<|FINAL_ANSWER_`+once+`|>
%s
<|FINAL_ANSWER_END_`+once+`|>
`, tagData)
	ctx := context.Background()
	maker := NewActionMaker("plan",
		WithActionTagToKey(tagName, keyName),
		WithActionNonce(once),
		WithActionFieldStreamHandler([]string{keyName}, func(key string, r io.Reader) {
			// no-op, just to test stream handling
			data, err := io.ReadAll(r)
			require.NoError(t, err)
			if string(data) == tagData {
				checkTagStream = true
			}
		}),
	)
	action := maker.ReadFromReader(ctx, strings.NewReader(input))
	action.WaitParse(ctx)
	require.Equal(t, action.GetString(keyName), tagData)
	action.WaitStream(ctx)
	require.True(t, checkTagStream)
}

// TestActionMaker_ExtraNonces_LLMUsesStableLiteral 验证 LLM 照抄占位符字面量
// nonce "[current-nonce]" 输出 AITAG 时, 由 ExtraNonces 注册的 callback 命中.
//
// 这是 CACHE_TOOL_CALL 双注册兜底的"LLM 字面照抄"分支:
//   - prompt 中 TOOL_PARAM_command 用 "[current-nonce]" 渲染, USER_QUERY 用 turn nonce 渲染
//   - LLM 没识破占位符, 直接照抄输出 <|TOOL_PARAM_command_[current-nonce]|>...
//   - ActionMaker 通过 ExtraNonces 给 TOOL_PARAM_command 同时注册了 turn nonce
//     和 "[current-nonce]" 两个 callback, 字面量分支命中
//
// 关键词: TestActionMaker, ExtraNonces, [current-nonce] 字面照抄,
//        CACHE_TOOL_CALL, 双注册兜底
func TestActionMaker_ExtraNonces_LLMUsesStableLiteral(t *testing.T) {
	turnNonce := uuid.NewString()
	stableNonce := "[current-nonce]"
	queryData := uuid.NewString()
	cmdData := uuid.NewString()

	input := fmt.Sprintf(`
	{ "@action": "directly_call_tool" }
<|USER_QUERY_%s|>
%s
<|USER_QUERY_END_%s|>
<|TOOL_PARAM_command_%s|>
%s
<|TOOL_PARAM_command_END_%s|>
`, turnNonce, queryData, turnNonce, stableNonce, cmdData, stableNonce)

	ctx := context.Background()
	maker := NewActionMaker("directly_call_tool",
		WithActionNonce(turnNonce),
		WithActionTagToKey("USER_QUERY", "user_query"),
		WithActionTagToKeyAndExtraNonces("TOOL_PARAM_command", "__aitag__command", stableNonce),
	)
	action := maker.ReadFromReader(ctx, strings.NewReader(input))
	action.WaitParse(ctx)
	require.Equal(t, queryData, action.GetString("user_query"))
	require.Equal(t, cmdData, action.GetString("__aitag__command"))
}

// TestActionMaker_ExtraNonces_LLMUsesTurnNonce 验证 LLM 把占位符替换为 turn nonce
// 输出 AITAG 时, 由 m.nonce 默认 callback 命中 (双注册的另一分支).
//
// 这是 CACHE_TOOL_CALL 双注册兜底的"LLM 识破占位符"分支:
//   - ActionMaker 同时注册 (TOOL_PARAM_command, turnNonce) + (TOOL_PARAM_command, "[current-nonce]")
//   - LLM 输出 <|TOOL_PARAM_command_<turnNonce>|>... 命中 turn nonce 分支
//
// 关键词: TestActionMaker, ExtraNonces, turn nonce 替换占位符, 双注册兜底
func TestActionMaker_ExtraNonces_LLMUsesTurnNonce(t *testing.T) {
	turnNonce := uuid.NewString()
	stableNonce := "[current-nonce]"
	cmdData := uuid.NewString()

	input := fmt.Sprintf(`
	{ "@action": "directly_call_tool" }
<|TOOL_PARAM_command_%s|>
%s
<|TOOL_PARAM_command_END_%s|>
`, turnNonce, cmdData, turnNonce)

	ctx := context.Background()
	maker := NewActionMaker("directly_call_tool",
		WithActionNonce(turnNonce),
		WithActionTagToKeyAndExtraNonces("TOOL_PARAM_command", "__aitag__command", stableNonce),
	)
	action := maker.ReadFromReader(ctx, strings.NewReader(input))
	action.WaitParse(ctx)
	require.Equal(t, cmdData, action.GetString("__aitag__command"))
}

// TestActionMaker_ExtraNonces_EmptyExtrasFallsBackToTurn 验证 ExtraNonces 全为空
// 字符串时被忽略, 等价于 WithActionTagToKey 行为 (只注册 m.nonce 一份 callback).
// 关键词: TestActionMaker, ExtraNonces 空字符串过滤, 向后兼容
func TestActionMaker_ExtraNonces_EmptyExtrasFallsBackToTurn(t *testing.T) {
	turnNonce := uuid.NewString()
	tagData := uuid.NewString()
	input := fmt.Sprintf(`
	{ "@action": "plan" }
<|FINAL_ANSWER_%s|>
%s
<|FINAL_ANSWER_END_%s|>
`, turnNonce, tagData, turnNonce)

	ctx := context.Background()
	maker := NewActionMaker("plan",
		WithActionNonce(turnNonce),
		WithActionTagToKeyAndExtraNonces("FINAL_ANSWER", "answer", "", "  "),
	)
	action := maker.ReadFromReader(ctx, strings.NewReader(input))
	action.WaitParse(ctx)
	require.Equal(t, tagData, action.GetString("answer"))
}

// TestActionMaker_LiteralCurrentNoncePlaceholder_FactsCase 验证: 当 AI 把
// prompt 示例里的 "CURRENT_NONCE" 占位符当字面量直接照抄输出时, ActionMaker
// 通过 ExtraNonces 注册 LiteralCurrentNoncePlaceholder 仍能命中, 把 FACTS
// 内容写入 action.params[facts]. 这是 reactloops/exec.go buildActionTagOption
// 默认追加 LiteralCurrentNoncePlaceholder 后的端到端兜底验证.
//
// 这个 case 模拟生产中实际观察到的 AI 输出:
//
//	{"@action": "output_facts"}
//	<|FACTS_CURRENT_NONCE|>
//	## 目标
//	- id.redhaze.top: 198.18.0.53
//	<|FACTS_END_CURRENT_NONCE|>
//
// 旧实现下只注册 turn nonce, callback 永远命中不到, action.facts 为空,
// verifier 触发 5 次重试黑洞 + [AI Transaction Failed] 致命中断. 新实现
// 默认双注册 turn nonce + LiteralCurrentNoncePlaceholder, 两种 AI 输出
// 行为都能正确抽出内容.
//
// 关键词: LiteralCurrentNoncePlaceholder, CURRENT_NONCE 字面量兼容,
//
//	output_facts FACTS 抽取, 5 次重试黑洞修复
func TestActionMaker_LiteralCurrentNoncePlaceholder_FactsCase(t *testing.T) {
	turnNonce := uuid.NewString()
	factsBody := "## 目标\n- id.redhaze.top: 198.18.0.53"
	input := fmt.Sprintf(`
{ "@action": "output_facts" }
<|FACTS_%s|>
%s
<|FACTS_END_%s|>
`, LiteralCurrentNoncePlaceholder, factsBody, LiteralCurrentNoncePlaceholder)

	ctx := context.Background()
	maker := NewActionMaker("output_facts",
		WithActionNonce(turnNonce),
		WithActionTagToKeyAndExtraNonces("FACTS", "facts", LiteralCurrentNoncePlaceholder),
	)
	action := maker.ReadFromReader(ctx, strings.NewReader(input))
	action.WaitParse(ctx)
	require.Equal(t, factsBody, strings.TrimSpace(action.GetString("facts")),
		"AI 把 CURRENT_NONCE 当字面量照抄时, ExtraNonces 双注册必须命中并把 FACTS 抽到 action.facts")
}

// TestActionMaker_ExtraNonces_DeprecatedSingleAlias 验证已 deprecated 的
// WithActionTagToKeyAndNonce 选项被 redirect 到 ExtraNonces 之后, 旧调用路径
// 仍然兼容 (单 nonce 作为额外候选追加, m.nonce 始终保留).
//
// 关键词: TestActionMaker, WithActionTagToKeyAndNonce deprecated 兼容, ExtraNonces
func TestActionMaker_ExtraNonces_DeprecatedSingleAlias(t *testing.T) {
	stableNonce := "[current-nonce]"
	cmdData := uuid.NewString()
	input := fmt.Sprintf(`
	{ "@action": "directly_call_tool" }
<|TOOL_PARAM_command_%s|>
%s
<|TOOL_PARAM_command_END_%s|>
`, stableNonce, cmdData, stableNonce)

	ctx := context.Background()
	maker := NewActionMaker("directly_call_tool",
		WithActionTagToKeyAndNonce("TOOL_PARAM_command", "__aitag__command", stableNonce),
	)
	action := maker.ReadFromReader(ctx, strings.NewReader(input))
	action.WaitParse(ctx)
	require.Equal(t, cmdData, action.GetString("__aitag__command"))
}
