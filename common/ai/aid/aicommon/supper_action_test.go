package aicommon

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestSupperAction_SetAndGet(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(3)
	action := &SupperAction{
		name:           "test_action",
		params:         make(aitool.InvokeParams),
		barrier:        utils.NewCondBarrierContext(ctx),
		wholeParamsKey: "whole",
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
	maker := &SupperActionMaker{
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
