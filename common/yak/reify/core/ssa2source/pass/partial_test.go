package pass_test

import (
	"context"
	. "github.com/yaklang/yaklang/common/yak/reify/core/ssa2source/pass"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
)

func TestSimplifyPartialRegisterMerge(t *testing.T) {
	program := parseSSA(t, `partial <i64> (1) low
type: (i64) -> i64
entry_0:
    <i64> t1 = <i64> 0x41
    <i64> t2 = <i64> t1 and-not <i64> 0xff
    <i64> t3 = <i64> low and <i64> 0xff
    <i64> t4 = <i64> t2 or <i64> t3
    <i64> t5 = type_cast[i64] <i64> t4
    ret <i64> t5`)
	SimplifyPartialWrites(program)
	text, err := ssa2source.Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "and-not 0xff") {
		t.Fatalf("partial register merge survived:\n%s", text)
	}
}
