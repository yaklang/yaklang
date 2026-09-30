package yakvm

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestSelectAdversarialMaximumCases(t *testing.T) {
	f := NewFrame(New())
	f.ctx = context.Background()
	operands := make([]*Value, 65535)
	for i := range operands {
		operands[i] = selectTestOperand(reflect.SelectRecv, (chan int)(nil), nil)
	}
	for _, position := range []int{0, 32767, 65534} {
		ch := make(chan int, 1)
		ch <- position
		operands[position] = selectTestOperand(reflect.SelectRecv, ch, nil)
		got := f.SelectChannels(operands)
		if !reflect.DeepEqual(got, []any{position, position, true}) || len(ch) != 0 {
			t.Fatalf("maximum select: %v", got)
		}
		operands[position] = selectTestOperand(reflect.SelectRecv, (chan int)(nil), nil)
	}
	operands[65534] = selectTestOperand(reflect.SelectDefault, nil, nil)
	if got := f.SelectChannels(operands); !reflect.DeepEqual(got, []any{65534, nil, false}) {
		t.Fatalf("maximum default: %v", got)
	}
}

func TestSelectAdversarialDisabledCasesStillValidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		operand *Value
		message string
	}{
		{"wrong direction", selectTestOperand(reflect.SelectRecv, (chan<- int)(nil), nil), "cannot receive from send-only channel"},
		{"wrong payload", selectTestOperand(reflect.SelectSend, (chan int)(nil), "wrong"), "cannot send string to channel of int"},
		{"nil payload", selectTestOperand(reflect.SelectSend, (chan int)(nil), nil), "cannot send nil to channel of int"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := NewFrame(New())
			f.ctx = context.Background()
			defer func() {
				if got := recover(); fmt.Sprint(got) != tc.message {
					t.Fatalf("invalid disabled case: %v, want %s", got, tc.message)
				}
			}()
			f.SelectChannels([]*Value{tc.operand, selectTestOperand(reflect.SelectDefault, nil, nil)})
		})
	}
}

func BenchmarkSelectAdversarialRuntime(b *testing.B) {
	for _, count := range []int{1, 16, 256, 4096, 65535} {
		for _, sparse := range []bool{false, true} {
			name := "ready"
			if sparse {
				name = "sparse"
			}
			b.Run(fmt.Sprintf("%d/%s", count, name), func(b *testing.B) {
				f := NewFrame(New())
				f.ctx = context.Background()
				operands := make([]*Value, count)
				ch := make(chan int)
				close(ch)
				for i := range operands {
					channel := ch
					if sparse && i != count-1 {
						channel = nil
					}
					operands[i] = selectTestOperand(reflect.SelectRecv, channel, nil)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					got := f.SelectChannels(operands)
					if sparse && got[0] != count-1 {
						b.Fatalf("chosen=%v", got)
					}
				}
			})
		}
	}
}

func TestSelectAdversarialCompactedIndexOverflow(t *testing.T) {
	for _, position := range []int{1, 8, 17, 62} {
		f := NewFrame(New())
		f.ctx = context.Background()
		operands := make([]*Value, 64)
		for i := range operands {
			ch := make(chan int)
			if i%3 == 0 {
				ch = nil
			}
			operands[i] = selectTestOperand(reflect.SelectRecv, ch, nil)
		}
		ch := make(chan int, 1)
		ch <- position
		operands[position] = selectTestOperand(reflect.SelectRecv, ch, nil)
		if got := f.SelectChannels(operands); !reflect.DeepEqual(got, []any{position, position, true}) {
			t.Fatalf("compacted receive index: %v, want=%d", got, position)
		}
	}
	for _, position := range []int{0, 7, 8, 16, 63} {
		f := NewFrame(New())
		f.ctx = context.Background()
		operands := make([]*Value, 64)
		for i := range operands {
			ch := make(chan int)
			if i%3 == 0 {
				ch = nil
			}
			operands[i] = selectTestOperand(reflect.SelectRecv, ch, nil)
		}
		operands[position] = selectTestOperand(reflect.SelectDefault, nil, nil)
		if got := f.SelectChannels(operands); !reflect.DeepEqual(got, []any{position, nil, false}) {
			t.Fatalf("compacted default index: %v, want=%d", got, position)
		}
	}
}

func TestSelectAdversarialCompactedSendIndexes(t *testing.T) {
	f := NewFrame(New())
	f.ctx = context.Background()
	ch := make(chan int, 1)
	operands := make([]*Value, 64)
	for i := range operands {
		channel := ch
		if i%3 == 0 {
			channel = nil
		}
		operands[i] = selectTestOperand(reflect.SelectSend, channel, i)
	}
	for i := 0; i < 128; i++ {
		got := f.SelectChannels(operands)
		position, ok := got[0].(int)
		if !ok || position < 0 || position >= len(operands) || position%3 == 0 {
			t.Fatalf("selected disabled/invalid send: %v", got)
		}
		if !reflect.DeepEqual(got, []any{position, nil, false}) {
			t.Fatalf("send result changed: %v", got)
		}
		if sent := <-ch; sent != position {
			t.Fatalf("sent=%d selected=%d", sent, position)
		}
	}
}

func TestSelectAdversarialCompactedCancellation(t *testing.T) {
	f := NewFrame(New())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	f.ctx = ctx
	operands := make([]*Value, 4096)
	for i := range operands {
		operands[i] = selectTestOperand(reflect.SelectRecv, (chan int)(nil), nil)
	}
	got := f.SelectChannels(operands)
	if !reflect.DeepEqual(got, []any{-1, nil, false}) || ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("all-disabled cancellation: %v, error=%v", got, ctx.Err())
	}
}

func TestSelectAdversarialAlternatingChannelTypes(t *testing.T) {
	f := NewFrame(New())
	f.ctx = context.Background()
	ch := make(chan string, 1)
	ch <- "payload"
	operands := []*Value{
		selectTestOperand(reflect.SelectRecv, (chan int)(nil), nil),
		selectTestOperand(reflect.SelectRecv, (<-chan int)(nil), nil),
		selectTestOperand(reflect.SelectSend, (chan<- int)(nil), 7),
		selectTestOperand(reflect.SelectRecv, (<-chan string)(ch), nil),
	}
	if got := f.SelectChannels(operands); !reflect.DeepEqual(got, []any{3, "payload", true}) {
		t.Fatalf("alternating channel types: %v", got)
	}
	for _, tc := range []struct {
		operand *Value
		message string
	}{
		{selectTestOperand(reflect.SelectSend, (<-chan int)(nil), 7), "cannot send on receive-only channel"},
		{selectTestOperand(reflect.SelectRecv, (chan<- int)(nil), nil), "cannot receive from send-only channel"},
	} {
		func() {
			defer func() {
				if got := recover(); fmt.Sprint(got) != tc.message {
					t.Fatalf("changed-type validation: %v, want %s", got, tc.message)
				}
			}()
			f.SelectChannels([]*Value{selectTestOperand(reflect.SelectRecv, (chan int)(nil), nil), tc.operand, selectTestOperand(reflect.SelectDefault, nil, nil)})
		}()
	}
}

func TestSelectAdversarialDirectionRepresentation(t *testing.T) {
	f := NewFrame(New())
	f.ctx = context.Background()
	for _, direction := range []any{int(reflect.SelectRecv), int64(reflect.SelectRecv), uint8(reflect.SelectRecv), reflect.SelectRecv} {
		ch := make(chan int, 1)
		ch <- 7
		operand := selectTestOperand(reflect.SelectRecv, ch, nil)
		operand.ValueList()[0] = NewValue("", direction, "")
		if got := f.SelectChannels([]*Value{operand}); !reflect.DeepEqual(got, []any{0, 7, true}) {
			t.Fatalf("direction %T: %v", direction, got)
		}
	}
	for _, direction := range []*Value{nil, NewValue("", nil, ""), NewIntValue(-1)} {
		func() {
			defer func() {
				if got := recover(); fmt.Sprint(got) != "invalid select direction" {
					t.Fatalf("invalid direction: %v", got)
				}
			}()
			operand := selectTestOperand(reflect.SelectRecv, (chan int)(nil), nil)
			operand.ValueList()[0] = direction
			f.SelectChannels([]*Value{operand})
		}()
	}
}
