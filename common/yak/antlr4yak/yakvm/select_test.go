package yakvm

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func selectTestOperand(dir reflect.SelectDir, ch, send any) *Value {
	return NewValue("__opcode_list__", []*Value{NewIntValue(int(dir)), NewValue("", ch, ""), NewValue("", send, "")}, "")
}

func TestSelectRuntimeCancelledBeforeCommunication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := NewFrame(New())
	f.ctx = ctx
	ch := make(chan int, 1)
	ch <- 7
	got := f.SelectChannels([]*Value{selectTestOperand(reflect.SelectRecv, ch, nil), selectTestOperand(reflect.SelectDefault, nil, nil)})
	if !reflect.DeepEqual(got, []any{-1, nil, false}) || len(ch) != 1 {
		t.Fatalf("cancelled select communicated: %v, buffered=%d", got, len(ch))
	}
}

func TestSelectRuntimeOperandValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		operands []*Value
		message  string
	}{
		{"limit", make([]*Value, 65536), "too many cases"},
		{"shape", []*Value{NewIntValue(1)}, "invalid select operands"},
		{"direction", []*Value{selectTestOperand(0, nil, nil)}, "invalid select direction"},
		{"defaults", []*Value{selectTestOperand(reflect.SelectDefault, nil, nil), selectTestOperand(reflect.SelectDefault, nil, nil)}, "multiple default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := NewFrame(New())
			f.ctx = context.Background()
			defer func() {
				p := recover()
				if p == nil || !strings.Contains(fmt.Sprint(p), tc.message) {
					t.Fatalf("wanted %q, got %v", tc.message, p)
				}
			}()
			f.SelectChannels(tc.operands)
		})
	}
}
