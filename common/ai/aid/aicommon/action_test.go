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
		once:       "abc",
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
	action, err := ExtractActionFormStream(ctx, strings.NewReader(raw), "plan")
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
	action, err := ExtractActionFormStream(ctx, strings.NewReader(raw), "plan")
	require.NoError(t, err)
	require.Equal(t, action.GetString("mytest"), token)
}

func TestSupperAction_WaitString_MissingParam(t *testing.T) {
	token := uuid.NewString()
	raw := `{ "type": "object", "required": [ "@action", "main_task" ], "properties": { "@action": {"const": "plan"}, "main_task": {"type": "string"}, "mytest": "` + token + `" } }`
	ctx := context.Background()
	action, err := ExtractActionFormStream(ctx, strings.NewReader(raw), "plan")
	require.NoError(t, err)
	require.Equal(t, action.GetString("mytest"), token)
	require.Equal(t, action.GetString("not_exist"), "")
}

func TestSupperAction_WaitObject(t *testing.T) {
	token := uuid.NewString()
	raw := `{ "type": "object", "required": [ "@action", "main_task", "info" ], "properties": { "@action": {"const": "plan"}, "main_task": {"type": "string"}, "info": { "type": "object", "mytest": "` + token + `", "age": 18 } } }`
	ctx := context.Background()
	action, err := ExtractActionFormStream(ctx, strings.NewReader(raw), "plan")
	require.NoError(t, err)
	info := action.GetInvokeParams("info")
	require.NotNil(t, info)
	require.Equal(t, info["mytest"], token)
	require.Equal(t, info["age"], 18)
}

func TestSupperAction_WaitStringSlice(t *testing.T) {
	raw := `{ "type": "object", "required": [ "@action", "items" ], "properties": { "@action": {"const": "plan"}, "items": ["a", "b", "c"] } }`
	ctx := context.Background()
	action, err := ExtractActionFormStream(ctx, strings.NewReader(raw), "plan")
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
		action, err := ExtractActionFormStream(ctx, pr, "plan")
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
		WithActionOnce(once),
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
		WithActionOnce(once),
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
