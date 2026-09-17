package antlr4yak

import (
	"context"
	"reflect"
	"testing"
)

func TestCoreChannelPreservesDynamicType(t *testing.T) {
	for _, input := range []any{int64(42), uint64(1<<63 + 9), float32(1.25)} {
		e := New()
		ch := make(chan any, 1)
		e.ImportLibs(map[string]any{"ch": ch, "input": input})
		if err := e.SafeEval(context.Background(), "ch <- input"); err != nil {
			t.Fatal(err)
		}
		got := <-ch
		if !reflect.DeepEqual(got, input) {
			t.Errorf("send changed dynamic type/value: before %T(%v), after %T(%v)", input, input, got, got)
		}
	}
}
