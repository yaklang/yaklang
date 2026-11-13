package pass_test

import (
	"context"
	. "github.com/yaklang/yaklang/common/yak/reify/core/ssa2source/pass"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
)

func TestInferPointerFromMallocAndFree(t *testing.T) {
	program := parseSSA(t, `extern malloc
extern free
caller
type: () -> void
entry_0:
    <i64> t1 = <i64> 8
    <any> t2 = call <any> malloc (<i64> t1) binding[] member[]
    <i64> t3 = <i64> 0
    <any> t4 = <any> t2 eq <i64> t3
    <any> t5 = call <any> free (<any> t2) binding[] member[]
    ret`)
	mallocCall := findCall(t, program, "malloc")

	ApplyFunctionDefine(program)
	InferTypes(program)
	allocation, ok := mallocCall.GetValueById(mallocCall.GetId())
	if !ok || allocation == nil {
		t.Fatal("malloc result is missing")
	}
	if allocation.GetType().String() != "ptr64" {
		t.Fatalf("malloc result type = %s", allocation.GetType())
	}
	text, err := ssa2source.Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "malloc(") || !strings.Contains(text, "free(") {
		t.Fatalf("source lacks inferred pointer values:\n%s", text)
	}
}
