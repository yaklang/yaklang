package ssa

import (
	"reflect"
	"testing"

	"github.com/yaklang/yaklang/common/utils/memedit"
)

func TestLazyFunctionStackUsesSourceBeforeGeneratedName(t *testing.T) {
	editor := memedit.NewMemEditor("first\nsecond\nthird\n")
	first := &Function{anValue: NewValue()}
	first.SetName("AnonymousFunc-99")
	first.SetRange(editor.GetRangeOffset(0, 5))
	second := &Function{anValue: NewValue()}
	second.SetName("AnonymousFunc-1")
	second.SetRange(editor.GetRangeOffset(6, 12))
	for _, input := range [][]*Function{{first, second}, {second, first}} {
		sortLazyFunctionStack(input)
		var got []*Function
		for len(input) != 0 {
			got = append(got, input[len(input)-1])
			input = input[:len(input)-1]
		}
		if !reflect.DeepEqual(got, []*Function{first, second}) {
			t.Fatal("source order must win over allocation counters and candidate insertion order")
		}
	}
}
